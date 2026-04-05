package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/FanBB2333/downleaf/internal/api"
	"github.com/FanBB2333/downleaf/internal/auth"
	"github.com/FanBB2333/downleaf/internal/credential"
	"github.com/FanBB2333/downleaf/internal/ignore"
	"github.com/FanBB2333/downleaf/internal/model"
	"github.com/FanBB2333/downleaf/internal/mount"
	"github.com/FanBB2333/downleaf/internal/version"
	dav "github.com/FanBB2333/downleaf/internal/webdav"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil {
		// silently ignore — not required if using stored credentials
	}

	cmd := "ls"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	// Commands that don't require authentication
	switch cmd {
	case "version", "--version", "-v":
		fmt.Printf("downleaf %s\n", version.Version)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	case "sync":
		return cmdSync()
	case "umount", "unmount":
		mountpoint := filepath.Join(os.Getenv("HOME"), "downleaf")
		if len(os.Args) > 2 {
			mountpoint = os.Args[2]
		}
		return cmdUmount(mountpoint)
	case "login":
		return cmdLogin()
	case "logout":
		return cmdLogout()
	case "accounts":
		return cmdAccounts()
	}

	// Authenticate: try stored credentials first, then fall back to env vars
	siteURL, identity, err := authenticate()
	if err != nil {
		return err
	}
	fmt.Printf("Authenticated as: %s\n", identity.Email)

	client := api.NewClient(siteURL, identity)

	switch cmd {
	case "ls":
		return cmdLS(client)
	case "tree":
		if len(os.Args) < 3 {
			return fmt.Errorf("usage: downleaf tree <project-id>")
		}
		return cmdTree(client, os.Args[2])
	case "cat":
		if len(os.Args) < 4 {
			return fmt.Errorf("usage: downleaf cat <project-id> <doc-id>")
		}
		return cmdCat(client, os.Args[2], os.Args[3])
	case "mount":
		mountpoint := filepath.Join(os.Getenv("HOME"), "downleaf")
		addr := "localhost:9090"
		var projectFilters []string
		if envProject := os.Getenv("PROJECT"); envProject != "" {
			projectFilters = []string{envProject}
		}
		zenMode := false
		foreground := false
		mountAll := false
		backendName := "webdav"
		for i := 2; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--project":
				if i+1 < len(os.Args) {
					projectFilters = append(projectFilters, os.Args[i+1])
					i++
				}
			case "--port":
				if i+1 < len(os.Args) {
					addr = "localhost:" + os.Args[i+1]
					i++
				}
			case "--zen":
				zenMode = true
			case "--all":
				mountAll = true
			case "--foreground", "-f":
				foreground = true
			case "--backend":
				if i+1 < len(os.Args) {
					backendName = os.Args[i+1]
					i++
				}
			default:
				mountpoint = os.Args[i]
			}
		}
		// Interactive selection is default unless --all or --project is specified
		if !mountAll && len(projectFilters) == 0 {
			selected, err := selectProject(client)
			if err != nil {
				return err
			}
			if selected != "" {
				projectFilters = []string{selected}
			}
		}

		// Validate backend name early
		if _, err := mount.Get(backendName); err != nil {
			return err
		}

		// Default: daemonize. With --foreground: block in terminal.
		if !foreground {
			return cmdMountDaemon(addr, mountpoint, projectFilters, zenMode, backendName)
		}
		return cmdMount(client, addr, mountpoint, projectFilters, zenMode, backendName)
	case "download":
		if len(os.Args) < 3 {
			return fmt.Errorf("usage: downleaf download <project-id> [dest-dir]")
		}
		dest := "."
		if len(os.Args) > 3 {
			dest = os.Args[3]
		}
		return cmdDownload(client, os.Args[2], dest)
	default:
		printUsage()
		return nil
	}
}

