package fuse

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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

// fileMeta stores the metadata needed to upload a dirty file back to Overleaf.
type fileMeta struct {
	projectID string
	folderID  string
	name      string
}

// OverleafFS holds shared state for the entire filesystem.
type OverleafFS struct {
	Client        *api.Client
	Cache         *cache.Cache
	BatchMode     bool   // if true, Flush is a no-op; use FlushAll to sync
	projectFilter string // if set, only show projects matching this name or ID

	projectsMu sync.RWMutex
	projects   []model.Project

	sioMu   sync.Mutex
	sioConn map[string]*api.SocketIOClient
	trees   map[string]*model.ProjectMeta

	metaMu   sync.RWMutex
	metaMap  map[string]fileMeta // cacheKey -> file metadata for upload
}

func NewOverleafFS(client *api.Client) *OverleafFS {
	return &OverleafFS{
		Client:  client,
		Cache:   cache.New(5 * time.Minute),
		sioConn: make(map[string]*api.SocketIOClient),
		trees:   make(map[string]*model.ProjectMeta),
		metaMap: make(map[string]fileMeta),
	}
}

func (o *OverleafFS) registerMeta(cacheKey, projectID, folderID, name string) {
	o.metaMu.Lock()
	o.metaMap[cacheKey] = fileMeta{projectID: projectID, folderID: folderID, name: name}
	o.metaMu.Unlock()
}

func (o *OverleafFS) getMeta(cacheKey string) (fileMeta, bool) {
	o.metaMu.RLock()
	m, ok := o.metaMap[cacheKey]
	o.metaMu.RUnlock()
	return m, ok
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

func (o *OverleafFS) matchesFilter(p model.Project) bool {
	if o.projectFilter == "" {
		return true
	}
	return p.ID == o.projectFilter || strings.EqualFold(p.Name, o.projectFilter)
}

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

func (o *OverleafFS) invalidateTree(projectID string) {
	o.sioMu.Lock()
	delete(o.trees, projectID)
	if sio, ok := o.sioConn[projectID]; ok {
		sio.Disconnect()
		delete(o.sioConn, projectID)
	}
	o.sioMu.Unlock()
}

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
// RootNode
// ==========================================================================

type RootNode struct {
	gofuse.Inode
	ofs *OverleafFS
}

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
		if !r.ofs.matchesFilter(p) {
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
		if sanitizeName(p.Name) == name && !p.Archived && !p.Trashed && r.ofs.matchesFilter(p) {
			node := &ProjectNode{ofs: r.ofs, project: p}
			out.Mode = syscall.S_IFDIR | 0755
			child := r.NewInode(ctx, node, gofuse.StableAttr{Mode: syscall.S_IFDIR})
			return child, 0
		}
	}
	return nil, syscall.ENOENT
}

func (r *RootNode) Getattr(ctx context.Context, fh gofuse.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFDIR | 0755
	return 0
}

// ==========================================================================
// ProjectNode — delegates to root folder, acts as a DirNode
// ==========================================================================

type ProjectNode struct {
	gofuse.Inode
	ofs     *OverleafFS
	project model.Project
}

func (p *ProjectNode) getRootFolder() (*model.Folder, error) {
	tree, err := p.ofs.getProjectTree(p.project.ID)
	if err != nil {
		return nil, err
	}
	if len(tree.RootFolder) == 0 {
		return nil, fmt.Errorf("project has no root folder")
	}
	return &tree.RootFolder[0], nil
}

func (p *ProjectNode) Readdir(ctx context.Context) (gofuse.DirStream, syscall.Errno) {
	root, err := p.getRootFolder()
	if err != nil {
		log.Printf("get tree for %s: %v", p.project.Name, err)
		return nil, syscall.EIO
	}
	return gofuse.NewListDirStream(folderEntries(root)), 0
}

