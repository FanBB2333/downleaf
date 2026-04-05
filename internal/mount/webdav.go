package mount

import (
	"log"

	dav "github.com/FanBB2333/downleaf/internal/webdav"
)

func init() {
	Register("webdav", func() Backend { return &webdavBackend{} })
}

type webdavBackend struct {
	ofs        *dav.OverleafFS
	addr       string
	mountpoint string
}

func (w *webdavBackend) Name() string { return "webdav" }

func (w *webdavBackend) Start(cfg Config) error {
	w.addr = cfg.Addr
	w.mountpoint = cfg.Mountpoint

	ofs := dav.NewOverleafFS(cfg.Client)
	ofs.ZenMode = cfg.ZenMode
	ofs.Cache.ZenMode = cfg.ZenMode
	if len(cfg.ProjectFilters) > 0 {
		ofs.ProjectFilters = cfg.ProjectFilters
	}
	if cfg.Ignore != nil {
		ofs.Ignore = cfg.Ignore
	}
	w.ofs = ofs

	// Start WebDAV server in a goroutine, then mount
	errCh := make(chan error, 1)
	go func() {
		errCh <- dav.Serve(cfg.Addr, ofs)
	}()

	log.Printf("WebDAV server: http://%s", cfg.Addr)
	log.Printf("Mounting at %s ...", cfg.Mountpoint)

	if err := dav.MountNative(cfg.Addr, cfg.Mountpoint); err != nil {
		log.Printf("Auto-mount failed: %v", err)
		log.Printf("  macOS:  mount_webdav http://%s %s", cfg.Addr, cfg.Mountpoint)
		log.Printf("  Linux:  sudo mount -t davfs http://%s %s", cfg.Addr, cfg.Mountpoint)
	} else {
		log.Printf("Mounted at %s", cfg.Mountpoint)
	}

	// Block until the server returns (only on error)
	return <-errCh
}

func (w *webdavBackend) Stop() error {
	if w.ofs != nil {
		w.ofs.FlushAll()
		w.ofs.DisconnectAll()
	}
	return dav.Unmount(w.mountpoint)
}

func (w *webdavBackend) FlushAll() (flushed, errors int) {
	if w.ofs == nil {
		return 0, 0
	}
	return w.ofs.FlushAll()
}

func (w *webdavBackend) DirtySummary() []FileStat {
	if w.ofs == nil {
		return nil
	}
	davStats := w.ofs.DirtySummary()
	stats := make([]FileStat, len(davStats))
	for i, s := range davStats {
		stats[i] = FileStat{Name: s.Name, Lines: s.Lines, Bytes: s.Bytes}
	}
	return stats
}
