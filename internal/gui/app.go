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
	"time"

	"github.com/joho/godotenv"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/FanBB2333/downleaf/internal/api"
	"github.com/FanBB2333/downleaf/internal/auth"
	"github.com/FanBB2333/downleaf/internal/credential"
	"github.com/FanBB2333/downleaf/internal/ignore"
	"github.com/FanBB2333/downleaf/internal/model"
	"github.com/FanBB2333/downleaf/internal/mount"
	"github.com/FanBB2333/downleaf/internal/version"
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
	WebDAVAddr string   `json:"webdavAddr"`
	Backend    string   `json:"backend"`
}

// BackendInfo describes a mount backend for the frontend.
type BackendInfo struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

// App is the Wails binding struct that bridges Go backend and JS frontend.
type App struct {
	ctx context.Context

	mu          sync.Mutex
	identity    *auth.Identity
	client      *api.Client
	backend     mount.Backend // active mount backend (nil when not mounted)
	backendName string        // selected backend name
	siteURL     string
	mounted     bool
	mountpoint  string
	zenMode     bool
	addr        string
	projects    []string // project filters (names or empty for all)

	credStore *credential.Store // credential storage

	logMu  sync.Mutex
	logBuf []string
}

func NewApp() *App {
	return &App{
		addr:        "localhost:9090",
		backendName: "webdav",
		logBuf:      make([]string, 0, 512),
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

	// Initialize credential store
	store, err := credential.NewStore()
	if err != nil {
		log.Printf("Warning: failed to initialize credential store: %v", err)
	} else {
		a.credStore = store
	}
}

// Shutdown is called by Wails when the window closes.
func (a *App) Shutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.mounted && a.backend != nil {
		if a.zenMode {
			stats := a.backend.DirtySummary()
			if len(stats) > 0 {
				log.Printf("Syncing %d modified file(s):", len(stats))
				for _, s := range stats {
					log.Printf("  %-30s | %5d lines", s.Name, s.Lines)
				}
			}
		}
		a.backend.Stop()
		a.mounted = false
		a.backend = nil
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

// GetVersion returns the application version.
func (a *App) GetVersion() string {
	return version.Version
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

// ListTags returns all tags for the authenticated user.
func (a *App) ListTags() ([]model.Tag, error) {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()
	if client == nil {
		return nil, fmt.Errorf("not logged in")
	}

	return client.ListTags()
}

// Mount starts the mount backend and mounts the filesystem.
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

	backend, err := mount.Get(a.backendName)
	if err != nil {
		return fmt.Errorf("mount backend %q: %w", a.backendName, err)
	}

	// Load .dlignore from mountpoint directory
	igMatcher, igErr := ignore.ParseFile(filepath.Join(mountpoint, ".dlignore"))
	if igErr != nil {
		log.Printf("warning: failed to parse .dlignore: %v", igErr)
		igMatcher = ignore.New()
	}

	cfg := mount.Config{
		Client:         a.client,
		Addr:           a.addr,
		Mountpoint:     mountpoint,
		ProjectFilters: projectNames,
		ZenMode:        zenMode,
		Ignore:         igMatcher,
	}

	// Start backend in background (it blocks until Stop)
	go func() {
		if err := backend.Start(cfg); err != nil {
			log.Printf("Mount backend error: %v", err)
		}
	}()

	a.backend = backend
	a.mounted = true
	a.mountpoint = mountpoint
	a.zenMode = zenMode
	a.projects = projectNames

	wailsRuntime.EventsEmit(a.ctx, "mountStatusChanged")
	return nil
}

// Unmount stops the mount backend (flushes, disconnects, unmounts).
func (a *App) Unmount() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.mounted || a.backend == nil {
		return fmt.Errorf("not mounted")
	}

	if err := a.backend.Stop(); err != nil {
		return fmt.Errorf("unmount failed: %w", err)
	}

	log.Printf("Unmounted %s", a.mountpoint)
	a.mounted = false
	a.backend = nil

	wailsRuntime.EventsEmit(a.ctx, "mountStatusChanged")
	return nil
}

// Sync flushes all dirty files to Overleaf.
func (a *App) Sync() (string, error) {
	a.mu.Lock()
	backend := a.backend
	mounted := a.mounted
	a.mu.Unlock()

	if !mounted || backend == nil {
		return "", fmt.Errorf("not mounted")
	}

	flushed, errors := backend.FlushAll()
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
		Backend:    a.backendName,
	}
}

// GetBackend returns the current backend name.
func (a *App) GetBackend() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.backendName
}