func (p *ProjectNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*gofuse.Inode, syscall.Errno) {
	if isSystemFile(name) {
		return nil, syscall.ENOENT
	}
	root, err := p.getRootFolder()
	if err != nil {
		return nil, syscall.EIO
	}
	return lookupInFolder(ctx, p, p.ofs, p.project.ID, root, name, out)
}

func (p *ProjectNode) Getattr(ctx context.Context, fh gofuse.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFDIR | 0755
	return 0
}

func (p *ProjectNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*gofuse.Inode, gofuse.FileHandle, uint32, syscall.Errno) {
	root, err := p.getRootFolder()
	if err != nil {
		return nil, nil, 0, syscall.EIO
	}
	return createFileInFolder(ctx, p, p.ofs, p.project.ID, root.ID, name, out)
}

func (p *ProjectNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*gofuse.Inode, syscall.Errno) {
	root, err := p.getRootFolder()
	if err != nil {
		return nil, syscall.EIO
	}
	return mkdirInFolder(ctx, p, p.ofs, p.project.ID, root.ID, name, out)
}

func (p *ProjectNode) Unlink(ctx context.Context, name string) syscall.Errno {
	root, err := p.getRootFolder()
	if err != nil {
		return syscall.EIO
	}
	return unlinkInFolder(p.ofs, p.project.ID, root, name)
}

func (p *ProjectNode) Rmdir(ctx context.Context, name string) syscall.Errno {
	root, err := p.getRootFolder()
	if err != nil {
		return syscall.EIO
	}
	return rmdirInFolder(p.ofs, p.project.ID, root, name)
}

func (p *ProjectNode) Rename(ctx context.Context, name string, newParent gofuse.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	root, err := p.getRootFolder()
	if err != nil {
		return syscall.EIO
	}
	return renameInFolder(p.ofs, p.project.ID, root, name, newParent, newName)
}

// ==========================================================================
// FolderNode
// ==========================================================================

type FolderNode struct {
	gofuse.Inode
	ofs       *OverleafFS
	projectID string
	folder    model.Folder
}

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
	out.Mode = syscall.S_IFDIR | 0755
	return 0
}

func (f *FolderNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*gofuse.Inode, gofuse.FileHandle, uint32, syscall.Errno) {
	return createFileInFolder(ctx, f, f.ofs, f.projectID, f.folder.ID, name, out)
}

func (f *FolderNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*gofuse.Inode, syscall.Errno) {
	return mkdirInFolder(ctx, f, f.ofs, f.projectID, f.folder.ID, name, out)
}

func (f *FolderNode) Unlink(ctx context.Context, name string) syscall.Errno {
	return unlinkInFolder(f.ofs, f.projectID, &f.folder, name)
}

func (f *FolderNode) Rmdir(ctx context.Context, name string) syscall.Errno {
	return rmdirInFolder(f.ofs, f.projectID, &f.folder, name)
}

func (f *FolderNode) Rename(ctx context.Context, name string, newParent gofuse.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	return renameInFolder(f.ofs, f.projectID, &f.folder, name, newParent, newName)
}

// ==========================================================================
// FileNode — supports read & write
// ==========================================================================

type FileNode struct {
	gofuse.Inode
	ofs       *OverleafFS
	projectID string
	folderID  string // parent folder ID for uploads
	id        string
	name      string
	isDoc     bool
	mu        sync.Mutex
}

func (f *FileNode) Getattr(ctx context.Context, fh gofuse.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFREG | 0644
	cacheKey := f.projectID + "/" + f.id
	if data, ok := f.ofs.Cache.Get(cacheKey); ok {
		out.Size = uint64(len(data))
	}
	return 0
}

func (f *FileNode) Setattr(ctx context.Context, fh gofuse.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	cacheKey := f.projectID + "/" + f.id
	if sz, ok := in.GetSize(); ok {
		// Truncate
		f.mu.Lock()
		defer f.mu.Unlock()
		data, _ := f.ofs.Cache.Get(cacheKey)
		if int(sz) < len(data) {
			data = data[:sz]
		} else {
			newData := make([]byte, sz)
			copy(newData, data)
			data = newData
		}
		f.ofs.Cache.SetDirty(cacheKey, data)
	}
	out.Mode = syscall.S_IFREG | 0644
	if data, ok := f.ofs.Cache.Get(cacheKey); ok {
		out.Size = uint64(len(data))
	}
	return 0
}

