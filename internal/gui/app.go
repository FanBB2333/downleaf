package gui

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/joho/godotenv"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/FanBB2333/downleaf/internal/api"
	"github.com/FanBB2333/downleaf/internal/auth"
	"github.com/FanBB2333/downleaf/internal/model"
	dav "github.com/FanBB2333/downleaf/internal/webdav"
)

// LoginStatus is returned to the frontend.
type LoginStatus struct {
	LoggedIn bool   `json:"loggedIn"`
	Email    string `json:"email"`
	SiteURL  string `json:"siteURL"`
}

// MountStatus is returned to the frontend.
type MountStatus struct {
	Mounted    bool     `json:"mounted"`
	Mountpoint string   `json:"mountpoint"`
	Project    []string `json:"project"`
	ZenMode    bool     `json:"zenMode"`
	WebDAVAddr string `json:"webdavAddr"`
}

// App is the Wails binding struct that bridges Go backend and JS frontend.
type App struct {
	ctx context.Context

	mu         sync.Mutex
	identity   *auth.Identity
	client     *api.Client
	ofs        *dav.OverleafFS
	siteURL    string
	mounted    bool
	mountpoint string
	zenMode    bool
	addr       string
	projects   []string // project filters (names or empty for all)

	logMu  sync.Mutex
	logBuf []string
}

func NewApp() *App {
	return &App{
		addr: "localhost:9090",
		logBuf: make([]string, 0, 512),
	}
}

// Startup is called by Wails on app startup.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	// Capture log output and forward to frontend via events.
	log.SetOutput(&logWriter{app: a})
	log.SetFlags(log.Ltime)

	// Try to load .env defaults
	if err := godotenv.Load(); err == nil {
		a.siteURL = os.Getenv("SITE")
	}
}

// Shutdown is called by Wails when the window closes.
func (a *App) Shutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.mounted && a.ofs != nil {
		a.ofs.FlushAll()
		a.ofs.DisconnectAll()
		dav.Unmount(a.mountpoint)
		a.mounted = false
	}
}

// GetEnvDefaults returns saved .env values so the frontend can pre-fill fields.
func (a *App) GetEnvDefaults() map[string]string {
	return map[string]string{
		"site":    os.Getenv("SITE"),
		"cookies": os.Getenv("COOKIES"),
	}
}

// Login authenticates with Overleaf using site URL and cookies.
func (a *App) Login(siteURL, cookies string) (*LoginStatus, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	siteURL = strings.TrimRight(siteURL, "/")
	identity, err := auth.LoginWithCookies(siteURL, cookies)
	if err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}

	a.identity = identity
	a.siteURL = siteURL
	a.client = api.NewClient(siteURL, identity)

	log.Printf("Authenticated as %s", identity.Email)
	return &LoginStatus{LoggedIn: true, Email: identity.Email, SiteURL: siteURL}, nil
}

// GetLoginStatus returns current login state.
func (a *App) GetLoginStatus() *LoginStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.identity == nil {
		return &LoginStatus{}
	}
	return &LoginStatus{LoggedIn: true, Email: a.identity.Email, SiteURL: a.siteURL}
}

// ListProjects returns all active projects.
func (a *App) ListProjects() ([]model.Project, error) {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()
	if client == nil {
		return nil, fmt.Errorf("not logged in")
	}

	projects, err := client.ListProjects()
	if err != nil {
		return nil, err
	}
	var active []model.Project
	for _, p := range projects {
		if !p.Archived && !p.Trashed {
			active = append(active, p)
		}
	}
	return active, nil
}

// Mount starts the WebDAV server and mounts the filesystem.
func (a *App) Mount(projectNames []string, mountpoint string, zenMode bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client == nil {
		return fmt.Errorf("not logged in")
	}
	if a.mounted {
		return fmt.Errorf("already mounted at %s", a.mountpoint)
	}

	if mountpoint == "" {
		mountpoint = filepath.Join(os.Getenv("HOME"), "downleaf")
	} else if mountpoint == "~" {
		mountpoint = os.Getenv("HOME")
	} else if strings.HasPrefix(mountpoint, "~/") {
		mountpoint = filepath.Join(os.Getenv("HOME"), mountpoint[2:])
	}

	ofs := dav.NewOverleafFS(a.client)
	ofs.ZenMode = zenMode
	if len(projectNames) > 0 {
		ofs.ProjectFilters = projectNames
	}

	// Start WebDAV server
	go func() {
		if err := dav.Serve(a.addr, ofs); err != nil {
			log.Printf("WebDAV server error: %v", err)
		}
	}()

	// Mount
	if err := dav.MountNative(a.addr, mountpoint); err != nil {
		log.Printf("Auto-mount failed: %v (try mounting manually via http://%s)", err, a.addr)
	} else {
		log.Printf("Mounted at %s", mountpoint)
	}

	a.ofs = ofs
	a.mounted = true
	a.mountpoint = mountpoint
	a.zenMode = zenMode
	a.projects = projectNames

	wailsRuntime.EventsEmit(a.ctx, "mountStatusChanged")
	return nil
}

// Unmount stops the mount.
func (a *App) Unmount() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.mounted {
		return fmt.Errorf("not mounted")
	}

	a.ofs.FlushAll()
	a.ofs.DisconnectAll()
	if err := dav.Unmount(a.mountpoint); err != nil {
		return fmt.Errorf("unmount failed: %w", err)
	}

	log.Printf("Unmounted %s", a.mountpoint)
	a.mounted = false
	a.ofs = nil

	wailsRuntime.EventsEmit(a.ctx, "mountStatusChanged")
	return nil
}

// Sync flushes all dirty files to Overleaf.
func (a *App) Sync() (string, error) {
	a.mu.Lock()
	ofs := a.ofs
	mounted := a.mounted
	a.mu.Unlock()

	if !mounted || ofs == nil {
		return "", fmt.Errorf("not mounted")
	}

	flushed, errors := ofs.FlushAll()
	msg := fmt.Sprintf("Sync complete: %d flushed, %d errors", flushed, errors)
	log.Print(msg)
	return msg, nil
}

// GetMountStatus returns current mount state.
func (a *App) GetMountStatus() *MountStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return &MountStatus{
		Mounted:    a.mounted,
		Mountpoint: a.mountpoint,
		Project:    a.projects,
		ZenMode:    a.zenMode,
		WebDAVAddr: a.addr,
	}
}

// GetLogs returns recent log lines.
func (a *App) GetLogs() []string {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	out := make([]string, len(a.logBuf))
	copy(out, a.logBuf)
	return out
}

// OpenMountpoint opens the mount directory in the file manager.
func (a *App) OpenMountpoint() error {
	a.mu.Lock()
	mp := a.mountpoint
	a.mu.Unlock()
	if mp == "" {
		return fmt.Errorf("not mounted")
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", mp).Start()
	case "linux":
		return exec.Command("xdg-open", mp).Start()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// logWriter captures log output and emits events to the frontend.
type logWriter struct {
	app *App
}

func (w *logWriter) Write(p []byte) (int, error) {
	line := strings.TrimRight(string(p), "\n")
	if line == "" {
		return len(p), nil
	}

	w.app.logMu.Lock()
	if len(w.app.logBuf) >= 500 {
		w.app.logBuf = w.app.logBuf[1:]
	}
	w.app.logBuf = append(w.app.logBuf, line)
	w.app.logMu.Unlock()

	if w.app.ctx != nil {
		wailsRuntime.EventsEmit(w.app.ctx, "log", line)
	}

	// Also write to stderr for debugging
	_, _ = io.WriteString(os.Stderr, line+"\n")
	return len(p), nil
}
