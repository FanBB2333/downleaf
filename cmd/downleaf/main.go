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
		return fmt.Errorf("unknown command: %s\nUsage: downleaf [ls|mount|umount] [mountpoint]", cmd)
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