// SetBackend changes the mount backend. Cannot be changed while mounted.
func (a *App) SetBackend(name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.mounted {
		return fmt.Errorf("cannot change backend while mounted — unmount first")
	}
	if _, err := mount.Get(name); err != nil {
		return err
	}
	a.backendName = name
	log.Printf("Mount backend set to %s", name)
	return nil
}

// ListBackends returns all registered backends with availability info.
func (a *App) ListBackends() []BackendInfo {
	// Currently only webdav is fully available; fuse is registered but placeholder
	names := []string{"webdav", "fuse"}
	var infos []BackendInfo
	for _, n := range names {
		_, err := mount.Get(n)
		infos = append(infos, BackendInfo{
			Name:      n,
			Available: err == nil,
		})
	}
	return infos
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

// ============================================================================
// Credential Management Methods
// ============================================================================

// IsBrowserLoginSupported returns true if browser login is available on this platform.
func (a *App) IsBrowserLoginSupported() bool {
	return auth.IsBrowserLoginSupported()
}

// LoginWithBrowser opens a native browser window for Overleaf login.
// On success, saves the credential and returns login status.
func (a *App) LoginWithBrowser(siteURL string) (*LoginStatus, error) {
	siteURL = strings.TrimRight(siteURL, "/")

	log.Printf("Opening browser login for %s...", siteURL)

	// Open browser and capture cookies
	cookies, err := auth.BrowserLogin(siteURL)
	if err != nil {
		return nil, fmt.Errorf("browser login failed: %w", err)
	}

	log.Printf("Browser login successful, validating cookies...")

	// Validate cookies and extract identity
	identity, err := auth.LoginWithCookies(siteURL, cookies)
	if err != nil {
		return nil, fmt.Errorf("cookie validation failed: %w", err)
	}

	// Save credential
	if a.credStore != nil {
		cred := &credential.Credential{
			ID:         credential.GenerateID(siteURL, identity.Email),
			SiteURL:    siteURL,
			Email:      identity.Email,
			UserID:     identity.UserID,
			Cookies:    cookies,
			CSRFToken:  identity.CSRFToken,
			CreatedAt:  time.Now(),
			LastUsedAt: time.Now(),
		}
		if err := a.credStore.Save(cred); err != nil {
			log.Printf("Warning: failed to save credential: %v", err)
		} else {
			log.Printf("Credential saved for %s", identity.Email)
		}
	}

	// Update app state
	a.mu.Lock()
	a.identity = identity
	a.siteURL = siteURL
	a.client = api.NewClient(siteURL, identity)
	a.mu.Unlock()

	log.Printf("Authenticated as %s", identity.Email)
	return &LoginStatus{LoggedIn: true, Email: identity.Email, SiteURL: siteURL}, nil
}

// ListCredentials returns summaries of all saved credentials.
func (a *App) ListCredentials() ([]credential.CredentialInfo, error) {
	if a.credStore == nil {
		return nil, fmt.Errorf("credential store not initialized")
	}
	return a.credStore.List()
}

// LoginWithCredential loads a saved credential and logs in.
func (a *App) LoginWithCredential(id string) (*LoginStatus, error) {
	if a.credStore == nil {
		return nil, fmt.Errorf("credential store not initialized")
	}

	cred, err := a.credStore.Load(id)
	if err != nil {
		return nil, fmt.Errorf("failed to load credential: %w", err)
	}

	log.Printf("Logging in with saved credential for %s...", cred.Email)

	// Validate cookies still work
	identity, err := auth.LoginWithCookies(cred.SiteURL, cred.Cookies)
	if err != nil {
		return nil, fmt.Errorf("session expired, please login again: %w", err)
	}

	// Update last used time
	if err := a.credStore.UpdateLastUsed(id); err != nil {
		log.Printf("Warning: failed to update last used time: %v", err)
	}

	// Update app state
	a.mu.Lock()
	a.identity = identity
	a.siteURL = cred.SiteURL
	a.client = api.NewClient(cred.SiteURL, identity)
	a.mu.Unlock()

	log.Printf("Authenticated as %s", identity.Email)
	return &LoginStatus{LoggedIn: true, Email: identity.Email, SiteURL: cred.SiteURL}, nil
}

// DeleteCredential removes a saved credential.
func (a *App) DeleteCredential(id string) error {
	if a.credStore == nil {
		return fmt.Errorf("credential store not initialized")
	}
	if err := a.credStore.Delete(id); err != nil {
		return fmt.Errorf("failed to delete credential: %w", err)
	}
	log.Printf("Credential deleted: %s", id)
	return nil
}
