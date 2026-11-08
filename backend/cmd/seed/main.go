package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/database"
	"giki-sports-backend/internal/seed"
)

func main() {
	demoFlag := flag.Bool("demo", false, "Seed demonstration facilities and recurring schedules for development")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: seed [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nNote: This command is strictly for local development and demonstration.\n")
		fmt.Fprintf(os.Stderr, "It populates synthetic demonstration facilities and schedules labeled '[Demo]'.\n")
		fmt.Fprintf(os.Stderr, "It does not represent approved institutional GIKI data.\n")
	}
	flag.Parse()

	if !*demoFlag {
		fmt.Fprintln(os.Stderr, "[ERROR] Missing required flag: --demo")
		fmt.Fprintln(os.Stderr, "Seeding is restricted to development demo data and requires explicit opt-in.")
		fmt.Fprintln(os.Stderr, "Usage: go run ./cmd/seed --demo")
		os.Exit(1)
	}

	// Step 1: Load configuration without exposing credentials
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Failed to load configuration: %v", err)
	}

	// Step 2: Connect to PostgreSQL safely via pgx connection pool
	log.Printf("Connecting to PostgreSQL at %s:%d (database: %s, user: %s, sslmode: %s)...",
		cfg.DB.Host, cfg.DB.Port, cfg.DB.Name, cfg.DB.User, cfg.DB.SSLMode)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.DB)
	if err != nil {
		log.Fatalf("[FATAL] Database connection failed: %v", err)
	}
	defer pool.Close()

	// Step 3: Apply demonstration fixtures within an atomic transaction
	log.Println("Applying demonstration fixtures within an atomic transaction...")
	summary, err := seed.ApplyDemo(ctx, pool)
	if err != nil {
		log.Fatalf("[FATAL] Demonstration seeding failed: %v", err)
	}

	log.Println("✓ Demonstration seeding completed successfully.")
	fmt.Printf("\nSeed Summary:\n")
	fmt.Printf("  Facilities: %d inserted, %d skipped\n", summary.FacilitiesInserted, summary.FacilitiesSkipped)
	fmt.Printf("  Schedules:  %d inserted, %d skipped\n", summary.SchedulesInserted, summary.SchedulesSkipped)
}