func (f *FileNode) Open(ctx context.Context, flags uint32) (gofuse.FileHandle, uint32, syscall.Errno) {
	cacheKey := f.projectID + "/" + f.id

	// If file has local dirty modifications, keep using cached version
	if f.ofs.Cache.IsDirty(cacheKey) {
		return nil, fuse.FOPEN_KEEP_CACHE, 0
	}

	// Always fetch latest from remote on open
	data, err := f.fetchContent()
	if err != nil {
		log.Printf("fetch %s: %v", f.name, err)
		// Fall back to cache if available
		if _, ok := f.ofs.Cache.Get(cacheKey); ok {
			return nil, fuse.FOPEN_KEEP_CACHE, 0
		}
		return nil, 0, syscall.EIO
	}
	f.ofs.Cache.Set(cacheKey, data)

	// Don't set FOPEN_KEEP_CACHE — let kernel re-read fresh content
	return nil, 0, 0
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
	end := min(int(off)+len(dest), len(data))
	if int(off) >= len(data) {
		return fuse.ReadResultData(nil), 0
	}
	return fuse.ReadResultData(data[off:end]), 0
}

func (f *FileNode) Write(ctx context.Context, fh gofuse.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()

	cacheKey := f.projectID + "/" + f.id
	existing, _ := f.ofs.Cache.Get(cacheKey)

	end := int(off) + len(data)
	if end > len(existing) {
		newBuf := make([]byte, end)
		copy(newBuf, existing)
		existing = newBuf
	}
	copy(existing[off:], data)

	f.ofs.Cache.SetDirty(cacheKey, existing)
	f.ofs.registerMeta(cacheKey, f.projectID, f.folderID, f.name)
	return uint32(len(data)), 0
}

func (f *FileNode) Flush(ctx context.Context, fh gofuse.FileHandle) syscall.Errno {
	cacheKey := f.projectID + "/" + f.id
	if !f.ofs.Cache.IsDirty(cacheKey) {
		return 0
	}

	// In batch mode, keep dirty — will be flushed by FlushAll
	if f.ofs.BatchMode {
		f.ofs.registerMeta(cacheKey, f.projectID, f.folderID, f.name)
		return 0
	}

	data, ok := f.ofs.Cache.Get(cacheKey)
	if !ok {
		return 0
	}

	log.Printf("flushing %s to Overleaf", f.name)

	if err := f.ofs.Client.UploadFile(f.projectID, f.folderID, f.name, data); err != nil {
		log.Printf("flush %s: %v", f.name, err)
		return syscall.EIO
	}

	f.ofs.Cache.ClearDirty(cacheKey)
	return 0
}

func (f *FileNode) fetchContent() ([]byte, error) {
	if f.isDoc {
		return f.ofs.getDocContent(f.projectID, f.id)
	}
	return f.ofs.Client.DownloadFile(f.projectID, f.id)
}

// ==========================================================================
// Write operation helpers (shared by ProjectNode and FolderNode)
// ==========================================================================

