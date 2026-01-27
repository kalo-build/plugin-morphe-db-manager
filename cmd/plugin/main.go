// WASM plugin entry point for the database manager.
// This plugin manages database schema through SQL migrations.
//
// Supported modes (via config.mode):
//
//   - "up" (default): Apply pending migrations
//     Production-safe: Yes
//     Applies base schema files and diff migrations that haven't been applied yet.
//
//   - "down": Drop all tables
//     Production-safe: NO - destroys all data
//     Drops all tables in the public schema.
//
//   - "refresh": Drop all tables and recreate
//     Production-safe: NO - destroys all data
//     Equivalent to running "down" then "up". Use for development/testing.
//
//   - "seed": Insert initial data
//     Production-safe: Yes (if scripts are idempotent)
//     Executes seed SQL scripts from the seed store.
//
//   - "reset": Reset to base schema only (dev-only)
//     Production-safe: NO - destroys all data and clears migration history
//     Drops all tables, clears migration tracking, re-applies only base schema.
//     Optionally deletes diff migration files. Use when you want to consolidate
//     during development - the codebase diff files become obsolete.
//
// Future modes (not yet implemented):
//   - "squash": Production-safe migration consolidation
//     Would mark a "squash point" in the migrations table without modifying the database.
//     Allows archiving old migration files while existing databases continue normally.
//     Similar to Laravel's schema:dump command.
//
// Supports two input modes:
//   - Single input: reads from /input mount (KA_MIGRATIONS store)
//   - Dual input: reads base schema from /schema mount, then diffs from /migrations mount
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	kalo "github.com/kalo-build/kalo-sdk-go"
	"github.com/kalo-build/plugin-morphe-db-manager/pkg/dbmigrate"
)

// Migration mode constants
const (
	ModeUp      = "up"
	ModeDown    = "down"
	ModeRefresh = "refresh"
	ModeSeed    = "seed"
	ModeReset   = "reset"

	// ModeFlatten is deprecated, use ModeReset instead
	// Kept for backwards compatibility
	ModeFlatten = "flatten"
)

func main() {
	// Get plugin config
	config := kalo.GetConfig()

	// Get mode (default: up)
	mode := ModeUp
	if m, ok := config["mode"].(string); ok && m != "" {
		mode = m
	}

	// Get options
	dryRun := false
	if dr, ok := config["dryRun"].(bool); ok {
		dryRun = dr
	}

	verbose := false
	if v, ok := config["verbose"].(bool); ok {
		verbose = v
	}

	// Get database store (output)
	outputStoreName := "DB_MAIN"
	if name, ok := config["outputStore"].(string); ok && name != "" {
		outputStoreName = name
	}
	dbStore := kalo.DB(outputStoreName)
	db := dbmigrate.NewDatabase(dbStore)

	opts := &dbmigrate.MigrateOptions{
		DryRun:  dryRun,
		Verbose: verbose,
	}

	if verbose {
		fmt.Printf("Running in mode: %s\n", mode)
		if dryRun {
			fmt.Println("DRY RUN MODE - no changes will be made")
		}
	}

	// Execute based on mode
	switch mode {
	case ModeDown:
		runDownMode(db, dryRun, verbose)
	case ModeRefresh:
		runRefreshMode(db, opts, verbose)
	case ModeSeed:
		runSeedMode(db, config, verbose)
	case ModeReset, ModeFlatten:
		// "flatten" is deprecated, use "reset" instead
		deleteFiles := false
		if df, ok := config["deleteFiles"].(bool); ok {
			deleteFiles = df
		}
		runResetMode(db, verbose, deleteFiles)
	case ModeUp:
		fallthrough
	default:
		runUpMode(db, opts, verbose, dryRun)
	}
}

