package webdav

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	gowebdav "golang.org/x/net/webdav"

	"github.com/FanBB2333/downleaf/internal/api"
	"github.com/FanBB2333/downleaf/internal/cache"
	"github.com/FanBB2333/downleaf/internal/model"
)

// fileMeta stores metadata needed to upload a dirty file back to Overleaf.
type fileMeta struct {
	projectID string
	folderID  string
	name      string
}

// OverleafFS implements gowebdav.FileSystem backed by Overleaf.
type OverleafFS struct {
	Client        *api.Client
	Cache         *cache.Cache
	BatchMode     bool
	ProjectFilter string

	projectsMu sync.RWMutex
	projects   []model.Project

	sioMu   sync.Mutex
	sioConn map[string]*api.SocketIOClient
	trees   map[string]*model.ProjectMeta

	metaMu  sync.RWMutex
	metaMap map[string]fileMeta
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

func (o *OverleafFS) refreshProjects() error {
	o.projectsMu.Lock()
	defer o.projectsMu.Unlock()
	projects, err := o.Client.ListProjects()
	if err != nil {
		return err
	}
	o.projects = projects
	return nil
}

func (o *OverleafFS) getActiveProjects() []model.Project {
	o.projectsMu.RLock()
	defer o.projectsMu.RUnlock()
	var result []model.Project
	for _, p := range o.projects {
		if p.Archived || p.Trashed {
			continue
		}
		if o.ProjectFilter != "" && p.ID != o.ProjectFilter && !strings.EqualFold(p.Name, o.ProjectFilter) {
			continue
		}
		result = append(result, p)
	}
	return result
}

func (o *OverleafFS) findProjectByName(name string) (model.Project, bool) {
	o.projectsMu.RLock()
	defer o.projectsMu.RUnlock()
	for _, p := range o.projects {
		if p.Archived || p.Trashed {
			continue
		}
		if o.ProjectFilter != "" && p.ID != o.ProjectFilter && !strings.EqualFold(p.Name, o.ProjectFilter) {
			continue
		}
		if sanitizeName(p.Name) == name {
			return p, true
		}
	}
	return model.Project{}, false
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

// FlushAll uploads all dirty cached files to Overleaf.
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

// ==========================================================================
// Path resolution
// ==========================================================================

type pathInfo struct {
	isRoot       bool
	project      model.Project
	folder       *model.Folder
	doc          *model.Doc
	fileRef      *model.FileRef
	parentFolder *model.Folder
}

func (pi *pathInfo) isDir() bool {
	return pi.isRoot || pi.folder != nil
}

func (pi *pathInfo) entityID() string {
	if pi.doc != nil {
		return pi.doc.ID
	}
	if pi.fileRef != nil {
		return pi.fileRef.ID
	}
	if pi.folder != nil {
		return pi.folder.ID
	}
	return ""
}

func (pi *pathInfo) entityName() string {
	if pi.doc != nil {
		return pi.doc.Name
	}
	if pi.fileRef != nil {
		return pi.fileRef.Name
	}
	if pi.folder != nil {
		return pi.folder.Name
	}
	return ""
}

func (pi *pathInfo) entityType() string {
	if pi.doc != nil {
		return "doc"
	}
	if pi.fileRef != nil {
		return "file"
	}
	if pi.folder != nil {
		return "folder"
	}
	return ""
}

func (o *OverleafFS) resolve(name string) (*pathInfo, error) {
	name = path.Clean(name)
	name = strings.TrimPrefix(name, "/")

	if name == "" || name == "." {
		return &pathInfo{isRoot: true}, nil
	}

	// Ensure projects are loaded
	o.projectsMu.RLock()
	loaded := len(o.projects) > 0
	o.projectsMu.RUnlock()
	if !loaded {
		if err := o.refreshProjects(); err != nil {
			return nil, err
		}
	}

	parts := strings.Split(name, "/")

	project, ok := o.findProjectByName(parts[0])
	if !ok {
		return nil, os.ErrNotExist
	}

	tree, err := o.getProjectTree(project.ID)
	if err != nil {
		return nil, err
	}
	if len(tree.RootFolder) == 0 {
		return nil, os.ErrNotExist
	}

	// Just the project root
	if len(parts) == 1 {
		return &pathInfo{project: project, folder: &tree.RootFolder[0]}, nil
	}

	// Navigate into the project tree
	folder := &tree.RootFolder[0]
	for i := 1; i < len(parts); i++ {
		target := parts[i]
		isLast := i == len(parts)-1

		// Look for subfolder
		found := false
		for j := range folder.Folders {
			if sanitizeName(folder.Folders[j].Name) == target {
				if isLast {
					return &pathInfo{project: project, folder: &folder.Folders[j], parentFolder: folder}, nil
				}
				folder = &folder.Folders[j]
				found = true
				break
			}
		}
		if found {
			continue
		}

		if !isLast {
			return nil, os.ErrNotExist
		}

		// Look for doc
		for j := range folder.Docs {
			if sanitizeName(folder.Docs[j].Name) == target {
				return &pathInfo{project: project, doc: &folder.Docs[j], parentFolder: folder}, nil
			}
		}

		// Look for file ref
		for j := range folder.FileRefs {
			if sanitizeName(folder.FileRefs[j].Name) == target {
				return &pathInfo{project: project, fileRef: &folder.FileRefs[j], parentFolder: folder}, nil
			}
		}

		return nil, os.ErrNotExist
	}

	return nil, os.ErrNotExist
}

// ==========================================================================
// webdav.FileSystem implementation
// ==========================================================================

var _ gowebdav.FileSystem = (*OverleafFS)(nil)

func (o *OverleafFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	name = path.Clean(name)
	dir := path.Dir(name)
	base := path.Base(name)

	parent, err := o.resolve(dir)
	if err != nil {
		return err
	}
	if !parent.isDir() || parent.isRoot {
		return os.ErrPermission
	}

	_, err = o.Client.CreateFolder(parent.project.ID, base, parent.folder.ID)
	if err != nil {
		return err
	}
	o.invalidateTree(parent.project.ID)
	return nil
}

func (o *OverleafFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (gowebdav.File, error) {
	name = path.Clean(name)

	if flag&os.O_CREATE != 0 {
		info, err := o.resolve(name)
		if err == os.ErrNotExist {
			return o.createFile(name)
		}
		if err != nil {
			return nil, err
		}
		if flag&os.O_TRUNC != 0 && !info.isDir() {
			return o.openFileForWrite(info, true)
		}
		return o.openExisting(info, flag)
	}

	info, err := o.resolve(name)
	if err != nil {
		return nil, err
	}
	return o.openExisting(info, flag)
}

func (o *OverleafFS) openExisting(info *pathInfo, flag int) (gowebdav.File, error) {
	if info.isDir() {
		return o.openDir(info)
	}
	if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		return o.openFileForWrite(info, flag&os.O_TRUNC != 0)
	}
	return o.openFileForRead(info)
}

func (o *OverleafFS) openDir(info *pathInfo) (gowebdav.File, error) {
	if info.isRoot {
		if err := o.refreshProjects(); err != nil {
			return nil, err
		}
		var entries []fs.FileInfo
		for _, p := range o.getActiveProjects() {
			entries = append(entries, &fileInfo{
				name: sanitizeName(p.Name), dir: true, modTime: time.Now(),
			})
		}
		return &dirFile{
			info:    &fileInfo{name: "/", dir: true, modTime: time.Now()},
			entries: entries,
		}, nil
	}

	folder := info.folder
	var entries []fs.FileInfo
	for _, sub := range folder.Folders {
		entries = append(entries, &fileInfo{
			name: sanitizeName(sub.Name), dir: true, modTime: time.Now(),
		})
	}
	for _, doc := range folder.Docs {
		var size int64
		if data, ok := o.Cache.Get(info.project.ID + "/" + doc.ID); ok {
			size = int64(len(data))
		}
		entries = append(entries, &fileInfo{
			name: sanitizeName(doc.Name), size: size, modTime: time.Now(),
		})
	}
	for _, ref := range folder.FileRefs {
		var size int64
		if data, ok := o.Cache.Get(info.project.ID + "/" + ref.ID); ok {
			size = int64(len(data))
		}
		entries = append(entries, &fileInfo{
			name: sanitizeName(ref.Name), size: size, modTime: ref.Created,
		})
	}

	return &dirFile{
		info:    &fileInfo{name: sanitizeName(info.entityName()), dir: true, modTime: time.Now()},
		entries: entries,
	}, nil
}

func (o *OverleafFS) openFileForRead(info *pathInfo) (gowebdav.File, error) {
	cacheKey := info.project.ID + "/" + info.entityID()

	// Always fetch fresh unless dirty
	if !o.Cache.IsDirty(cacheKey) {
		var data []byte
		var err error
		if info.doc != nil {
			data, err = o.getDocContent(info.project.ID, info.doc.ID)
		} else {
			data, err = o.Client.DownloadFile(info.project.ID, info.fileRef.ID)
		}
		if err != nil {
			if cached, ok := o.Cache.Get(cacheKey); ok {
				data = cached
			} else {
				return nil, err
			}
		} else {
			o.Cache.Set(cacheKey, data)
		}
	}

	data, _ := o.Cache.Get(cacheKey)

	folderID := ""
	if info.parentFolder != nil {
		folderID = info.parentFolder.ID
	}

	return &regularFile{
		ofs:       o,
		info:      &fileInfo{name: sanitizeName(info.entityName()), size: int64(len(data)), modTime: time.Now()},
		projectID: info.project.ID,
		folderID:  folderID,
		entityID:  info.entityID(),
		name:      info.entityName(),
		isDoc:     info.doc != nil,
		content:   data,
	}, nil
}

func (o *OverleafFS) openFileForWrite(info *pathInfo, truncate bool) (gowebdav.File, error) {
	cacheKey := info.project.ID + "/" + info.entityID()

	var content []byte
	if !truncate {
		if cached, ok := o.Cache.Get(cacheKey); ok {
			content = make([]byte, len(cached))
			copy(content, cached)
		}
	}

	folderID := ""
	if info.parentFolder != nil {
		folderID = info.parentFolder.ID
	}

	return &regularFile{
		ofs:       o,
		info:      &fileInfo{name: sanitizeName(info.entityName()), size: int64(len(content)), modTime: time.Now()},
		projectID: info.project.ID,
		folderID:  folderID,
		entityID:  info.entityID(),
		name:      info.entityName(),
		isDoc:     info.doc != nil,
		content:   content,
		writable:  true,
	}, nil
}

func (o *OverleafFS) createFile(name string) (gowebdav.File, error) {
	dir := path.Dir(name)
	base := path.Base(name)

	parent, err := o.resolve(dir)
	if err != nil {
		return nil, err
	}
	if !parent.isDir() || parent.isRoot {
		return nil, os.ErrPermission
	}

	isDoc := isDocFile(base)
	var entityID string

	if isDoc {
		if err := o.Client.CreateDoc(parent.project.ID, base, parent.folder.ID); err != nil {
			return nil, err
		}
		o.invalidateTree(parent.project.ID)
		tree, err := o.getProjectTree(parent.project.ID)
		if err != nil {
			return nil, err
		}
		entityID = findDocID(tree, base)
	} else {
		if err := o.Client.UploadFile(parent.project.ID, parent.folder.ID, base, []byte{}); err != nil {
			return nil, err
		}
		o.invalidateTree(parent.project.ID)
		tree, err := o.getProjectTree(parent.project.ID)
		if err != nil {
			return nil, err
		}
		entityID = findFileRefID(tree, base)
	}

	if entityID == "" {
		entityID = "pending-" + base
	}

	o.Cache.Set(parent.project.ID+"/"+entityID, []byte{})

	return &regularFile{
		ofs:       o,
		info:      &fileInfo{name: base, modTime: time.Now()},
		projectID: parent.project.ID,
		folderID:  parent.folder.ID,
		entityID:  entityID,
		name:      base,
		isDoc:     isDoc,
		content:   []byte{},
		writable:  true,
	}, nil
}

func (o *OverleafFS) RemoveAll(ctx context.Context, name string) error {
	info, err := o.resolve(name)
	if err != nil {
		return err
	}
	if info.isRoot {
		return os.ErrPermission
	}

	eType := info.entityType()
	eID := info.entityID()
	if eType == "" || eID == "" {
		return os.ErrNotExist
	}

	if err := o.Client.DeleteEntity(info.project.ID, eType, eID); err != nil {
		return err
	}
	o.invalidateTree(info.project.ID)
	o.Cache.Delete(info.project.ID + "/" + eID)
	return nil
}

func (o *OverleafFS) Rename(ctx context.Context, oldName, newName string) error {
	oldName = path.Clean(oldName)
	newName = path.Clean(newName)

	info, err := o.resolve(oldName)
	if err != nil {
		return err
	}
	if info.isRoot {
		return os.ErrPermission
	}

	eType := info.entityType()
	eID := info.entityID()
	oldBase := path.Base(oldName)
	newBase := path.Base(newName)
	oldDir := path.Dir(oldName)
	newDir := path.Dir(newName)

	if oldBase != newBase {
		if err := o.Client.RenameEntity(info.project.ID, eType, eID, newBase); err != nil {
			return err
		}
	}

	if oldDir != newDir {
		destParent, err := o.resolve(newDir)
		if err != nil {
			return err
		}
		if destParent.folder == nil {
			return os.ErrInvalid
		}
		if err := o.Client.MoveEntity(info.project.ID, eType, eID, destParent.folder.ID); err != nil {
			return err
		}
	}

	o.invalidateTree(info.project.ID)
	return nil
}

func (o *OverleafFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	info, err := o.resolve(name)
	if err != nil {
		return nil, err
	}

	if info.isRoot {
		return &fileInfo{name: "/", dir: true, modTime: time.Now()}, nil
	}

	if info.isDir() {
		return &fileInfo{
			name: sanitizeName(info.entityName()), dir: true, modTime: time.Now(),
		}, nil
	}

	var size int64
	cacheKey := info.project.ID + "/" + info.entityID()
	if data, ok := o.Cache.Get(cacheKey); ok {
		size = int64(len(data))
	}

	modTime := time.Now()
	if info.fileRef != nil && !info.fileRef.Created.IsZero() {
		modTime = info.fileRef.Created
	}

	return &fileInfo{
		name: sanitizeName(info.entityName()), size: size, modTime: modTime,
	}, nil
}

// ==========================================================================
// fileInfo implements os.FileInfo
// ==========================================================================

type fileInfo struct {
	name    string
	size    int64
	dir     bool
	modTime time.Time
}

func (fi *fileInfo) Name() string      { return fi.name }
func (fi *fileInfo) Size() int64       { return fi.size }
func (fi *fileInfo) ModTime() time.Time { return fi.modTime }
func (fi *fileInfo) IsDir() bool       { return fi.dir }
func (fi *fileInfo) Sys() any          { return nil }
func (fi *fileInfo) Mode() os.FileMode {
	if fi.dir {
		return 0755 | os.ModeDir
	}
	return 0644
}

// ==========================================================================
// dirFile implements gowebdav.File for directories
// ==========================================================================

type dirFile struct {
	info    *fileInfo
	entries []fs.FileInfo
	pos     int
}

func (d *dirFile) Close() error                             { return nil }
func (d *dirFile) Read([]byte) (int, error)                 { return 0, os.ErrInvalid }
func (d *dirFile) Write([]byte) (int, error)                { return 0, os.ErrInvalid }
func (d *dirFile) Stat() (fs.FileInfo, error)               { return d.info, nil }
func (d *dirFile) Seek(offset int64, whence int) (int64, error) {
	if offset == 0 && whence == io.SeekStart {
		d.pos = 0
		return 0, nil
	}
	return 0, os.ErrInvalid
}

func (d *dirFile) Readdir(count int) ([]fs.FileInfo, error) {
	if count <= 0 {
		entries := d.entries[d.pos:]
		d.pos = len(d.entries)
		return entries, nil
	}
	end := min(d.pos+count, len(d.entries))
	entries := d.entries[d.pos:end]
	d.pos = end
	if d.pos >= len(d.entries) {
		return entries, io.EOF
	}
	return entries, nil
}

// ==========================================================================
// regularFile implements gowebdav.File for files
// ==========================================================================

type regularFile struct {
	ofs       *OverleafFS
	info      *fileInfo
	projectID string
	folderID  string
	entityID  string
	name      string
	isDoc     bool
	content   []byte
	readPos   int64
	writable  bool
	dirty     bool
}

func (f *regularFile) Stat() (fs.FileInfo, error)               { return f.info, nil }
func (f *regularFile) Readdir(int) ([]fs.FileInfo, error)       { return nil, os.ErrInvalid }

func (f *regularFile) Read(p []byte) (int, error) {
	if f.readPos >= int64(len(f.content)) {
		return 0, io.EOF
	}
	n := copy(p, f.content[f.readPos:])
	f.readPos += int64(n)
	return n, nil
}

func (f *regularFile) Seek(offset int64, whence int) (int64, error) {
	var pos int64
	switch whence {
	case io.SeekStart:
		pos = offset
	case io.SeekCurrent:
		pos = f.readPos + offset
	case io.SeekEnd:
		pos = int64(len(f.content)) + offset
	default:
		return 0, os.ErrInvalid
	}
	if pos < 0 {
		return 0, os.ErrInvalid
	}
	f.readPos = pos
	return pos, nil
}

func (f *regularFile) Write(p []byte) (int, error) {
	if !f.writable {
		return 0, os.ErrPermission
	}
	f.content = append(f.content, p...)
	f.info.size = int64(len(f.content))
	f.dirty = true
	return len(p), nil
}

func (f *regularFile) Close() error {
	if !f.dirty {
		return nil
	}

	cacheKey := f.projectID + "/" + f.entityID
	f.ofs.Cache.SetDirty(cacheKey, f.content)
	f.ofs.registerMeta(cacheKey, f.projectID, f.folderID, f.name)

	if f.ofs.BatchMode {
		return nil
	}

	log.Printf("uploading %s to Overleaf", f.name)
	if err := f.ofs.Client.UploadFile(f.projectID, f.folderID, f.name, f.content); err != nil {
		log.Printf("upload %s: %v", f.name, err)
		return err
	}
	f.ofs.Cache.ClearDirty(cacheKey)
	return nil
}

// ==========================================================================
// Helpers
// ==========================================================================

func sanitizeName(name string) string {
	return strings.ReplaceAll(name, "/", "_")
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

// ==========================================================================
// Server
// ==========================================================================

// PIDFile is the path where the serving process writes its PID for sync.
const PIDFile = "/tmp/downleaf.pid"

// Serve starts the WebDAV server on the given address.
func Serve(addr string, ofs *OverleafFS) error {
	handler := &gowebdav.Handler{
		FileSystem: ofs,
		LockSystem: gowebdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				log.Printf("WebDAV %s %s: %v", r.Method, r.URL.Path, err)
			}
		},
	}

	log.Printf("WebDAV server listening on %s", addr)
	return http.ListenAndServe(addr, handler)
}

// MountNative attempts to mount the WebDAV server using OS-native commands.
func MountNative(addr, mountpoint string) error {
	if err := os.MkdirAll(mountpoint, 0755); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("mount_webdav", "-S", "-v", "downleaf",
			"http://"+addr, mountpoint).Run()
	case "linux":
		return exec.Command("mount", "-t", "davfs",
			"http://"+addr, mountpoint).Run()
	default:
		log.Printf("Auto-mount not supported on %s. Access via http://%s", runtime.GOOS, addr)
		return nil
	}
}

// Unmount unmounts the filesystem.
func Unmount(mountpoint string) error {
	return exec.Command("umount", mountpoint).Run()
}