func printUsage() {
	fmt.Printf("downleaf %s\n", version.Version)
	fmt.Println()
	fmt.Println("Usage: downleaf <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  ls                                 List all projects")
	fmt.Println("  tree <project-id>                  Show project file tree")
	fmt.Println("  cat <project-id> <doc-id>          Print document content")
	fmt.Println("  download <project-id> [dest-dir]   Download project files locally")
	fmt.Println("  mount [mountpoint] [options]")
	fmt.Println("                                     Mount Overleaf projects locally (default: ~/downleaf, port 9090)")
	fmt.Println("                                     Interactive project selection by default")
	fmt.Println("                                     --all: mount all projects (skip interactive selection)")
	fmt.Println("                                     --project <name|id>: mount specific project(s), can be repeated")
	fmt.Println("                                     --foreground, -f: run in foreground (block terminal, Ctrl+C to stop)")
	fmt.Println("                                     --zen: changes stay local, sync on exit or 'downleaf sync'")
	fmt.Printf("                                     --backend <name>: mount backend (available: %s, default: webdav)\n", mount.Available())
	fmt.Println("  sync                               Push all local changes to Overleaf (zen mode)")
	fmt.Println("  umount [mountpoint]                Unmount filesystem")
	fmt.Println("  login [site-url]                   Log in (browser or cookie) and save credential")
	fmt.Println("  logout [email|id]                  Remove a saved credential")
	fmt.Println("  accounts                           List saved credentials")
	fmt.Println("  version                            Print version")
	fmt.Println()
	fmt.Println("Authentication:")
	fmt.Println("  Uses saved credentials by default. Env vars SITE+COOKIES override.")
	fmt.Println()
	fmt.Println("Environment variables (via .env or shell):")
	fmt.Println("  SITE       Overleaf site URL")
	fmt.Println("  COOKIES    Session cookie")
	fmt.Println("  PROJECT    Default project name or ID for mount")
}

func cmdLS(client *api.Client) error {
	projects, err := client.ListProjects()
	if err != nil {
		return err
	}

	fmt.Printf("\n%d projects:\n", len(projects))
	for _, p := range projects {
		status := ""
		if p.Archived {
			status = " [archived]"
		}
		if p.Trashed {
			status = " [trashed]"
		}
		fmt.Printf("  %s  %s%s\n", p.ID, p.Name, status)
	}
	return nil
}

func cmdTree(client *api.Client, projectID string) error {
	sio := api.NewSocketIOClient(client.SiteURL, client.Identity)
	tree, err := sio.JoinProject(projectID)
	if err != nil {
		return fmt.Errorf("join project: %w", err)
	}
	defer sio.Disconnect()

	fmt.Printf("Project: %s\n", tree.Name)
	if len(tree.RootFolder) > 0 {
		printFolder(&tree.RootFolder[0], "")
	}
	return nil
}

func printFolder(f *model.Folder, indent string) {
	for _, sub := range f.Folders {
		fmt.Printf("%s%s/ (id: %s)\n", indent, sub.Name, sub.ID)
		printFolder(&sub, indent+"  ")
	}
	for _, doc := range f.Docs {
		fmt.Printf("%s%s (doc: %s)\n", indent, doc.Name, doc.ID)
	}
	for _, ref := range f.FileRefs {
		fmt.Printf("%s%s (file: %s)\n", indent, ref.Name, ref.ID)
	}
}

func cmdCat(client *api.Client, projectID, docID string) error {
	sio := api.NewSocketIOClient(client.SiteURL, client.Identity)
	_, err := sio.JoinProject(projectID)
	if err != nil {
		return fmt.Errorf("join project: %w", err)
	}
	defer sio.Disconnect()

	content, version, err := sio.JoinDoc(projectID, docID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "note: joinDoc failed (%v), trying binary download\n", err)
		data, dlErr := client.DownloadFile(projectID, docID)
		if dlErr != nil {
			return fmt.Errorf("download: %w", dlErr)
		}
		os.Stdout.Write(data)
		return nil
	}

	fmt.Printf("--- version: %d ---\n", version)
	fmt.Println(content)
	return nil
}

func cmdDownload(client *api.Client, projectID, destDir string) error {
	sio := api.NewSocketIOClient(client.SiteURL, client.Identity)
	tree, err := sio.JoinProject(projectID)
	if err != nil {
		return fmt.Errorf("join project: %w", err)
	}
	defer sio.Disconnect()

	fmt.Printf("Downloading project: %s\n", tree.Name)
	if len(tree.RootFolder) == 0 {
		return fmt.Errorf("project has no root folder")
	}

	projectDir := filepath.Join(destDir, tree.Name)
	return downloadFolder(client, sio, projectID, &tree.RootFolder[0], projectDir)
}

