package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/FanBB2333/downleaf/internal/api"
	"github.com/FanBB2333/downleaf/internal/auth"
	downfuse "github.com/FanBB2333/downleaf/internal/fuse"
	"github.com/FanBB2333/downleaf/internal/model"
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

	// Parse subcommand
	cmd := "ls"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
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
		mountpoint := filepath.Join(os.Getenv("HOME"), "overleaf")
		if len(os.Args) > 2 {
			mountpoint = os.Args[2]
		}
		return cmdMount(client, mountpoint)
	case "umount", "unmount":
		mountpoint := filepath.Join(os.Getenv("HOME"), "overleaf")
		if len(os.Args) > 2 {
			mountpoint = os.Args[2]
		}
		return downfuse.Unmount(mountpoint)
	default:
		fmt.Println("Usage: downleaf <command> [args]")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  ls                          List all projects")
		fmt.Println("  tree <project-id>           Show project file tree")
		fmt.Println("  cat <project-id> <doc-id>   Print document content")
		fmt.Println("  mount [mountpoint]          Mount filesystem (default: ~/overleaf)")
		fmt.Println("  umount [mountpoint]         Unmount filesystem")
		return nil
	}
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
		return fmt.Errorf("join doc: %w", err)
	}

	fmt.Printf("--- version: %d ---\n", version)
	fmt.Println(content)
	return nil
}

func cmdMount(client *api.Client, mountpoint string) error {
	// Handle Ctrl+C for clean unmount
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nUnmounting...")
		downfuse.Unmount(mountpoint)
		os.Exit(0)
	}()

	fmt.Printf("Mounting at %s ...\n", mountpoint)
	return downfuse.Mount(mountpoint, client)
}