func createFileInFolder(ctx context.Context, parent gofuse.InodeEmbedder, ofs *OverleafFS, projectID, folderID, name string, out *fuse.EntryOut) (*gofuse.Inode, gofuse.FileHandle, uint32, syscall.Errno) {
	if isSystemFile(name) {
		return nil, nil, 0, syscall.EPERM
	}

	isDoc := isDocFile(name)
	var entityID string

	if isDoc {
		err := ofs.Client.CreateDoc(projectID, name, folderID)
		if err != nil {
			log.Printf("create doc %s: %v", name, err)
			return nil, nil, 0, syscall.EIO
		}
		// Re-fetch tree to get the new doc's ID
		ofs.invalidateTree(projectID)
		tree, err := ofs.getProjectTree(projectID)
		if err != nil {
			return nil, nil, 0, syscall.EIO
		}
		entityID = findDocID(tree, name)
	} else {
		// Upload an empty file
		err := ofs.Client.UploadFile(projectID, folderID, name, []byte{})
		if err != nil {
			log.Printf("create file %s: %v", name, err)
			return nil, nil, 0, syscall.EIO
		}
		ofs.invalidateTree(projectID)
		tree, err := ofs.getProjectTree(projectID)
		if err != nil {
			return nil, nil, 0, syscall.EIO
		}
		entityID = findFileRefID(tree, name)
	}

	if entityID == "" {
		entityID = "pending-" + name
	}

	node := &FileNode{
		ofs: ofs, projectID: projectID, folderID: folderID,
		id: entityID, name: name, isDoc: isDoc,
	}
	out.Mode = syscall.S_IFREG | 0644
	child := parent.EmbeddedInode().NewInode(ctx, node, gofuse.StableAttr{Mode: syscall.S_IFREG})

	// Initialize empty cache entry
	ofs.Cache.Set(projectID+"/"+entityID, []byte{})

	return child, nil, fuse.FOPEN_KEEP_CACHE, 0
}

func mkdirInFolder(ctx context.Context, parent gofuse.InodeEmbedder, ofs *OverleafFS, projectID, parentFolderID, name string, out *fuse.EntryOut) (*gofuse.Inode, syscall.Errno) {
	folder, err := ofs.Client.CreateFolder(projectID, name, parentFolderID)
	if err != nil {
		log.Printf("mkdir %s: %v", name, err)
		return nil, syscall.EIO
	}

	ofs.invalidateTree(projectID)

	node := &FolderNode{ofs: ofs, projectID: projectID, folder: *folder}
	out.Mode = syscall.S_IFDIR | 0755
	child := parent.EmbeddedInode().NewInode(ctx, node, gofuse.StableAttr{Mode: syscall.S_IFDIR})
	return child, 0
}

func unlinkInFolder(ofs *OverleafFS, projectID string, folder *model.Folder, name string) syscall.Errno {
	for _, doc := range folder.Docs {
		if sanitizeName(doc.Name) == name {
			if err := ofs.Client.DeleteEntity(projectID, "doc", doc.ID); err != nil {
				log.Printf("unlink doc %s: %v", name, err)
				return syscall.EIO
			}
			ofs.invalidateTree(projectID)
			ofs.Cache.Delete(projectID + "/" + doc.ID)
			return 0
		}
	}
	for _, ref := range folder.FileRefs {
		if sanitizeName(ref.Name) == name {
			if err := ofs.Client.DeleteEntity(projectID, "file", ref.ID); err != nil {
				log.Printf("unlink file %s: %v", name, err)
				return syscall.EIO
			}
			ofs.invalidateTree(projectID)
			ofs.Cache.Delete(projectID + "/" + ref.ID)
			return 0
		}
	}
	return syscall.ENOENT
}

func rmdirInFolder(ofs *OverleafFS, projectID string, folder *model.Folder, name string) syscall.Errno {
	for _, sub := range folder.Folders {
		if sanitizeName(sub.Name) == name {
			if err := ofs.Client.DeleteEntity(projectID, "folder", sub.ID); err != nil {
				log.Printf("rmdir %s: %v", name, err)
				return syscall.EIO
			}
			ofs.invalidateTree(projectID)
			return 0
		}
	}
	return syscall.ENOENT
}