func downloadFolder(client *api.Client, sio *api.SocketIOClient, projectID string, folder *model.Folder, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	for _, doc := range folder.Docs {
		content, _, err := sio.JoinDoc(projectID, doc.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skip doc %s: %v\n", doc.Name, err)
			continue
		}
		sio.LeaveDoc(projectID, doc.ID)
		p := filepath.Join(dir, doc.Name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			return err
		}
		fmt.Printf("  %s\n", p)
	}

	for _, ref := range folder.FileRefs {
		data, err := client.DownloadFile(projectID, ref.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skip file %s: %v\n", ref.Name, err)
			continue
		}
		p := filepath.Join(dir, ref.Name)
		if err := os.WriteFile(p, data, 0644); err != nil {
			return err
		}
		fmt.Printf("  %s\n", p)
	}

	for _, sub := range folder.Folders {
		if err := downloadFolder(client, sio, projectID, &sub, filepath.Join(dir, sub.Name)); err != nil {
			return err
		}
	}
	return nil
}

func selectProject(client *api.Client) (string, error) {
	projects, err := client.ListProjects()
	if err != nil {
		return "", err
	}

	var active []model.Project
	for _, p := range projects {
		if !p.Archived && !p.Trashed {
			active = append(active, p)
		}
	}

	if len(active) == 0 {
		return "", fmt.Errorf("no active projects found")
	}

	fmt.Println()
	fmt.Printf("Select a project to mount (%d projects):\n", len(active))
	fmt.Println("  0) [all projects]")
	for i, p := range active {
		fmt.Printf("  %d) %s\n", i+1, p.Name)
	}
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Enter number (0 for all): ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)

		n, err := strconv.Atoi(line)
		if err != nil || n < 0 || n > len(active) {
			fmt.Println("Invalid selection, try again.")
			continue
		}

		if n == 0 {
			return "", nil
		}
		selected := active[n-1]
		fmt.Printf("Selected: %s (%s)\n", selected.Name, selected.ID)
		return selected.ID, nil
	}
}

// authenticate tries stored credentials first, then env vars.
func authenticate() (siteURL string, identity *auth.Identity, err error) {
	// If env vars are set, use them (override stored creds)
	siteURL = os.Getenv("SITE")
	cookies := os.Getenv("COOKIES")
	if siteURL != "" && cookies != "" {
		fmt.Printf("Authenticating with %s ...\n", siteURL)
		identity, err = auth.LoginWithCookies(siteURL, cookies)
		if err != nil {
			return "", nil, fmt.Errorf("authentication failed: %w", err)
		}
		return siteURL, identity, nil
	}

	// Try stored credentials (most recently used first)
	store, storeErr := credential.NewStore()
	if storeErr == nil {
		creds, listErr := store.List()
		if listErr == nil && len(creds) > 0 {
			// Use the most recently used credential
			cred, loadErr := store.Load(creds[0].ID)
			if loadErr == nil {
				fmt.Printf("Authenticating with %s (%s) ...\n", cred.SiteURL, cred.Email)
				identity, err = auth.LoginWithCookies(cred.SiteURL, cred.Cookies)
				if err != nil {
					return "", nil, fmt.Errorf("stored credential expired for %s, run 'downleaf login' to re-authenticate: %w", cred.Email, err)
				}
				store.UpdateLastUsed(cred.ID)
				return cred.SiteURL, identity, nil
			}
		}
	}

	return "", nil, fmt.Errorf("no credentials found. Run 'downleaf login' or set SITE+COOKIES in .env")
}

func cmdLogin() error {
	reader := bufio.NewReader(os.Stdin)

	// Get site URL
	siteURL := os.Getenv("SITE")
	if len(os.Args) > 2 {
		siteURL = os.Args[2]
	}
	if siteURL == "" {
		fmt.Print("Overleaf site URL (e.g. https://www.overleaf.com): ")
		line, _ := reader.ReadString('\n')
		siteURL = strings.TrimSpace(line)
	}
	if siteURL == "" {
		return fmt.Errorf("site URL is required")
	}
	siteURL = strings.TrimRight(siteURL, "/")

	store, err := credential.NewStore()
	if err != nil {
		return fmt.Errorf("credential store: %w", err)
	}

	// Try browser login if supported
	if auth.IsBrowserLoginSupported() {
		fmt.Println("Opening browser for login...")
		cookies, err := auth.BrowserLogin(siteURL)
		if err != nil {
			fmt.Printf("Browser login failed: %v\n", err)
			fmt.Println("Falling back to cookie login...")
		} else {
			return saveCookieCredential(store, siteURL, cookies)
		}
	}

	// Manual cookie login
	fmt.Println("Paste your session cookie (from browser DevTools → Application → Cookies):")
	fmt.Print("Cookie: ")
	line, _ := reader.ReadString('\n')
	cookies := strings.TrimSpace(line)
	if cookies == "" {
		return fmt.Errorf("cookie is required")
	}

	return saveCookieCredential(store, siteURL, cookies)
}

