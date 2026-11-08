package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/database"
	"giki-sports-backend/internal/devuser"
)

func main() {
	devFlag := flag.Bool("dev", false, "Explicit opt-in for local development account provisioning")
	localDevFlag := flag.Bool("local-dev", false, "Alias for --dev")
	presetFlag := flag.String("preset", "", "Preset account to provision: 'director' or 'student'")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: provision-dev-user --dev --preset <director|student>\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nNote: This tool is strictly for local development environments.\n")
		fmt.Fprintf(os.Stderr, "It provisions synthetic development accounts (director@example.test or student@example.test).\n")
		fmt.Fprintf(os.Stderr, "Passwords must be entered interactively; noninteractive input is refused.\n")
	}
	flag.Parse()

	if !*devFlag && !*localDevFlag {
		fmt.Fprintln(os.Stderr, "[ERROR] Missing required flag: --dev (or --local-dev)")
		fmt.Fprintln(os.Stderr, "Development account provisioning requires explicit local-development opt-in.")
		fmt.Fprintln(os.Stderr, "Usage: go run ./cmd/provision-dev-user --dev --preset <director|student>")
		os.Exit(1)
	}

	presetDef, err := devuser.GetPreset(*presetFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		fmt.Fprintln(os.Stderr, "Usage: go run ./cmd/provision-dev-user --dev --preset <director|student>")
		os.Exit(1)
	}

	// Step 1: Load configuration without exposing credentials
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Step 2: Target Safeguards - Refuse non-loopback hosts and disallowed databases
	if err := devuser.ValidateTarget(cfg.DB.Host, cfg.DB.Name); err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] Database target safeguard rejected: %v\n", err)
		os.Exit(1)
	}

	// Show nonsecret database target
	fmt.Printf("Database Target:\n")
	fmt.Printf("  Host:     %s\n", cfg.DB.Host)
	fmt.Printf("  Port:     %d\n", cfg.DB.Port)
	fmt.Printf("  Database: %s\n", cfg.DB.Name)
	fmt.Printf("  User:     %s\n", cfg.DB.User)
	fmt.Printf("  SSLMode:  %s\n\n", cfg.DB.SSLMode)
	fmt.Println("Target safeguards verified: connection is directed to local loopback and the development database.")
	fmt.Println("Note: These checks are defensive safeguards against accidental misconfiguration, not cryptographic proof of the database's identity.")
	fmt.Println()

	// Step 3: Interactive Password Prompting without echo
	isTerminal := term.IsTerminal(int(os.Stdin.Fd()))
	password, err := devuser.ReadPasswordInteractive(isTerminal, func() ([]byte, error) {
		return term.ReadPassword(int(os.Stdin.Fd()))
	}, os.Stderr, presetDef.Email)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		os.Exit(1)
	}

	// Step 4: Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.DB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] Database connection failed: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Step 5: Provision user and role in an atomic transaction
	result, err := devuser.Provision(ctx, pool, presetDef.Preset, password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] Provisioning failed: %v\n", err)
		os.Exit(1)
	}

	if result.AlreadyExists {
		fmt.Println("\n[NOTICE] Account already exists in the database:")
		fmt.Printf("  User ID:      %s\n", result.UserID)
		fmt.Printf("  Email:        %s\n", result.Email)
		fmt.Printf("  Display Name: %s\n", result.DisplayName)
		if len(result.ExistingRoles) > 0 {
			var roleStrs []string
			for _, r := range result.ExistingRoles {
				roleStrs = append(roleStrs, string(r))
			}
			fmt.Printf("  Role(s):      %s\n", strings.Join(roleStrs, ", "))
		}
		fmt.Println("\nNo changes were made to the existing account.")
		fmt.Println("Password, status, and assigned roles remain unchanged.")
		fmt.Println("Existing accounts are never silently promoted or reset.")
		return
	}

	fmt.Println("\n✓ Account provisioned successfully in an atomic transaction.")
	fmt.Println("\nProvisioning Summary:")
	fmt.Printf("  User ID:      %s\n", result.UserID)
	fmt.Printf("  Email:        %s\n", result.Email)
	fmt.Printf("  Display Name: %s\n", result.DisplayName)
	fmt.Printf("  Role:         %s\n", result.Role)
	fmt.Printf("  Status:       active\n")
	fmt.Printf("  Created At:   %s\n", result.CreatedAt.Format(time.RFC3339))
	fmt.Println("\nNote: No session was created. You can now log in via POST /api/auth/login with these credentials.")
}
