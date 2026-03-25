package fuse

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	gofuse "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"github.com/FanBB2333/downleaf/internal/api"
	"github.com/FanBB2333/downleaf/internal/cache"
	"github.com/FanBB2333/downleaf/internal/model"
)

// OverleafFS holds shared state for the entire filesystem.
type OverleafFS struct {
	Client *api.Client
	Cache  *cache.Cache

	projectsMu sync.RWMutex
	projects   []model.Project

	// Per-project Socket.IO connections and cached trees
	sioMu   sync.Mutex
	sioConn map[string]*api.SocketIOClient // projectID -> socket.io client
	trees   map[string]*model.ProjectMeta  // projectID -> project tree
}

// NewOverleafFS creates a new filesystem state.
func NewOverleafFS(client *api.Client) *OverleafFS {
	return &OverleafFS{
		Client:  client,
		Cache:   cache.New(5 * time.Minute),
		sioConn: make(map[string]*api.SocketIOClient),
		trees:   make(map[string]*model.ProjectMeta),
	}
}

func (o *OverleafFS) refreshProjects() ([]model.Project, error) {
	o.projectsMu.Lock()
	defer o.projectsMu.Unlock()

	projects, err := o.Client.ListProjects()
	if err != nil {
		return nil, err
	}
	o.projects = projects
	return projects, nil
}

// getProjectTree returns the project file tree, connecting via Socket.IO if needed.
func (o *OverleafFS) getProjectTree(projectID string) (*model.ProjectMeta, error) {
	o.sioMu.Lock()
	defer o.sioMu.Unlock()

	if tree, ok := o.trees[projectID]; ok {
		return tree, nil
	}

	sio := api.NewSocketIOClient(o.Client.SiteURL, o.Client.Identity)
	tree, err := sio.JoinProject(projectID)
	if err != nil {
		return nil, err
	}

	o.sioConn[projectID] = sio
	o.trees[projectID] = tree
	return tree, nil
}

// getDocContent retrieves a doc's content via Socket.IO joinDoc.
func (o *OverleafFS) getDocContent(projectID, docID string) ([]byte, error) {
	o.sioMu.Lock()
	sio, ok := o.sioConn[projectID]
	o.sioMu.Unlock()

	if !ok {
		return nil, fmt.Errorf("no socket.io connection for project %s", projectID)
	}

	content, _, err := sio.JoinDoc(projectID, docID)
	if err != nil {
		return nil, err
	}

	sio.LeaveDoc(projectID, docID)
	return []byte(content), nil
}

// ==========================================================================
// RootNode: top-level directory listing all projects
// ==========================================================================

type RootNode struct {
	gofuse.Inode
	ofs *OverleafFS
}

var _ gofuse.NodeReaddirer = (*RootNode)(nil)
var _ gofuse.NodeLookuper = (*RootNode)(nil)

func (r *RootNode) Readdir(ctx context.Context) (gofuse.DirStream, syscall.Errno) {
	projects, err := r.ofs.refreshProjects()
	if err != nil {
		log.Printf("refresh projects: %v", err)
		return nil, syscall.EIO
	}

	var entries []fuse.DirEntry
	for _, p := range projects {
		if p.Archived || p.Trashed {
			continue
		}
		entries = append(entries, fuse.DirEntry{
			Name: sanitizeName(p.Name),
			Mode: syscall.S_IFDIR,
		})
	}
	return gofuse.NewListDirStream(entries), 0
}

func (r *RootNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*gofuse.Inode, syscall.Errno) {
	r.ofs.projectsMu.RLock()
	projects := r.ofs.projects
	r.ofs.projectsMu.RUnlock()

	for _, p := range projects {
		if sanitizeName(p.Name) == name && !p.Archived && !p.Trashed {
			node := &ProjectNode{ofs: r.ofs, project: p}
			out.Mode = syscall.S_IFDIR | 0555
			child := r.NewInode(ctx, node, gofuse.StableAttr{Mode: syscall.S_IFDIR})
			return child, 0
		}
	}
	return nil, syscall.ENOENT
}

func (r *RootNode) Getattr(ctx context.Context, fh gofuse.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFDIR | 0555
	return 0
}