func renameInFolder(ofs *OverleafFS, projectID string, folder *model.Folder, oldName string, newParent gofuse.InodeEmbedder, newName string) syscall.Errno {
	entityType, entityID := findEntity(folder, oldName)
	if entityID == "" {
		return syscall.ENOENT
	}

	// Determine destination folder ID
	var destFolderID string
	switch np := newParent.(type) {
	case *ProjectNode:
		tree, err := np.ofs.getProjectTree(np.project.ID)
		if err == nil && len(tree.RootFolder) > 0 {
			destFolderID = tree.RootFolder[0].ID
		}
	case *FolderNode:
		destFolderID = np.folder.ID
	}

	// Rename if name changed
	if oldName != newName {
		if err := ofs.Client.RenameEntity(projectID, entityType, entityID, newName); err != nil {
			log.Printf("rename %s -> %s: %v", oldName, newName, err)
			return syscall.EIO
		}
	}

	// Move if parent changed
	if destFolderID != "" && destFolderID != folder.ID {
		if err := ofs.Client.MoveEntity(projectID, entityType, entityID, destFolderID); err != nil {
			log.Printf("move %s: %v", oldName, err)
			return syscall.EIO
		}
	}

	ofs.invalidateTree(projectID)
	return 0
}

// ==========================================================================
// Helpers
// ==========================================================================

func folderEntries(folder *model.Folder) []fuse.DirEntry {
	var entries []fuse.DirEntry
	for _, sub := range folder.Folders {
		entries = append(entries, fuse.DirEntry{Name: sanitizeName(sub.Name), Mode: syscall.S_IFDIR})
	}
	for _, doc := range folder.Docs {
		entries = append(entries, fuse.DirEntry{Name: sanitizeName(doc.Name), Mode: syscall.S_IFREG})
	}
	for _, ref := range folder.FileRefs {
		entries = append(entries, fuse.DirEntry{Name: sanitizeName(ref.Name), Mode: syscall.S_IFREG})
	}
	return entries
}

func lookupInFolder(ctx context.Context, parent gofuse.InodeEmbedder, ofs *OverleafFS, projectID string, folder *model.Folder, name string, out *fuse.EntryOut) (*gofuse.Inode, syscall.Errno) {
	for _, sub := range folder.Folders {
		if sanitizeName(sub.Name) == name {
			node := &FolderNode{ofs: ofs, projectID: projectID, folder: sub}
			out.Mode = syscall.S_IFDIR | 0755
			child := parent.EmbeddedInode().NewInode(ctx, node, gofuse.StableAttr{Mode: syscall.S_IFDIR})
			return child, 0
		}
	}
	for _, doc := range folder.Docs {
		if sanitizeName(doc.Name) == name {
			folderID := folder.ID
			node := &FileNode{ofs: ofs, projectID: projectID, folderID: folderID, id: doc.ID, name: doc.Name, isDoc: true}
			out.Mode = syscall.S_IFREG | 0644
			child := parent.EmbeddedInode().NewInode(ctx, node, gofuse.StableAttr{Mode: syscall.S_IFREG})
			return child, 0
		}
	}
	for _, ref := range folder.FileRefs {
		if sanitizeName(ref.Name) == name {
			folderID := folder.ID
			node := &FileNode{ofs: ofs, projectID: projectID, folderID: folderID, id: ref.ID, name: ref.Name, isDoc: false}
			out.Mode = syscall.S_IFREG | 0644
			child := parent.EmbeddedInode().NewInode(ctx, node, gofuse.StableAttr{Mode: syscall.S_IFREG})
			return child, 0
		}
	}
	return nil, syscall.ENOENT
}

func findEntity(folder *model.Folder, name string) (entityType string, entityID string) {
	for _, doc := range folder.Docs {
		if sanitizeName(doc.Name) == name {
			return "doc", doc.ID
		}
	}
	for _, ref := range folder.FileRefs {
		if sanitizeName(ref.Name) == name {
			return "file", ref.ID
		}
	}
	for _, sub := range folder.Folders {
		if sanitizeName(sub.Name) == name {
			return "folder", sub.ID
		}
	}
	return "", ""
}

func findDocID(tree *model.ProjectMeta, name string) string {
	if len(tree.RootFolder) == 0 {
		return ""
	}
	return findDocIDInFolder(&tree.RootFolder[0], name)
}

