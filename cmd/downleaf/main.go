package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/FanBB2333/downleaf/internal/api"
	"github.com/FanBB2333/downleaf/internal/auth"
	"github.com/FanBB2333/downleaf/internal/ignore"
	"github.com/FanBB2333/downleaf/internal/model"
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
		fmt.Println("warning: no .env file found, using environment variables")
	}

	siteURL := os.Getenv("SITE")
	cookies := os.Getenv("COOKIES")

	if siteURL == "" || cookies == "" {
		return fmt.Errorf("SITE and COOKIES must be set in .env or environment")
	}

	cmd := "ls"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	if cmd == "version" || cmd == "--version" || cmd == "-v" {
		fmt.Printf("downleaf %s\n", version.Version)
		return nil
	}

	if cmd == "help" || cmd == "--help" || cmd == "-h" {
		printUsage()
		return nil
	}

	if cmd == "sync" {
		return cmdSync()
	}

	if cmd == "umount" || cmd == "unmount" {
		mountpoint := filepath.Join(os.Getenv("HOME"), "downleaf")
		if len(os.Args) > 2 {
			mountpoint = os.Args[2]
		}
		return dav.Unmount(mountpoint)
	}

	// Authenticate
	fmt.Printf("Authenticating with %s ...\n", siteURL)
	identity, err := auth.LoginWithCookies(siteURL, cookies)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
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
		projectFilter := os.Getenv("PROJECT")
		zenMode := false
		interactive := false
		for i := 2; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--project":
				if i+1 < len(os.Args) {
					projectFilter = os.Args[i+1]
					i++
				}
			case "--port":
				if i+1 < len(os.Args) {
					addr = "localhost:" + os.Args[i+1]
					i++
				}
			case "--zen":
				zenMode = true
			case "-i", "--interactive":
				interactive = true
			default:
				mountpoint = os.Args[i]
			}
		}
		if interactive && projectFilter == "" {
			selected, err := selectProject(client)
			if err != nil {
				return err
			}
			projectFilter = selected
		}
		return cmdMount(client, addr, mountpoint, projectFilter, zenMode)
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
	fmt.Println("  mount [mountpoint] [--project <name|id>] [--zen] [-i] [--port PORT]")
	fmt.Println("                                     Start WebDAV server and mount (default: ~/downleaf, port 9090)")
	fmt.Println("                                     --zen: zen mode — changes stay local, sync on exit or 'downleaf sync'")
	fmt.Println("                                     -i: interactive project selection")
	fmt.Println("  sync                               Push all local changes to Overleaf (zen mode)")
	fmt.Println("  umount [mountpoint]                Unmount filesystem")
	fmt.Println("  version                            Print version")
	fmt.Println()
	fmt.Println("Environment variables (via .env or shell):")
	fmt.Println("  SITE       Overleaf site URL (required)")
	fmt.Println("  COOKIES    Session cookie (required)")
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

func cmdMount(client *api.Client, addr, mountpoint, projectFilter string, zenMode bool) error {
	if zenMode {
		fmt.Println("Zen mode: all changes stay local for distraction-free editing.")
		fmt.Println("  Sync manually:  downleaf sync")
		fmt.Println("  Sync on exit:   Ctrl+C")
	}

	ofs := dav.NewOverleafFS(client)
	ofs.ZenMode = zenMode
	if projectFilter != "" {
		ofs.ProjectFilters = []string{projectFilter}
	}

	// Load .dlignore from mountpoint directory
	igMatcher, err := ignore.ParseFile(filepath.Join(mountpoint, ".dlignore"))
	if err != nil {
		log.Printf("warning: failed to parse .dlignore: %v", err)
		igMatcher = ignore.New()
	}
	ofs.Ignore = igMatcher

	// Write PID file for sync command
	os.WriteFile(dav.PIDFile, fmt.Appendf(nil, "%d", os.Getpid()), 0644)
	defer os.Remove(dav.PIDFile)

	// Handle SIGUSR1 for zen mode sync
	syncCh := make(chan os.Signal, 1)
	signal.Notify(syncCh, syscall.SIGUSR1)
	go func() {
		for range syncCh {
			fmt.Println("\nSync requested — flushing dirty files...")
			flushed, errors := ofs.FlushAll()
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
			stats := ofs.DirtySummary()
			if len(stats) > 0 {
				printDirtySummary(stats)
			} else {
				fmt.Println("No local changes to sync.")
			}
		}

		fmt.Println("Flushing dirty files...")
		ofs.FlushAll()
		ofs.DisconnectAll()
		dav.Unmount(mountpoint)
		os.Remove(dav.PIDFile)
		os.Exit(0)
	}()

	// Start WebDAV server in background, then mount
	go func() {
		if err := dav.Serve(addr, ofs); err != nil {
			fmt.Fprintf(os.Stderr, "WebDAV server error: %v\n", err)
			os.Exit(1)
		}
	}()

	fmt.Printf("WebDAV server: http://%s\n", addr)
	fmt.Printf("Mounting at %s ...\n", mountpoint)

	if err := dav.MountNative(addr, mountpoint); err != nil {
		fmt.Printf("Auto-mount failed: %v\n", err)
		fmt.Printf("You can mount manually:\n")
		fmt.Printf("  macOS:  mount_webdav http://%s %s\n", addr, mountpoint)
		fmt.Printf("  Linux:  sudo mount -t davfs http://%s %s\n", addr, mountpoint)
		fmt.Printf("  Or open in Finder: Cmd+K → http://%s\n", addr)
	} else {
		fmt.Printf("Mounted at %s\n", mountpoint)
	}

	fmt.Println("Press Ctrl+C to stop.")
	select {} // block forever
}

// printDirtySummary displays a git diff --stat style summary of modified files.
func printDirtySummary(stats []dav.FileStat) {
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
