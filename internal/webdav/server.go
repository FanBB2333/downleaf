package webdav

import (
	"context"
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
	"golang.org/x/sync/singleflight"

	"github.com/FanBB2333/downleaf/internal/api"
	"github.com/FanBB2333/downleaf/internal/cache"
	"github.com/FanBB2333/downleaf/internal/ignore"
	"github.com/FanBB2333/downleaf/internal/model"
)

// fileMeta stores metadata needed to upload a dirty file back to Overleaf.
type fileMeta struct {
	projectID string
	folderID  string
	name      string
}

// resolveEntry caches the result of a path resolution (including negative results).
type resolveEntry struct {
	info      *pathInfo
	err       error
	fetchedAt time.Time
}

type treeEntry struct {
	folder       *model.Folder
	doc          *model.Doc
	fileRef      *model.FileRef
	parentFolder *model.Folder
}

type projectTreeState struct {
	meta    *model.ProjectMeta
	root    *model.Folder
	entries map[string]treeEntry
}

// OverleafFS implements gowebdav.FileSystem backed by Overleaf.
type OverleafFS struct {
	Client         *api.Client
	Cache          *cache.Cache
	ZenMode        bool
	ProjectFilters []string
	Ignore         *ignore.Matcher

	projectsMu      sync.RWMutex
	projects        []model.Project
	projectsFetched time.Time

	sioMu   sync.Mutex
	sioConn map[string]*api.SocketIOClient
	trees   map[string]*projectTreeState

	metaMu  sync.RWMutex
	metaMap map[string]fileMeta

	resolveMu    sync.RWMutex
	resolveCache map[string]*resolveEntry

	treeFlight    singleflight.Group
	projectFlight singleflight.Group
}

const (
	resolveTTL    = 2 * time.Minute
	resolveNegTTL = 5 * time.Second
	projectsTTL   = 5 * time.Minute
)