func saveCookieCredential(store *credential.Store, siteURL, cookies string) error {
	fmt.Println("Validating cookies...")
	identity, err := auth.LoginWithCookies(siteURL, cookies)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

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
	if err := store.Save(cred); err != nil {
		return fmt.Errorf("save credential: %w", err)
	}

	fmt.Printf("Logged in as %s (%s)\n", identity.Email, siteURL)
	fmt.Println("Credential saved.")
	return nil
}

func cmdLogout() error {
	store, err := credential.NewStore()
	if err != nil {
		return fmt.Errorf("credential store: %w", err)
	}

	creds, err := store.List()
	if err != nil {
		return err
	}
	if len(creds) == 0 {
		fmt.Println("No saved credentials.")
		return nil
	}

	// If an argument is provided, match by email or ID
	if len(os.Args) > 2 {
		target := os.Args[2]
		for _, c := range creds {
			if c.ID == target || strings.EqualFold(c.Email, target) {
				if err := store.Delete(c.ID); err != nil {
					return err
				}
				fmt.Printf("Removed credential for %s (%s)\n", c.Email, c.SiteURL)
				return nil
			}
		}
		return fmt.Errorf("credential not found: %s", target)
	}

	// Interactive selection
	fmt.Println("Select credential to remove:")
	for i, c := range creds {
		fmt.Printf("  %d) %s — %s\n", i+1, c.Email, c.SiteURL)
	}
	fmt.Print("Enter number: ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(creds) {
		return fmt.Errorf("invalid selection")
	}
	c := creds[n-1]
	if err := store.Delete(c.ID); err != nil {
		return err
	}
	fmt.Printf("Removed credential for %s (%s)\n", c.Email, c.SiteURL)
	return nil
}

func cmdAccounts() error {
	store, err := credential.NewStore()
	if err != nil {
		return fmt.Errorf("credential store: %w", err)
	}

	creds, err := store.List()
	if err != nil {
		return err
	}
	if len(creds) == 0 {
		fmt.Println("No saved credentials. Run 'downleaf login' to add one.")
		return nil
	}

	fmt.Printf("\n%d saved account(s):\n", len(creds))
	for i, c := range creds {
		marker := "  "
		if i == 0 {
			marker = "* " // most recently used = active
		}
		fmt.Printf("  %s%s — %s (last used: %s)\n", marker, c.Email, c.SiteURL, c.LastUsedAt.Format("2006-01-02"))
	}
	fmt.Println()
	fmt.Println("  * = active (most recently used)")
	return nil
}

func cmdUmount(mountpoint string) error {
	// Try to signal the daemon process to flush and exit gracefully
	pidData, err := os.ReadFile(dav.PIDFile)
	if err == nil {
		var pid int
		if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err == nil {
			proc, err := os.FindProcess(pid)
			if err == nil {
				fmt.Printf("Sending stop signal to daemon (PID %d)...\n", pid)
				// SIGTERM triggers the signal handler which flushes dirty files,
				// disconnects Socket.IO, unmounts, and exits.
				if err := proc.Signal(syscall.SIGTERM); err != nil {
					fmt.Printf("Could not signal process %d: %v\n", pid, err)
					fmt.Println("Falling back to direct unmount...")
				} else {
					// Wait for the process to exit (up to 30 seconds)
					done := make(chan error, 1)
					go func() {
						_, err := proc.Wait()
						done <- err
					}()
					select {
					case <-done:
						fmt.Println("Daemon stopped.")
						return nil
					case <-time.After(30 * time.Second):
						fmt.Println("Daemon did not exit in time, forcing unmount...")
					}
				}
			}
		}
	}

	// Fallback: direct unmount (no daemon found or signal failed)
	return dav.Unmount(mountpoint)
}

