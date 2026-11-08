package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"giki-sports-backend/internal/config"
	"giki-sports-backend/internal/database"
)

const defaultMigrationsDir = "migrations"

func main() {
	var dirFlag string
	flag.StringVar(&dirFlag, "dir", "", "Path to migrations directory (default: searches ./migrations or ../migrations)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: migrate [flags] <command>\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  status   Print current migration status and version history\n")
		fmt.Fprintf(os.Stderr, "  up       Apply all pending migrations\n")
		fmt.Fprintf(os.Stderr, "  down     Roll back the most recent single migration\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	command := strings.ToLower(args[0])
	switch command {
	case "status", "up", "down":
	default:
		log.Fatalf("[FATAL] Unknown migration command '%s'. Supported: status, up, down", command)
	}

	// Step 1: Load configuration without exposing credentials
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Failed to load configuration: %v", err)
	}

	// Step 2: Determine migrations directory path
	migrationsDir := resolveMigrationsDir(dirFlag)
	if migrationsDir == "" {
		log.Fatalf("[FATAL] Migrations directory could not be found. Please specify -dir or run from backend/")
	}

	// Step 3: Connect to PostgreSQL safely via standard library driver
	log.Printf("Connecting to PostgreSQL at %s:%d (database: %s, user: %s, sslmode: %s)...",
		cfg.DB.Host, cfg.DB.Port, cfg.DB.Name, cfg.DB.User, cfg.DB.SSLMode)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := database.NewSQLDB(ctx, cfg.DB)
	if err != nil {
		log.Fatalf("[FATAL] Database connection failed: %v", err)
	}
	defer db.Close()

	// Step 4: Construct shared MigrationRunner with PostgreSQL advisory locking
	runner, err := database.NewMigrationRunnerFromDir(db, migrationsDir)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize migration runner: %v", err)
	}
	defer runner.Close()

	log.Printf("Running migration command '%s' with PostgreSQL session advisory locking using directory: %s", command, migrationsDir)

	switch command {
	case "status":
		statusList, err := runner.Status(ctx)
		if err != nil {
			log.Fatalf("[FATAL] Migration status failed: %v", err)
		}
		fmt.Println("    Applied At                  Migration")
		fmt.Println("    =======================================")
		for _, s := range statusList {
			name := filepath.Base(s.Source.Path)
			if s.State == goose.StateApplied {
				fmt.Printf("    %-27s -- %s\n", s.AppliedAt.Format(time.ANSIC), name)
			} else {
				fmt.Printf("    %-27s -- %s\n", "Pending", name)
			}
		}
	case "up":
		results, err := runner.Up(ctx)
		if err != nil {
			log.Fatalf("[FATAL] Migration up failed: %v", err)
		}
		if len(results) == 0 {
			log.Println("goose: no migrations to run.")
		} else {
			for _, res := range results {
				log.Printf("OK   %s (%s)", filepath.Base(res.Source.Path), res.Duration)
			}
		}
		log.Println("✓ Migrations applied successfully.")
	case "down":
		res, err := runner.Down(ctx)
		if err != nil {
			log.Fatalf("[FATAL] Migration down failed: %v", err)
		}
		if res != nil {
			log.Printf("OK   %s (%s)", filepath.Base(res.Source.Path), res.Duration)
		}
		log.Println("✓ Single migration rolled back successfully.")
	}
}

// resolveMigrationsDir finds the migrations directory predictably.
func resolveMigrationsDir(explicit string) string {
	if explicit != "" {
		if stat, err := os.Stat(explicit); err == nil && stat.IsDir() {
			return explicit
		}
		return ""
	}

	candidates := []string{
		"migrations",
		filepath.Join(".", "migrations"),
		filepath.Join("..", "migrations"),
		filepath.Join("backend", "migrations"),
	}
	for _, c := range candidates {
		if stat, err := os.Stat(c); err == nil && stat.IsDir() {
			return c
		}
	}
	return ""
}
