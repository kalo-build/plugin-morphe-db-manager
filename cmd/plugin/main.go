// WASM plugin entry point for the database manager.
// This plugin applies SQL migrations from a filesystem store to a database store.
package main

import (
	"fmt"
	"os"

	kalo "github.com/kalo-build/kalo-sdk-go"
	"github.com/kalo-build/plugin-morphe-db-manager/pkg/dbmigrate"
)

func main() {
	// Get store names from plugin config
	config := kalo.GetConfig()

	// Default store names (can be overridden via config)
	inputStoreName := "KA_MIGRATIONS"
	outputStoreName := "DB_MAIN"

	if name, ok := config["inputStore"].(string); ok && name != "" {
		inputStoreName = name
	}
	if name, ok := config["outputStore"].(string); ok && name != "" {
		outputStoreName = name
	}

	// Get dry run option
	dryRun := false
	if dr, ok := config["dryRun"].(bool); ok {
		dryRun = dr
	}

	verbose := false
	if v, ok := config["verbose"].(bool); ok {
		verbose = v
	}

	// Get stores from SDK
	fs := kalo.FS(inputStoreName)
	db := kalo.DB(outputStoreName)

	// Create source and executor
	source := dbmigrate.NewSDKMigrationSource(fs)
	executor := dbmigrate.NewSDKMigrationExecutor(db)

	// Run migrations
	opts := &dbmigrate.MigrateOptions{
		DryRun:  dryRun,
		Verbose: verbose,
	}

	if verbose {
		fmt.Printf("Starting migration from store '%s' to store '%s'\n", inputStoreName, outputStoreName)
		if dryRun {
			fmt.Println("DRY RUN MODE - no changes will be made")
		}
	}

	result, err := dbmigrate.Migrate(source, executor, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
		os.Exit(1)
	}

	// Print results
	if len(result.Applied) > 0 {
		if dryRun {
			fmt.Printf("Would apply %d migration(s):\n", len(result.Applied))
		} else {
			fmt.Printf("Applied %d migration(s):\n", len(result.Applied))
		}
		for _, name := range result.Applied {
			fmt.Printf("  - %s\n", name)
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
			fmt.Fprintf(os.Stderr, "  - %v\n", err)
		}
		os.Exit(1)
	}

	if len(result.Applied) == 0 && len(result.Errors) == 0 {
		fmt.Println("No migrations to apply")
	} else if !dryRun {
		fmt.Println("Migrations applied successfully")
	}
}

