// Package mount defines the MountBackend interface and a registry for
// pluggable filesystem backends (WebDAV, FUSE, etc.).
package mount

import (
	"fmt"
	"sort"
	"sync"

	"github.com/FanBB2333/downleaf/internal/api"
	"github.com/FanBB2333/downleaf/internal/ignore"
)

// Backend is the interface every mount backend must implement.
type Backend interface {
	// Name returns the backend identifier (e.g. "webdav", "fuse").
	Name() string

	// Start initialises the backend and begins serving.
	// It should block until Stop is called or an error occurs.
	Start(cfg Config) error

	// Stop gracefully shuts down: flush dirty files, disconnect, unmount.
	Stop() error

	// FlushAll uploads all dirty cached files to the remote.
	FlushAll() (flushed, errors int)

	// DirtySummary returns stats about locally modified files.
	DirtySummary() []FileStat
}

// Config holds all the parameters a backend needs to start.
type Config struct {
	Client         *api.Client
	Addr           string   // e.g. "localhost:9090" (WebDAV-specific, ignored by FUSE)
	Mountpoint     string
	ProjectFilters []string
	ZenMode        bool
	Ignore         *ignore.Matcher
}

// FileStat describes a single dirty file (mirrors webdav.FileStat).
type FileStat struct {
	Name  string
	Lines int
	Bytes int
}

// ────────────────────────────────────────────────────────────────────────────
// Registry
// ────────────────────────────────────────────────────────────────────────────

var (
	registryMu sync.RWMutex
	registry   = map[string]func() Backend{}
)

// Register adds a backend constructor. Call from init() in each backend.
func Register(name string, ctor func() Backend) {
	registryMu.Lock()
	registry[name] = ctor
	registryMu.Unlock()
}

// Get returns a new instance of the named backend.
func Get(name string) (Backend, error) {
	registryMu.RLock()
	ctor, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown mount backend %q (available: %s)", name, Available())
	}
	return ctor(), nil
}

// Available returns sorted names of all registered backends.
func Available() string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return fmt.Sprintf("%v", names)
}
