// Standalone CLI for database migrations.
// This is a thin wrapper around the dbmigrate library for use outside of Kalo pipelines.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kalo-build/plugin-morphe-db-manager/pkg/dbmigrate"
)

func main() {
	var (
		migrationsDir  string
		dsn            string
		dryRun         bool
		verbose        bool
		status         bool
		showHelp       bool
		nonInteractive bool
	)

	flag.StringVar(&migrationsDir, "migrations", "./migrations", "Path to migrations directory")
	flag.StringVar(&migrationsDir, "m", "./migrations", "Path to migrations directory (shorthand)")
	flag.StringVar(&dsn, "dsn", "", "PostgreSQL connection string (or use DATABASE_URL env var)")
	flag.BoolVar(&dryRun, "dry-run", false, "Show what would be executed without running")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose output")
	flag.BoolVar(&verbose, "v", false, "Enable verbose output (shorthand)")
	flag.BoolVar(&status, "status", false, "Show current migration status")
	flag.BoolVar(&showHelp, "help", false, "Show help information")
	flag.BoolVar(&showHelp, "h", false, "Show help information (shorthand)")
	flag.BoolVar(&nonInteractive, "non-interactive", false, "Run without user prompts (for CI/CD)")

	flag.Parse()

	if showHelp {
		printHelp()
		return
	}

	// Get DSN from flag or environment
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		log.Fatal("Database connection string required. Use --dsn flag or DATABASE_URL environment variable.")
	}

	// Validate migrations directory
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		log.Fatalf("Migrations directory does not exist: %s", migrationsDir)
	}

	ctx := context.Background()

	// Connect to database
	if verbose {
		log.Printf("Connecting to database...")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	if verbose {
		log.Printf("Connected to database successfully")
		log.Printf("Migrations directory: %s", migrationsDir)
	}

	// Create source and executor
	source := dbmigrate.NewDirectFileSource(migrationsDir)
	executor := dbmigrate.NewDirectDBExecutor(ctx, pool)

	// Handle status command
	if status {
		showMigrationStatus(source, executor, verbose)
		return
	}

	// Run migrations
	opts := &dbmigrate.MigrateOptions{
		DryRun:  dryRun,
		Verbose: verbose,
	}

	if verbose {
		log.Println("Starting migration...")
		if dryRun {
			log.Println("DRY RUN MODE - no changes will be made")
		}
	}

	result, err := dbmigrate.Migrate(source, executor, opts)
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	// Print results
	if len(result.Applied) > 0 {
		if dryRun {
			fmt.Printf("Would apply %d migration(s):\n", len(result.Applied))
		} else {
			fmt.Printf("Applied %d migration(s):\n", len(result.Applied))
		}
		for _, name := range result.Applied {
			fmt.Printf("  ✓ %s\n", name)
		}
	}

	if verbose && len(result.Skipped) > 0 {
		fmt.Printf("Skipped %d migration(s) (already applied):\n", len(result.Skipped))
		for _, name := range result.Skipped {
			fmt.Printf("  - %s\n", name)
		}
	}

	if len(result.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "Encountered %d error(s):\n", len(result.Errors))
		for _, err := range result.Errors {
			fmt.Fprintf(os.Stderr, "  ✗ %v\n", err)
		}
		os.Exit(1)
	}

	if len(result.Applied) == 0 && len(result.Errors) == 0 {
		fmt.Println("No migrations to apply - database is up to date")
	} else if !dryRun && len(result.Applied) > 0 {
		fmt.Println("Migrations applied successfully")
	}
}

func showMigrationStatus(source dbmigrate.MigrationSource, executor dbmigrate.MigrationExecutor, verbose bool) {
	// Ensure tracking table exists
	if err := executor.EnsureTrackingTable(); err != nil {
		log.Printf("Warning: Could not ensure tracking table: %v", err)
	}

	// Get applied migrations
	applied, err := executor.GetApplied()
	if err != nil {
		log.Fatalf("Failed to get applied migrations: %v", err)
	}

	appliedSet := make(map[string]dbmigrate.AppliedMigration)
	for _, m := range applied {
		appliedSet[m.Name] = m
	}

	// Get available migrations
	available, err := source.ListMigrations()
	if err != nil {
		log.Fatalf("Failed to list migrations: %v", err)
	}

	fmt.Println("Migration Status")
	fmt.Println("================")
	fmt.Printf("Applied: %d\n", len(applied))
	fmt.Printf("Available: %d\n", len(available))
	fmt.Println()

	pending := 0
	checksumMismatch := 0

	for _, m := range available {
		if appliedMig, ok := appliedSet[m.Name]; ok {
			if appliedMig.Checksum != m.Checksum {
				fmt.Printf("  ⚠ %s (CHECKSUM MISMATCH)\n", m.Name)
				checksumMismatch++
			} else if verbose {
				fmt.Printf("  ✓ %s (applied)\n", m.Name)
			}
		} else {
			fmt.Printf("  ○ %s (pending)\n", m.Name)
			pending++
		}
	}

	fmt.Println()
	if pending > 0 {
		fmt.Printf("Pending migrations: %d\n", pending)
	} else {
		fmt.Println("All migrations have been applied")
	}

	if checksumMismatch > 0 {
		fmt.Printf("WARNING: %d migration(s) have checksum mismatches\n", checksumMismatch)
	}
}

func printHelp() {
	fmt.Println("Database Manager CLI")
	fmt.Println("====================")
	fmt.Println("A CLI tool for applying database migrations")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  dbmanager [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -m, --migrations <path>  Path to migrations directory (default: ./migrations)")
	fmt.Println("  --dsn <string>           PostgreSQL connection string")
	fmt.Println("  --dry-run                Show what would be executed without running")
	fmt.Println("  -v, --verbose            Enable verbose output")
	fmt.Println("  --status                 Show current migration status")
	fmt.Println("  --non-interactive        Run without user prompts (for CI/CD)")
	fmt.Println("  -h, --help               Show this help information")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  dbmanager --dsn \"postgres://user:pass@localhost/db\"")
	fmt.Println("  dbmanager -m ./db/migrations --dry-run")
	fmt.Println("  dbmanager --status")
	fmt.Println("  DATABASE_URL=\"postgres://...\" dbmanager")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  DATABASE_URL    PostgreSQL connection string (alternative to --dsn)")
}

