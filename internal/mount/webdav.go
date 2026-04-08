package mount

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	dav "github.com/FanBB2333/downleaf/internal/webdav"
)

func init() {
	Register("webdav", func() Backend { return &webdavBackend{} })
}

type webdavBackend struct {
	mu         sync.Mutex
	ofs        *dav.OverleafFS
	server     *dav.Server
	addr       string
	mountpoint string
}

func (w *webdavBackend) Name() string { return "webdav" }

func (w *webdavBackend) Start(cfg Config) error {
	ofs := dav.NewOverleafFS(cfg.Client)
	ofs.ZenMode = cfg.ZenMode
	ofs.Cache.ZenMode = cfg.ZenMode
	if len(cfg.ProjectFilters) > 0 {
		ofs.ProjectFilters = cfg.ProjectFilters
	}
	if cfg.Ignore != nil {
		ofs.Ignore = cfg.Ignore
	}
	server := dav.NewServer(cfg.Addr, ofs)

	w.mu.Lock()
	w.addr = cfg.Addr
	w.mountpoint = cfg.Mountpoint
	w.ofs = ofs
	w.server = server
	w.mu.Unlock()

	errCh, err := server.Start()
	if err != nil {
		w.clear()
		return err
	}

	if err := dav.MountNative(cfg.Addr, cfg.Mountpoint); err != nil {
		log.Printf("Auto-mount failed: %v (mount manually via http://%s)", err, cfg.Addr)
	} else {
		// Log with project names if filters are specified
		if len(cfg.ProjectFilters) > 0 {
			log.Printf("Mounted at %s (projects: %s)", cfg.Mountpoint, strings.Join(cfg.ProjectFilters, ", "))
		} else {
			log.Printf("Mounted at %s (all projects)", cfg.Mountpoint)
		}
	}

	// Block until the server returns (only on error)
	return <-errCh
}

func (w *webdavBackend) Stop() error {
	ofs, server, mountpoint := w.snapshot()
	if ofs != nil {
		ofs.FlushAll()
		ofs.DisconnectAll()
	}
	err := dav.Unmount(mountpoint)
	if err == dav.ErrMountBusy {
		return ErrMountBusy
	}
	if err != nil {
		return err
	}
	if err := shutdownServer(server); err != nil {
		return err
	}
	w.clear()
	return nil
}

func (w *webdavBackend) ForceStop() error {
	ofs, server, mountpoint := w.snapshot()
	if ofs != nil {
		ofs.FlushAll()
		ofs.DisconnectAll()
	}
	if err := dav.ForceUnmount(mountpoint); err != nil {
		return err
	}
	if err := shutdownServer(server); err != nil {
		return err
	}
	w.clear()
	return nil
}

func (w *webdavBackend) FlushAll() (flushed, errors int) {
	ofs, _, _ := w.snapshot()
	if ofs == nil {
		return 0, 0
	}
	return ofs.FlushAll()
}

func (w *webdavBackend) DirtySummary() []FileStat {
	ofs, _, _ := w.snapshot()
	if ofs == nil {
		return nil
	}
	davStats := ofs.DirtySummary()
	stats := make([]FileStat, len(davStats))
	for i, s := range davStats {
		stats[i] = FileStat{Name: s.Name, Lines: s.Lines, Bytes: s.Bytes}
	}
	return stats
}

func (w *webdavBackend) snapshot() (*dav.OverleafFS, *dav.Server, string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ofs, w.server, w.mountpoint
}

func (w *webdavBackend) clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ofs = nil
	w.server = nil
	w.addr = ""
	w.mountpoint = ""
}

func shutdownServer(server *dav.Server) error {
	if server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}
