// Package dbmigrate provides database migration functionality.
// All migration tracking logic is encapsulated here - the SDK only provides
// generic database access (Exec, Query).
package dbmigrate

// MigrationFile represents a migration file on disk.
type MigrationFile struct {
	Name     string // Name used for tracking (may include prefix like "schema:")
	Path     string // Actual file path for reading (optional, defaults to Name)
	Checksum string
}

// AppliedMigration represents a migration that has been applied.
type AppliedMigration struct {
	Name      string `json:"name"`
	Checksum  string `json:"checksum"`
	AppliedAt int64  `json:"applied_at"`
}

// MigrationSource provides access to migration files.
type MigrationSource interface {
	// ListMigrations returns all available migrations, sorted by name.
	ListMigrations() ([]MigrationFile, error)
	// ReadMigration reads the SQL content of a migration.
	ReadMigration(name string) ([]byte, error)
}
