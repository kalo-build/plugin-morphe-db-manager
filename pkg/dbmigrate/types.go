// Package dbmigrate provides database migration functionality.
package dbmigrate

// MigrationFile represents a migration file on disk.
type MigrationFile struct {
	Name     string
	Checksum string
}

// AppliedMigration represents a migration that has been applied.
type AppliedMigration struct {
	Name      string
	Checksum  string
	AppliedAt int64
}

// MigrationSource provides access to migration files.
type MigrationSource interface {
	// ListMigrations returns all available migrations, sorted by name.
	ListMigrations() ([]MigrationFile, error)
	// ReadMigration reads the SQL content of a migration.
	ReadMigration(name string) ([]byte, error)
}

// MigrationExecutor executes migrations against a database.
type MigrationExecutor interface {
	// EnsureTrackingTable creates the migration tracking table if it doesn't exist.
	EnsureTrackingTable() error
	// GetApplied returns all applied migrations.
	GetApplied() ([]AppliedMigration, error)
	// Apply executes a migration.
	Apply(name string, checksum string, sql []byte) error
}