// ==========================================================================
// ProjectNode: a directory representing one project
// ==========================================================================

type ProjectNode struct {
	gofuse.Inode
	ofs     *OverleafFS
	project model.Project
}

var _ gofuse.NodeReaddirer = (*ProjectNode)(nil)
var _ gofuse.NodeLookuper = (*ProjectNode)(nil)

func (p *ProjectNode) Readdir(ctx context.Context) (gofuse.DirStream, syscall.Errno) {
	tree, err := p.ofs.getProjectTree(p.project.ID)
	if err != nil {
		log.Printf("get tree for %s: %v", p.project.Name, err)
		return nil, syscall.EIO
	}

	if len(tree.RootFolder) == 0 {
		return gofuse.NewListDirStream(nil), 0
	}

	return gofuse.NewListDirStream(folderEntries(&tree.RootFolder[0])), 0
}

func (p *ProjectNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*gofuse.Inode, syscall.Errno) {
	if isSystemFile(name) {
		return nil, syscall.ENOENT
	}

	tree, err := p.ofs.getProjectTree(p.project.ID)
	if err != nil {
		return nil, syscall.EIO
	}

	if len(tree.RootFolder) == 0 {
		return nil, syscall.ENOENT
	}

	return lookupInFolder(ctx, p, p.ofs, p.project.ID, &tree.RootFolder[0], name, out)
}

func (p *ProjectNode) Getattr(ctx context.Context, fh gofuse.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFDIR | 0555
	return 0
}

// ==========================================================================
// FolderNode: a subdirectory within a project
// ==========================================================================

type FolderNode struct {
	gofuse.Inode
	ofs       *OverleafFS
	projectID string
	folder    model.Folder
}

var _ gofuse.NodeReaddirer = (*FolderNode)(nil)
var _ gofuse.NodeLookuper = (*FolderNode)(nil)

func (f *FolderNode) Readdir(ctx context.Context) (gofuse.DirStream, syscall.Errno) {
	return gofuse.NewListDirStream(folderEntries(&f.folder)), 0
}

func (f *FolderNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*gofuse.Inode, syscall.Errno) {
	if isSystemFile(name) {
		return nil, syscall.ENOENT
	}
	return lookupInFolder(ctx, f, f.ofs, f.projectID, &f.folder, name, out)
}

func (f *FolderNode) Getattr(ctx context.Context, fh gofuse.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFDIR | 0555
	return 0
}

// ==========================================================================
// FileNode: a file (doc or binary) within a project
// ==========================================================================

type FileNode struct {
	gofuse.Inode
	ofs       *OverleafFS
	projectID string
	id        string
	name      string
	isDoc     bool
}

var _ gofuse.NodeOpener = (*FileNode)(nil)
var _ gofuse.NodeReader = (*FileNode)(nil)
var _ gofuse.NodeGetattrer = (*FileNode)(nil)

func (f *FileNode) Getattr(ctx context.Context, fh gofuse.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFREG | 0444

	cacheKey := f.projectID + "/" + f.id
	if data, ok := f.ofs.Cache.Get(cacheKey); ok {
		out.Size = uint64(len(data))
	}
	return 0
}

