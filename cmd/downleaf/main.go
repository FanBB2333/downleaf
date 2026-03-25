package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/FanBB2333/downleaf/internal/api"
	"github.com/FanBB2333/downleaf/internal/auth"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		fmt.Println("warning: no .env file found, using environment variables")
	}

	siteURL := os.Getenv("SITE")
	cookies := os.Getenv("COOKIES")

	if siteURL == "" || cookies == "" {
		return fmt.Errorf("SITE and COOKIES must be set in .env or environment")
	}

	// Authenticate with cookies
	fmt.Printf("Authenticating with %s ...\n", siteURL)
	identity, err := auth.LoginWithCookies(siteURL, cookies)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	fmt.Printf("Authenticated as: %s (user: %s)\n", identity.Email, identity.UserID)

	// Create API client and list projects
	client := api.NewClient(siteURL, identity)
	projects, err := client.ListProjects()
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	fmt.Printf("\nFound %d projects:\n", len(projects))
	for _, p := range projects {
		status := ""
		if p.Archived {
			status = " [archived]"
		}
		if p.Trashed {
			status = " [trashed]"
		}
		fmt.Printf("  - %s (id: %s)%s\n", p.Name, p.ID, status)
	}

	// Test: get file tree for the first project
	if len(projects) > 0 {
		p := projects[0]
		fmt.Printf("\nFile tree for '%s':\n", p.Name)
		entities, err := client.GetProjectEntities(p.ID)
		if err != nil {
			return fmt.Errorf("get entities: %w", err)
		}
		for _, e := range entities {
			fmt.Printf("  [%s] %s\n", e.Type, e.Path)
		}
	}

	return nil
}