// runUpMode creates all tables by applying migrations
func runUpMode(db *dbmigrate.Database, opts *dbmigrate.MigrateOptions, verbose, dryRun bool) {
	var totalApplied, totalSkipped int
	var allErrors []error

	// Check for dual-input mode (schema + migrations)
	schemaPath := "/schema"
	migrationsPath := "/migrations"

	schemaExists := dirExists(schemaPath)
	migrationsExists := dirExists(migrationsPath)

	if schemaExists || migrationsExists {
		// Dual input mode: apply schema first, then migrations
		if schemaExists {
			if verbose {
				fmt.Println("Applying base schema from /schema...")
				fmt.Println("Processing subdirectories: enums/, models/, structures/, entities/")
			}
			schemaFS := kalo.FS("KA_MO_PSQL")
			schemaSource := dbmigrate.NewPSQLSchemaMigrationSource(schemaFS, "schema:")
			result, err := dbmigrate.Migrate(schemaSource, db, opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Base schema migration failed: %v\n", err)
				os.Exit(1)
			}
			totalApplied += len(result.Applied)
			totalSkipped += len(result.Skipped)
			allErrors = append(allErrors, result.Errors...)
			printResults("base schema", result, dryRun, verbose)
		}

		if migrationsExists {
			if verbose {
				fmt.Println("Applying diff migrations from /migrations...")
			}
			migrationsFS := kalo.FS("KA_MIGRATIONS")
			migrationsSource := dbmigrate.NewPSQLSchemaMigrationSource(migrationsFS, "diff:")
			result, err := dbmigrate.Migrate(migrationsSource, db, opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Diff migration failed: %v\n", err)
				os.Exit(1)
			}
			totalApplied += len(result.Applied)
			totalSkipped += len(result.Skipped)
			allErrors = append(allErrors, result.Errors...)
			printResults("diff migrations", result, dryRun, verbose)
		}
	} else {
		// Single input mode (legacy): read from /input
		if verbose {
			fmt.Println("Applying migrations from /input...")
		}
		inputStoreName := "KA_MIGRATIONS"
		if cfg := kalo.GetConfig(); cfg != nil {
			if name, ok := cfg["inputStore"].(string); ok && name != "" {
				inputStoreName = name
			}
		}
		fs := kalo.FS(inputStoreName)
		source := dbmigrate.NewSDKMigrationSource(fs)
		result, err := dbmigrate.Migrate(source, db, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
			os.Exit(1)
		}
		totalApplied = len(result.Applied)
		totalSkipped = len(result.Skipped)
		allErrors = result.Errors
		printResults("migrations", result, dryRun, verbose)
	}

	// Final summary
	if len(allErrors) > 0 {
		fmt.Fprintf(os.Stderr, "Encountered %d error(s)\n", len(allErrors))
		os.Exit(1)
	}

	if totalApplied == 0 && len(allErrors) == 0 {
		fmt.Println("No migrations to apply")
	} else if !dryRun {
		fmt.Printf("All migrations applied successfully (%d applied, %d skipped)\n", totalApplied, totalSkipped)
	}
}

// runDownMode deletes all tables
func runDownMode(db *dbmigrate.Database, dryRun, verbose bool) {
	if dryRun {
		fmt.Println("Would drop all tables in public schema")
		return
	}

	fmt.Println("Dropping all tables...")

	if err := db.DropAllTables(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to drop tables: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("All tables dropped successfully")
}

// runRefreshMode deletes all tables and recreates them
func runRefreshMode(db *dbmigrate.Database, opts *dbmigrate.MigrateOptions, verbose bool) {
	if opts.DryRun {
		fmt.Println("Would drop all tables and recreate them")
		return
	}

	fmt.Println("Refreshing database (drop all tables, then recreate)...")

	// Drop all tables
	if err := db.DropAllTables(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to drop tables: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("All tables dropped.")

	// Apply all migrations from scratch
	fmt.Println("Recreating tables...")
	runUpMode(db, opts, verbose, false)
}

// runSeedMode fills in initial data
func runSeedMode(db *dbmigrate.Database, config map[string]interface{}, verbose bool) {
	fmt.Println("Seeding database with initial data...")

	// Read seed files from the seed store (mounted at /seed via inputs.seed)
	seedStoreName := "KA_SEED"
	if name, ok := config["seedStore"].(string); ok && name != "" {
		seedStoreName = name
	}

	// Access the seed store via the WASI filesystem
	// The CLI mounts inputs.seed at /seed
	seedPath := "/seed"

	// Try to list the seed directory
	entries, err := os.ReadDir(seedPath)
	if err != nil {
		fmt.Printf("Seed directory not mounted at %s: %v\n", seedPath, err)
		fmt.Println("Make sure the 'seed' input is configured in kalo.yaml")
		return
	}

	if len(entries) == 0 {
		fmt.Println("No seed files found")
		return
	}

	// Sort entries to ensure consistent ordering
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)

	// Also try the kalo.FS approach for backwards compatibility
	fs := kalo.FS(seedStoreName)

	for _, name := range files {
		if verbose {
			fmt.Printf("Executing seed file: %s\n", name)
		}

		// Try reading via kalo.FS first, fall back to direct file read
		var content []byte
		content, err = fs.ReadFile(name)
		if err != nil {
			// Fall back to direct file read
			content, err = os.ReadFile(seedPath + "/" + name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read seed file %s: %v\n", name, err)
				continue
			}
		}

		if err := db.Exec(content); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to execute seed file %s: %v\n", name, err)
			os.Exit(1)
		}

		fmt.Printf("Applied seed: %s\n", name)
	}

	fmt.Println("Database seeded successfully")
}