func (f *FileNode) Open(ctx context.Context, flags uint32) (gofuse.FileHandle, uint32, syscall.Errno) {
	cacheKey := f.projectID + "/" + f.id
	if _, ok := f.ofs.Cache.Get(cacheKey); !ok {
		data, err := f.fetchContent()
		if err != nil {
			log.Printf("fetch %s: %v", f.name, err)
			return nil, 0, syscall.EIO
		}
		f.ofs.Cache.Set(cacheKey, data)
	}
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

func (f *FileNode) Read(ctx context.Context, fh gofuse.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	cacheKey := f.projectID + "/" + f.id
	data, ok := f.ofs.Cache.Get(cacheKey)
	if !ok {
		var err error
		data, err = f.fetchContent()
		if err != nil {
			log.Printf("read %s: %v", f.name, err)
			return nil, syscall.EIO
		}
		f.ofs.Cache.Set(cacheKey, data)
	}

	end := int(off) + len(dest)
	if end > len(data) {
		end = len(data)
	}
	if int(off) >= len(data) {
		return fuse.ReadResultData(nil), 0
	}
	return fuse.ReadResultData(data[off:end]), 0
}

func (f *FileNode) fetchContent() ([]byte, error) {
	if f.isDoc {
		// .tex docs: get content via Socket.IO joinDoc
		return f.ofs.getDocContent(f.projectID, f.id)
	}
	// Binary files (images, PDFs): download via REST API
	return f.ofs.Client.DownloadFile(f.projectID, f.id)
}

// ==========================================================================
// Helpers
// ==========================================================================

func folderEntries(folder *model.Folder) []fuse.DirEntry {
	var entries []fuse.DirEntry

	for _, sub := range folder.Folders {
		entries = append(entries, fuse.DirEntry{
			Name: sanitizeName(sub.Name),
			Mode: syscall.S_IFDIR,
		})
	}
	for _, doc := range folder.Docs {
		entries = append(entries, fuse.DirEntry{
			Name: sanitizeName(doc.Name),
			Mode: syscall.S_IFREG,
		})
	}
	for _, ref := range folder.FileRefs {
		entries = append(entries, fuse.DirEntry{
			Name: sanitizeName(ref.Name),
			Mode: syscall.S_IFREG,
		})
	}
	return entries
}

func lookupInFolder(ctx context.Context, parent gofuse.InodeEmbedder, ofs *OverleafFS, projectID string, folder *model.Folder, name string, out *fuse.EntryOut) (*gofuse.Inode, syscall.Errno) {
	for _, sub := range folder.Folders {
		if sanitizeName(sub.Name) == name {
			node := &FolderNode{ofs: ofs, projectID: projectID, folder: sub}
			out.Mode = syscall.S_IFDIR | 0555
			child := parent.EmbeddedInode().NewInode(ctx, node, gofuse.StableAttr{Mode: syscall.S_IFDIR})
			return child, 0
		}
	}
	for _, doc := range folder.Docs {
		if sanitizeName(doc.Name) == name {
			node := &FileNode{ofs: ofs, projectID: projectID, id: doc.ID, name: doc.Name, isDoc: true}
			out.Mode = syscall.S_IFREG | 0444
			child := parent.EmbeddedInode().NewInode(ctx, node, gofuse.StableAttr{Mode: syscall.S_IFREG})
			return child, 0
		}
	}
	for _, ref := range folder.FileRefs {
		if sanitizeName(ref.Name) == name {
			node := &FileNode{ofs: ofs, projectID: projectID, id: ref.ID, name: ref.Name, isDoc: false}
			out.Mode = syscall.S_IFREG | 0444
			child := parent.EmbeddedInode().NewInode(ctx, node, gofuse.StableAttr{Mode: syscall.S_IFREG})
			return child, 0
		}
	}
	return nil, syscall.ENOENT
}

func sanitizeName(name string) string {
	return strings.ReplaceAll(name, "/", "_")
}

func isSystemFile(name string) bool {
	switch name {
	case ".DS_Store", ".Spotlight-V100", ".Trashes", ".fseventsd",
		"._DS_Store", ".localized", "Icon\r", ".hidden":
		return true
	}
	return strings.HasPrefix(name, "._")
}

// Mount mounts the Overleaf filesystem at the given mountpoint.
func Mount(mountpoint string, client *api.Client) error {
	if err := os.MkdirAll(mountpoint, 0755); err != nil {
		return fmt.Errorf("create mountpoint: %w", err)
	}

	ofs := NewOverleafFS(client)
	root := &RootNode{ofs: ofs}

	server, err := gofuse.Mount(mountpoint, root, &gofuse.Options{
		MountOptions: fuse.MountOptions{
			FsName: "downleaf",
			Name:   "overleaf",
		},
		FirstAutomaticIno: 1,
	})
	if err != nil {
		return fmt.Errorf("mount: %w", err)
	}

	log.Printf("Mounted at %s", mountpoint)
	log.Printf("Use 'umount %s' or press Ctrl+C to unmount", mountpoint)

	server.Wait()
	return nil
}

// Unmount unmounts the filesystem using the system umount command.
func Unmount(mountpoint string) error {
	return exec.Command("umount", mountpoint).Run()
}