func NewOverleafFS(client *api.Client) *OverleafFS {
	return &OverleafFS{
		Client:       client,
		Cache:        cache.New(5 * time.Minute),
		sioConn:      make(map[string]*api.SocketIOClient),
		trees:        make(map[string]*projectTreeState),
		metaMap:      make(map[string]fileMeta),
		resolveCache: make(map[string]*resolveEntry),
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
	o.projectsFetched = time.Now()
	return nil
}

func (o *OverleafFS) refreshProjectsIfStale() error {
	o.projectsMu.RLock()
	hasProjects := len(o.projects) > 0
	fresh := hasProjects && (o.ZenMode || time.Since(o.projectsFetched) < projectsTTL)
	o.projectsMu.RUnlock()
	if fresh {
		return nil
	}
	_, err, _ := o.projectFlight.Do("projects", func() (any, error) {
		return nil, o.refreshProjects()
	})
	return err
}

// resolveWithCache wraps resolve() with a TTL cache for both positive and negative results.
// Positive results (file/dir found) use resolveTTL; negative results (not found) use the
// shorter resolveNegTTL so that newly created files appear quickly.
func (o *OverleafFS) resolveWithCache(name string) (*pathInfo, error) {
	key := path.Clean(strings.TrimPrefix(name, "/"))

	o.resolveMu.RLock()
	if entry, ok := o.resolveCache[key]; ok {
		// In zen mode, positive resolve results never expire
		if o.ZenMode && entry.err == nil {
			o.resolveMu.RUnlock()
			return entry.info, nil
		}
		ttl := resolveTTL
		if entry.err != nil {
			ttl = resolveNegTTL
		}
		if time.Since(entry.fetchedAt) < ttl {
			o.resolveMu.RUnlock()
			return entry.info, entry.err
		}
	}
	o.resolveMu.RUnlock()

	info, err := o.resolve(name)

	o.resolveMu.Lock()
	o.resolveCache[key] = &resolveEntry{info: info, err: err, fetchedAt: time.Now()}
	o.resolveMu.Unlock()

	return info, err
}

// invalidateResolveCache clears all resolve cache entries for a given project name prefix.
func (o *OverleafFS) invalidateResolveCache(prefix string) {
	o.resolveMu.Lock()
	for k := range o.resolveCache {
		if strings.HasPrefix(k, prefix) || k == prefix {
			delete(o.resolveCache, k)
		}
	}
	o.resolveMu.Unlock()
}

func (o *OverleafFS) isProjectAllowed(p model.Project) bool {
	if len(o.ProjectFilters) == 0 {
		return true
	}
	for _, filter := range o.ProjectFilters {
		if p.ID == filter || strings.EqualFold(p.Name, filter) {
			return true
		}
	}
	return false
}

func (o *OverleafFS) getActiveProjects() []model.Project {
	o.projectsMu.RLock()
	defer o.projectsMu.RUnlock()
	var result []model.Project
	for _, p := range o.projects {
		if p.Archived || p.Trashed {
			continue
		}
		if !o.isProjectAllowed(p) {
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
		if !o.isProjectAllowed(p) {
			continue
		}
		if sanitizeName(p.Name) == name {
			return p, true
		}
	}
	return model.Project{}, false
}

func buildProjectTreeState(meta *model.ProjectMeta) *projectTreeState {
	state := &projectTreeState{
		meta:    meta,
		entries: make(map[string]treeEntry),
	}
	if meta == nil || len(meta.RootFolder) == 0 {
		return state
	}

	state.root = &meta.RootFolder[0]
	state.entries[""] = treeEntry{folder: state.root}
	indexFolderEntries(state, state.root, "")
	return state
}

func indexFolderEntries(state *projectTreeState, folder *model.Folder, prefix string) {
	for i := range folder.Folders {
		child := &folder.Folders[i]
		key := joinTreePath(prefix, sanitizeName(child.Name))
		state.entries[key] = treeEntry{
			folder:       child,
			parentFolder: folder,
		}
		indexFolderEntries(state, child, key)
	}

	for i := range folder.Docs {
		doc := &folder.Docs[i]
		key := joinTreePath(prefix, sanitizeName(doc.Name))
		state.entries[key] = treeEntry{
			doc:          doc,
			parentFolder: folder,
		}
	}

	for i := range folder.FileRefs {
		ref := &folder.FileRefs[i]
		key := joinTreePath(prefix, sanitizeName(ref.Name))
		state.entries[key] = treeEntry{
			fileRef:      ref,
			parentFolder: folder,
		}
	}
}

func joinTreePath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

func (o *OverleafFS) cacheProjectTree(projectID string, state *projectTreeState) *projectTreeState {
	o.sioMu.Lock()
	defer o.sioMu.Unlock()
	if existing, ok := o.trees[projectID]; ok {
		return existing
	}
	o.trees[projectID] = state
	return state
}

func (o *OverleafFS) getProjectTree(projectID string) (*projectTreeState, error) {
	o.sioMu.Lock()
	if tree, ok := o.trees[projectID]; ok {
		o.sioMu.Unlock()
		return tree, nil
	}
	o.sioMu.Unlock()

	// singleflight ensures that concurrent PROPFIND requests for the same
	// project only trigger one HTTP/socket call; the rest wait and share the result.
	val, err, _ := o.treeFlight.Do(projectID, func() (any, error) {
		// Double-check after winning the flight — another goroutine may have
		// populated the cache while we were waiting.
		o.sioMu.Lock()
		if tree, ok := o.trees[projectID]; ok {
			o.sioMu.Unlock()
			return tree, nil
		}
		o.sioMu.Unlock()

		// Prefer the editor page metadata for path lookup. This avoids opening a
		// Socket.IO session for metadata-only requests such as stat/cd/autocomplete.
		if detail, err := o.Client.GetProjectDetail(projectID); err == nil {
			if len(detail.Project.RootFolder) > 0 {
				return o.cacheProjectTree(projectID, buildProjectTreeState(&detail.Project)), nil
			}
		} else {
			log.Printf("warning: could not refresh CSRF token for project %s: %v", projectID, err)
		}

		sio := api.NewSocketIOClient(o.Client.SiteURL, o.Client.Identity)
		tree, err := sio.JoinProject(projectID)
		if err != nil {
			return nil, err
		}

		state := buildProjectTreeState(tree)

		o.sioMu.Lock()
		if _, ok := o.sioConn[projectID]; ok {
			o.sioMu.Unlock()
			sio.Disconnect()
			return o.cacheProjectTree(projectID, state), nil
		}
		o.sioConn[projectID] = sio
		o.trees[projectID] = state
		o.sioMu.Unlock()
		return state, nil
	})
	if err != nil {
		return nil, err
	}
	return val.(*projectTreeState), nil
}

func (o *OverleafFS) ensureProjectSocket(projectID string) (*api.SocketIOClient, error) {
	o.sioMu.Lock()
	if sio, ok := o.sioConn[projectID]; ok {
		o.sioMu.Unlock()
		return sio, nil
	}
	o.sioMu.Unlock()

	// Refresh the project page before opening a Socket.IO session so writes keep
	// using the most recent CSRF token for this project.
	if _, err := o.Client.GetProjectDetail(projectID); err != nil {
		log.Printf("warning: could not refresh CSRF token for project %s: %v", projectID, err)
	}

	sio := api.NewSocketIOClient(o.Client.SiteURL, o.Client.Identity)
	tree, err := sio.JoinProject(projectID)
	if err != nil {
		return nil, err
	}
	state := buildProjectTreeState(tree)

	o.sioMu.Lock()
	if existing, ok := o.sioConn[projectID]; ok {
		o.sioMu.Unlock()
		sio.Disconnect()
		return existing, nil
	}
	o.sioConn[projectID] = sio
	o.trees[projectID] = state
	o.sioMu.Unlock()
	return sio, nil
}

func (o *OverleafFS) invalidateTree(projectID string) {
	o.sioMu.Lock()
	delete(o.trees, projectID)
	if sio, ok := o.sioConn[projectID]; ok {
		sio.Disconnect()
		delete(o.sioConn, projectID)
	}
	o.sioMu.Unlock()
	// Also clear resolve cache — find the project name for this ID
	o.projectsMu.RLock()
	for _, p := range o.projects {
		if p.ID == projectID {
			o.invalidateResolveCache(sanitizeName(p.Name))
			break
		}
	}
	o.projectsMu.RUnlock()
}

func (o *OverleafFS) getDocContent(projectID, docID string) ([]byte, error) {
	sio, err := o.ensureProjectSocket(projectID)
	if err != nil {
		return nil, err
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

// FileStat describes a single dirty file for summary display.
type FileStat struct {
	Name  string
	Lines int
	Bytes int
}

// DirtySummary returns stats for all dirty (locally modified) files.
func (o *OverleafFS) DirtySummary() []FileStat {
	var stats []FileStat
	for _, key := range o.Cache.DirtyKeys() {
		data, ok := o.Cache.Get(key)
		if !ok {
			continue
		}
		meta, hasMeta := o.getMeta(key)
		if !hasMeta {
			continue
		}
		lines := 0
		if len(data) > 0 {
			lines = 1
			for _, b := range data {
				if b == '\n' {
					lines++
				}
			}
		}
		stats = append(stats, FileStat{
			Name:  meta.name,
			Lines: lines,
			Bytes: len(data),
		})
	}
	return stats
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
		if o.isIgnored(meta.name, false) {
			log.Printf("flush-all: %s ignored by .dlignore, skipping", meta.name)
			o.Cache.ClearDirty(key)
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
	if tree.root == nil {
		return nil, os.ErrNotExist
	}

	// Just the project root
	if len(parts) == 1 {
		return &pathInfo{project: project, folder: tree.root}, nil
	}

	entry, ok := tree.entries[strings.Join(parts[1:], "/")]
	if !ok {
		return nil, os.ErrNotExist
	}

	return &pathInfo{
		project:      project,
		folder:       entry.folder,
		doc:          entry.doc,
		fileRef:      entry.fileRef,
		parentFolder: entry.parentFolder,
	}, nil
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
		info, err := o.resolveWithCache(name)
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

	info, err := o.resolveWithCache(name)
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
		if err := o.refreshProjectsIfStale(); err != nil {
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

	// Use cache if data is fresh or dirty; only fetch when cache misses or expires
	if data, ok := o.Cache.Get(cacheKey); ok {
		// Cache hit (within TTL or dirty) — use cached data
		_ = data
	} else {
		// Cache miss or expired — fetch from remote
		var data []byte
		var err error
		if info.doc != nil {
			data, err = o.getDocContent(info.project.ID, info.doc.ID)
		} else {
			data, err = o.Client.DownloadFile(info.project.ID, info.fileRef.ID)
		}
		if err != nil {
			return nil, err
		}
		o.Cache.Set(cacheKey, data)
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

func (o *OverleafFS) statTempFile(name string) (os.FileInfo, error) {
	parts := strings.Split(strings.TrimPrefix(path.Clean(name), "/"), "/")
	if len(parts) < 2 {
		return nil, os.ErrNotExist
	}
	project, ok := o.findProjectByName(parts[0])
	if !ok {
		return nil, os.ErrNotExist
	}
	base := path.Base(name)
	cacheKey := project.ID + "/tmp-" + base
	data, ok := o.Cache.Get(cacheKey)
	if !ok {
		return nil, os.ErrNotExist
	}
	return &fileInfo{name: base, size: int64(len(data)), modTime: time.Now()}, nil
}

// isTempFile returns true for editor temp files (atomic save pattern).
func isTempFile(name string) bool {
	// Patterns: file.ext.tmp.PID.TIMESTAMP, .file.swp, file~, #file#
	base := path.Base(name)
	if strings.Contains(base, ".tmp.") {
		return true
	}
	if strings.HasSuffix(base, "~") || strings.HasSuffix(base, ".swp") || strings.HasSuffix(base, ".swx") {
		return true
	}
	if strings.HasPrefix(base, "#") && strings.HasSuffix(base, "#") {
		return true
	}
	return false
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

	// Temp files from editors: keep in memory only, never upload to Overleaf
	if isTempFile(name) {
		tmpID := "tmp-" + base
		cacheKey := parent.project.ID + "/" + tmpID
		o.Cache.Set(cacheKey, []byte{})
		return &regularFile{
			ofs:       o,
			info:      &fileInfo{name: base, modTime: time.Now()},
			projectID: parent.project.ID,
			folderID:  parent.folder.ID,
			entityID:  tmpID,
			name:      base,
			isDoc:     false,
			content:   []byte{},
			writable:  true,
			tempFile:  true,
		}, nil
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
		entityID = findDocID(tree.meta, base)
	} else {
		if err := o.Client.UploadFile(parent.project.ID, parent.folder.ID, base, []byte{}); err != nil {
			return nil, err
		}
		o.invalidateTree(parent.project.ID)
		tree, err := o.getProjectTree(parent.project.ID)
		if err != nil {
			return nil, err
		}
		entityID = findFileRefID(tree.meta, base)
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

	// Handle temp file → real file rename (atomic save pattern).
	// Copy temp file content to the real file's cache entry as dirty.
	if isTempFile(oldName) && !isTempFile(newName) {
		return o.renameTempToReal(oldName, newName)
	}

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

// renameTempToReal handles the editor atomic save: tmp file → real file.
// It copies the temp content into the real file's cache as dirty.
func (o *OverleafFS) renameTempToReal(oldName, newName string) error {
	newName = path.Clean(newName)

	// Resolve the target to get project/entity info
	target, err := o.resolve(newName)
	if err != nil {
		return err
	}

	// Read temp file content from cache
	parts := strings.Split(strings.TrimPrefix(path.Clean(oldName), "/"), "/")
	if len(parts) < 2 {
		return os.ErrInvalid
	}
	project, ok := o.findProjectByName(parts[0])
	if !ok {
		return os.ErrNotExist
	}
	tmpBase := path.Base(oldName)
	tmpCacheKey := project.ID + "/tmp-" + tmpBase
	data, ok := o.Cache.Get(tmpCacheKey)
	if !ok {
		return os.ErrNotExist
	}

	// Write content to the real file's cache as dirty
	realCacheKey := target.project.ID + "/" + target.entityID()
	o.Cache.SetDirty(realCacheKey, data)

	folderID := ""
	if target.parentFolder != nil {
		folderID = target.parentFolder.ID
	}
	o.registerMeta(realCacheKey, target.project.ID, folderID, target.entityName())

	// Clean up temp cache entry
	o.Cache.Delete(tmpCacheKey)

	// In non-zen mode, upload immediately
	if !o.ZenMode {
		if o.isIgnored(target.entityName(), false) {
			log.Printf("skipping upload of %s (ignored by .dlignore)", target.entityName())
			o.Cache.ClearDirty(realCacheKey)
			return nil
		}
		log.Printf("uploading %s to Overleaf", target.entityName())
		if err := o.Client.UploadFile(target.project.ID, folderID, target.entityName(), data); err != nil {
			log.Printf("upload %s: %v", target.entityName(), err)
			return err
		}
		o.Cache.ClearDirty(realCacheKey)
	}

	return nil
}

func (o *OverleafFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	// Temp files: check cache directly
	if isTempFile(name) {
		return o.statTempFile(name)
	}

	info, err := o.resolveWithCache(name)
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

func (fi *fileInfo) Name() string       { return fi.name }
func (fi *fileInfo) Size() int64        { return fi.size }
func (fi *fileInfo) ModTime() time.Time { return fi.modTime }
func (fi *fileInfo) IsDir() bool        { return fi.dir }
func (fi *fileInfo) Sys() any           { return nil }
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

func (d *dirFile) Close() error               { return nil }
func (d *dirFile) Read([]byte) (int, error)   { return 0, os.ErrInvalid }
func (d *dirFile) Write([]byte) (int, error)  { return 0, os.ErrInvalid }
func (d *dirFile) Stat() (fs.FileInfo, error) { return d.info, nil }
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
	tempFile  bool
}

func (f *regularFile) Stat() (fs.FileInfo, error)         { return f.info, nil }
func (f *regularFile) Readdir(int) ([]fs.FileInfo, error) { return nil, os.ErrInvalid }

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

	// Temp files: cache content only (for rename), don't upload or mark dirty
	if f.tempFile {
		f.ofs.Cache.Set(cacheKey, f.content)
		return nil
	}

	f.ofs.Cache.SetDirty(cacheKey, f.content)
	f.ofs.registerMeta(cacheKey, f.projectID, f.folderID, f.name)

	if f.ofs.ZenMode {
		return nil
	}

	if f.ofs.isIgnored(f.name, false) {
		log.Printf("skipping upload of %s (ignored by .dlignore)", f.name)
		f.ofs.Cache.ClearDirty(cacheKey)
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

// isIgnored checks whether a filename should be skipped during sync.
func (o *OverleafFS) isIgnored(name string, isDir bool) bool {
	if o.Ignore == nil {
		return false
	}
	return o.Ignore.Match(name, isDir)
}

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

// isNoiseRequest returns true for metadata files that editors/OS probe for
// and that will never exist on Overleaf. These are silently ignored in logs.
func isNoiseRequest(path string) bool {
	base := filepath.Base(path)
	switch {
	case strings.HasPrefix(base, "."):
		return true // .git, .DS_Store, .claude, .hidden, .Spotlight-V100, etc.
	case base == "package.json" || base == "node_modules":
		return true
	case strings.HasPrefix(base, "._"):
		return true // macOS resource forks
	}
	return false
}

// Serve starts the WebDAV server on the given address.
func Serve(addr string, ofs *OverleafFS) error {
	handler := &gowebdav.Handler{
		FileSystem: ofs,
		LockSystem: gowebdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				if os.IsNotExist(err) && (r.Method == "PROPFIND" || isNoiseRequest(r.URL.Path)) {
					return
				}
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