func findDocIDInFolder(folder *model.Folder, name string) string {
	for _, doc := range folder.Docs {
		if doc.Name == name {
			return doc.ID
		}
	}
	for _, sub := range folder.Folders {
		if id := findDocIDInFolder(&sub, name); id != "" {
			return id
		}
	}
	return ""
}

func findFileRefID(tree *model.ProjectMeta, name string) string {
	if len(tree.RootFolder) == 0 {
		return ""
	}
	return findFileRefIDInFolder(&tree.RootFolder[0], name)
}

func findFileRefIDInFolder(folder *model.Folder, name string) string {
	for _, ref := range folder.FileRefs {
		if ref.Name == name {
			return ref.ID
		}
	}
	for _, sub := range folder.Folders {
		if id := findFileRefIDInFolder(&sub, name); id != "" {
			return id
		}
	}
	return ""
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

func isDocFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".tex", ".sty", ".cls", ".bst", ".bib", ".txt", ".md",
		".tikz", ".mtx", ".rtex", ".asy", ".lbx", ".bbx", ".cbx",
		".lco", ".dtx", ".ins", ".ist", ".def", ".clo", ".ldf",
		".rmd", ".lua", ".gv", ".mf", ".yml", ".yaml", ".cfg",
		".ltx", ".inc", ".csv":
		return true
	}
	return false
}

// FlushAll flushes all dirty cached files to Overleaf.
// Returns the number of files flushed and the number of errors.
func (o *OverleafFS) FlushAll() (flushed, errors int) {
	for _, key := range o.Cache.DirtyKeys() {
		data, ok := o.Cache.Get(key)
		if !ok {
			continue
		}
		meta, hasMeta := o.getMeta(key)
		if !hasMeta {
			log.Printf("flush-all: no metadata for %s, skipping", key)
			errors++
			continue
		}
		log.Printf("flushing %s (%s)", meta.name, key)
		if err := o.Client.UploadFile(meta.projectID, meta.folderID, meta.name, data); err != nil {
			log.Printf("flush-all %s: %v", meta.name, err)
			errors++
		} else {
			o.Cache.ClearDirty(key)
			flushed++
		}
	}
	return
}

// DisconnectAll disconnects all Socket.IO connections.
func (o *OverleafFS) DisconnectAll() {
	o.sioMu.Lock()
	defer o.sioMu.Unlock()
	for id, sio := range o.sioConn {
		sio.Disconnect()
		delete(o.sioConn, id)
	}
}

// MountResult holds the FUSE server and filesystem state for clean shutdown.
type MountResult struct {
	Server *fuse.Server
	OFS    *OverleafFS
}

// PIDFile is the path where the mount process writes its PID for sync.
const PIDFile = "/tmp/downleaf.pid"

// Mount mounts the Overleaf filesystem at the given mountpoint.
func Mount(mountpoint string, client *api.Client, projectFilter string, batchMode bool) (*MountResult, error) {
	if err := os.MkdirAll(mountpoint, 0755); err != nil {
		return nil, fmt.Errorf("create mountpoint: %w", err)
	}

	ofs := NewOverleafFS(client)
	ofs.projectFilter = projectFilter
	ofs.BatchMode = batchMode
	root := &RootNode{ofs: ofs}

	server, err := gofuse.Mount(mountpoint, root, &gofuse.Options{
		MountOptions: fuse.MountOptions{
			FsName: "downleaf",
			Name:   "overleaf",
		},
		FirstAutomaticIno: 1,
		EntryTimeout:      &entryTimeout,
		AttrTimeout:       &attrTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("mount: %w", err)
	}

	log.Printf("Mounted at %s", mountpoint)
	log.Printf("Use 'umount %s' or press Ctrl+C to unmount", mountpoint)

	return &MountResult{Server: server, OFS: ofs}, nil
}

var (
	entryTimeout = 5 * time.Second
	attrTimeout  = 5 * time.Second
)

// Unmount unmounts the filesystem using the system umount command.
func Unmount(mountpoint string) error {
	return exec.Command("umount", mountpoint).Run()
}
