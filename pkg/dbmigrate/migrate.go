package dbmigrate

import (
	"fmt"
)

// MigrateOptions configures the migration behavior.
type MigrateOptions struct {
	DryRun  bool
	Verbose bool
}

// MigrateResult contains the result of a migration run.
type MigrateResult struct {
	Applied []string
	Skipped []string
	Errors  []error
}

// Migrate applies all pending migrations from source using executor.
func Migrate(source MigrationSource, executor MigrationExecutor, opts *MigrateOptions) (*MigrateResult, error) {
	if opts == nil {
		opts = &MigrateOptions{}
	}

	result := &MigrateResult{
		Applied: make([]string, 0),
		Skipped: make([]string, 0),
		Errors:  make([]error, 0),
	}

	// Ensure tracking table exists
	if err := executor.EnsureTrackingTable(); err != nil {
		return nil, fmt.Errorf("failed to ensure tracking table: %w", err)
	}

	// Get applied migrations
	applied, err := executor.GetApplied()
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Build set of applied migration names
	appliedSet := make(map[string]AppliedMigration)
	for _, m := range applied {
		appliedSet[m.Name] = m
	}

	// Get all available migrations
	migrations, err := source.ListMigrations()
	if err != nil {
		return nil, fmt.Errorf("failed to list migrations: %w", err)
	}

	// Apply pending migrations
	for _, m := range migrations {
		if existing, ok := appliedSet[m.Name]; ok {
			// Already applied - check checksum
			if existing.Checksum != m.Checksum {
				result.Errors = append(result.Errors,
					fmt.Errorf("migration %s has changed (checksum mismatch)", m.Name))
			}
			result.Skipped = append(result.Skipped, m.Name)
			continue
		}

		if opts.DryRun {
			result.Applied = append(result.Applied, m.Name)
			continue
		}

		// Read migration SQL
		sql, err := source.ReadMigration(m.Name)
		if err != nil {
			result.Errors = append(result.Errors,
				fmt.Errorf("failed to read migration %s: %w", m.Name, err))
			continue
		}

		// Apply migration
		if err := executor.Apply(m.Name, m.Checksum, sql); err != nil {
			result.Errors = append(result.Errors,
				fmt.Errorf("failed to apply migration %s: %w", m.Name, err))
			// Stop on first error
			return result, fmt.Errorf("migration failed: %w", err)
		}

		result.Applied = append(result.Applied, m.Name)
	}

	return result, nil
}

