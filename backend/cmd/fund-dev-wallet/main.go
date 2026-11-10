package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/database"
	"giki-sports-backend/internal/devuser"
	"giki-sports-backend/internal/devwallet"
)

func main() {
	devFlag := flag.Bool("dev", false, "Explicit opt-in for local development wallet funding")
	localDevFlag := flag.Bool("local-dev", false, "Alias for --dev")
	presetFlag := flag.String("preset", "", "Preset account to fund: 'director' or 'student'")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: fund-dev-wallet --dev --preset <director|student>\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nNote: This tool is strictly for local development environments.\n")
		fmt.Fprintf(os.Stderr, "It provisions and funds an existing synthetic development account's Sports wallet\n")
		fmt.Fprintf(os.Stderr, "with 50,000 paisa (PKR 500.00) of synthetic test credit.\n")
		fmt.Fprintf(os.Stderr, "Rerunning the command is idempotent and returns the original transaction.\n")
	}
	flag.Parse()

	if !*devFlag && !*localDevFlag {
		fmt.Fprintln(os.Stderr, "[ERROR] Missing required flag: --dev (or --local-dev)")
		fmt.Fprintln(os.Stderr, "Development wallet funding requires explicit local-development opt-in.")
		fmt.Fprintln(os.Stderr, "Usage: go run ./cmd/fund-dev-wallet --dev --preset <director|student>")
		os.Exit(1)
	}

	presetDef, err := devuser.GetPreset(*presetFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		fmt.Fprintln(os.Stderr, "Usage: go run ./cmd/fund-dev-wallet --dev --preset <director|student>")
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

	// Step 3: Connect to PostgreSQL via connection pool
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.DB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] Database connection failed: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Step 4: Fund dev wallet
	result, err := devwallet.FundDevWallet(ctx, pool, presetDef.Preset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] Wallet funding failed: %v\n", err)
		os.Exit(1)
	}

	rupees := float64(result.AmountPaisa) / 100.0
	currentRupees := float64(result.CurrentBalance) / 100.0

	if result.AlreadyPosted {
		fmt.Println("[NOTICE] Synthetic test credit was already posted for this user:")
		fmt.Printf("  User ID:         %s\n", result.User.ID)
		fmt.Printf("  Email:           %s\n", result.User.Email)
		fmt.Printf("  Display Name:    %s\n", result.User.DisplayName)
		fmt.Printf("  Personal Wallet: %s\n", result.UserWalletID)
		fmt.Printf("  Transaction ID:  %s\n", result.TransactionID)
		fmt.Printf("  Credit Amount:   PKR %.2f (%d paisa)\n", rupees, result.AmountPaisa)
		fmt.Printf("  Current Balance: PKR %.2f (%d paisa)\n", currentRupees, result.CurrentBalance)
		fmt.Printf("  Status:          Original transaction returned. No duplicate funds added.\n")
		return
	}

	fmt.Println("✓ Sports wallet funded successfully with synthetic test credit.")
	fmt.Println("\nFunding Summary:")
	fmt.Printf("  User ID:         %s\n", result.User.ID)
	fmt.Printf("  Email:           %s\n", result.User.Email)
	fmt.Printf("  Display Name:    %s\n", result.User.DisplayName)
	fmt.Printf("  Personal Wallet: %s\n", result.UserWalletID)
	fmt.Printf("  Clearing Wallet: %s\n", result.ClearingWalletID)
	fmt.Printf("  Transaction ID:  %s\n", result.TransactionID)
	fmt.Printf("  Credit Amount:   PKR %.2f (%d paisa)\n", rupees, result.AmountPaisa)
	fmt.Printf("  Current Balance: PKR %.2f (%d paisa)\n", currentRupees, result.CurrentBalance)
	fmt.Printf("  Status:          New double-entry transfer committed.\n")
}