// runResetMode resets the database to base schema only.
// WARNING: This is a dev-only operation that destroys all data.
//
// Steps:
// 1. (Optional) Delete all diff migration files from /migrations
// 2. Clear migration tracking table
// 3. Drop all tables
// 4. Re-apply only the base schema (no diffs)
//
// Use case: During development, when you want to consolidate migrations and
// start fresh with just the base schema. The diff files become obsolete
// after this operation.
func runResetMode(db *dbmigrate.Database, verbose bool, deleteFiles bool) {
	fmt.Println("Resetting database to base schema...")
	fmt.Println()
	fmt.Println("WARNING: This is a dev-only operation that destroys all data!")
	fmt.Println()
	fmt.Println("This will:")
	if deleteFiles {
		fmt.Println("  1. Delete all diff migration files")
	}
	fmt.Println("  2. Clear migration tracking")
	fmt.Println("  3. Drop all tables")
	fmt.Println("  4. Re-apply only the base schema")
	fmt.Println()

	// Step 1: Optionally delete diff migration files from /migrations
	migrationsPath := "/migrations"
	if deleteFiles && dirExists(migrationsPath) {
		entries, err := os.ReadDir(migrationsPath)
		if err == nil {
			deletedCount := 0
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if !strings.HasSuffix(name, ".sql") {
					continue
				}
				// Delete the migration file
				filePath := migrationsPath + "/" + name
				if err := os.Remove(filePath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not delete %s: %v\n", name, err)
				} else {
					if verbose {
						fmt.Printf("Deleted: %s\n", name)
					}
					deletedCount++
				}
			}
			fmt.Printf("Deleted %d diff migration files\n", deletedCount)
		}
	}

	// Step 2: Clear migration tracking table
	fmt.Println("Clearing migration tracking...")
	if err := db.ClearMigrations(); err != nil {
		// Table might not exist, that's ok
		if verbose {
			fmt.Printf("Note: could not clear migrations: %v\n", err)
		}
	}

	// Step 3: Drop all tables
	fmt.Println("Dropping all tables...")
	if err := db.DropAllTables(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to drop tables: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("All tables dropped.")

	// Step 4: Re-apply only base schema (no diffs)
	fmt.Println("Re-applying base schema...")
	schemaPath := "/schema"
	if !dirExists(schemaPath) {
		fmt.Println("No base schema found at /schema, skipping re-apply")
		fmt.Println("Reset complete - database is now empty")
		return
	}

	// Ensure tracking table exists
	if err := db.EnsureTrackingTable(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create tracking table: %v\n", err)
		os.Exit(1)
	}

	// Create a source for just the base schema using kalo.FS
	schemaFS := kalo.FS("KA_MO_PSQL")
	schemaSource := dbmigrate.NewPSQLSchemaMigrationSource(schemaFS, "schema:")
	schemaMigrations, err := schemaSource.ListMigrations()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list schema files: %v\n", err)
		os.Exit(1)
	}

	appliedCount := 0
	for _, m := range schemaMigrations {
		sql, err := schemaSource.ReadMigration(m.Name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read schema file %s: %v\n", m.Name, err)
			os.Exit(1)
		}
		if err := db.ApplyMigration(m.Name, sql); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to apply schema %s: %v\n", m.Name, err)
			os.Exit(1)
		}
		appliedCount++
	}

	fmt.Printf("Applied %d base schema files\n", appliedCount)
	fmt.Println("Reset complete - database now matches base schema only")
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func printResults(source string, result *dbmigrate.MigrateResult, dryRun, verbose bool) {
	if len(result.Applied) > 0 {
		if dryRun {
			fmt.Printf("Would apply %d %s:\n", len(result.Applied), source)
		} else {
			fmt.Printf("Applied %d %s:\n", len(result.Applied), source)
		}
		for _, name := range result.Applied {
			fmt.Printf("  - %s\n", name)
		}
	}

	if verbose && len(result.Skipped) > 0 {
		fmt.Printf("Skipped %d %s (already applied):\n", len(result.Skipped), source)
		for _, name := range result.Skipped {
			fmt.Printf("  - %s\n", name)
		}
	}

	if len(result.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "Errors in %s:\n", source)
		for _, err := range result.Errors {
			fmt.Fprintf(os.Stderr, "  - %v\n", err)
		}
	}
}