func cmdSync() error {
	pidData, err := os.ReadFile(dav.PIDFile)
	if err != nil {
		return fmt.Errorf("no running mount found (cannot read %s): %w", dav.PIDFile, err)
	}

	var pid int
	if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err != nil {
		return fmt.Errorf("invalid PID file: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}

	fmt.Printf("Sending sync signal to mount process (PID %d)...\n", pid)
	if err := proc.Signal(syscall.SIGUSR1); err != nil {
		return fmt.Errorf("failed to signal process: %w", err)
	}

	fmt.Println("Sync triggered. Check mount process output for details.")
	return nil
}

// cmdMountDaemon re-execs the current binary with --foreground in the background.
func cmdMountDaemon(addr, mountpoint string, projectFilters []string, zenMode bool, backendName string) error {
	args := []string{"mount", "--foreground", "--backend", backendName, "--port", strings.TrimPrefix(addr, "localhost:"), mountpoint}
	for _, pf := range projectFilters {
		args = append(args, "--project", pf)
	}
	if zenMode {
		args = append(args, "--zen")
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find executable: %w", err)
	}

	cmd := exec.Command(exe, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	// Detach from parent process group
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	fmt.Printf("Downleaf daemon started (PID %d)\n", cmd.Process.Pid)
	fmt.Printf("  WebDAV: http://%s\n", addr)
	fmt.Printf("  Mount:  %s\n", mountpoint)
	if zenMode {
		fmt.Println("  Mode:   zen (sync with 'downleaf sync')")
	}
	fmt.Println("  Stop:   downleaf umount")
	return nil
}

func cmdMount(client *api.Client, addr, mountpoint string, projectFilters []string, zenMode bool, backendName string) error {
	if zenMode {
		fmt.Println("Zen mode: all changes stay local for distraction-free editing.")
		fmt.Println("  Sync manually:  downleaf sync")
		fmt.Println("  Sync on exit:   Ctrl+C")
	}

	// Load .dlignore from mountpoint directory
	igMatcher, err := ignore.ParseFile(filepath.Join(mountpoint, ".dlignore"))
	if err != nil {
		log.Printf("warning: failed to parse .dlignore: %v", err)
		igMatcher = ignore.New()
	}

	backend, err := mount.Get(backendName)
	if err != nil {
		return err
	}

	cfg := mount.Config{
		Client:         client,
		Addr:           addr,
		Mountpoint:     mountpoint,
		ProjectFilters: projectFilters,
		ZenMode:        zenMode,
		Ignore:         igMatcher,
	}

	// Write PID file for sync command
	os.WriteFile(dav.PIDFile, fmt.Appendf(nil, "%d", os.Getpid()), 0644)
	defer os.Remove(dav.PIDFile)

	// Handle SIGUSR1 for zen mode sync
	syncCh := make(chan os.Signal, 1)
	signal.Notify(syncCh, syscall.SIGUSR1)
	go func() {
		for range syncCh {
			fmt.Println("\nSync requested — flushing dirty files...")
			flushed, errors := backend.FlushAll()
			fmt.Printf("Sync complete: %d flushed, %d errors\n", flushed, errors)
		}
	}()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println()

		// In zen mode, show a git-style summary of modified files before flushing
		if zenMode {
			stats := backend.DirtySummary()
			if len(stats) > 0 {
				printDirtySummary(stats)
			} else {
				fmt.Println("No local changes to sync.")
			}
		}

		fmt.Println("Flushing dirty files...")
		backend.Stop()
		os.Remove(dav.PIDFile)
		os.Exit(0)
	}()

	fmt.Printf("Using %s backend\n", backendName)
	fmt.Println("Press Ctrl+C to stop.")

	// Start blocks until error or Stop()
	return backend.Start(cfg)
}

// printDirtySummary displays a git diff --stat style summary of modified files.
func printDirtySummary(stats []mount.FileStat) {
	if len(stats) == 0 {
		return
	}

	// Find longest name for alignment
	maxName := 0
	maxLines := 0
	for _, s := range stats {
		if len(s.Name) > maxName {
			maxName = len(s.Name)
		}
		if s.Lines > maxLines {
			maxLines = s.Lines
		}
	}

	const maxBarWidth = 40

	fmt.Println("Changes to be synced:")
	fmt.Println()

	totalFiles := len(stats)
	totalLines := 0

	for _, s := range stats {
		totalLines += s.Lines

		// Scale bar width relative to the largest file
		barLen := s.Lines
		if maxLines > maxBarWidth {
			barLen = s.Lines * maxBarWidth / maxLines
		}
		if barLen < 1 && s.Lines > 0 {
			barLen = 1
		}

		bar := strings.Repeat("+", barLen)
		fmt.Printf(" %-*s | %5d %s\n", maxName, s.Name, s.Lines, bar)
	}

	fmt.Println()
	fmt.Printf(" %d file(s) changed, %d lines total\n", totalFiles, totalLines)
	fmt.Println()
}
