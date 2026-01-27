package dbmigrate

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	kalo "github.com/kalo-build/kalo-sdk-go"
)

// Database wraps a DBStore and provides migration-specific operations.
// All migration tracking logic is encapsulated here.
type Database struct {
	db kalo.DBStore
}

// NewDatabase creates a new Database wrapper.
func NewDatabase(db kalo.DBStore) *Database {
	return &Database{db: db}
}

// EnsureTrackingTable creates the migration tracking table if it doesn't exist.
func (d *Database) EnsureTrackingTable() error {
	return d.db.Exec([]byte(`
		CREATE TABLE IF NOT EXISTS kalo_migrations (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`))
}

// GetAppliedMigrations returns all applied migrations from the tracking table.
func (d *Database) GetAppliedMigrations() ([]AppliedMigration, error) {
	result, err := d.db.Query([]byte(`
		SELECT name, checksum, EXTRACT(EPOCH FROM applied_at)::bigint as applied_at
		FROM kalo_migrations
		ORDER BY name ASC
	`))
	if err != nil {
		// Table might not exist yet, return empty
		return []AppliedMigration{}, nil
	}

	var migrations []AppliedMigration
	if err := json.Unmarshal(result, &migrations); err != nil {
		return nil, fmt.Errorf("failed to parse migrations: %w", err)
	}

	return migrations, nil
}

// ApplyMigration executes a migration and records it in the tracking table.
func (d *Database) ApplyMigration(name string, sql []byte) error {
	// Execute the migration SQL
	if err := d.db.Exec(sql); err != nil {
		return fmt.Errorf("migration SQL failed: %w", err)
	}

	// Record the migration in the tracking table
	checksum := ComputeChecksum(sql)
	recordSQL := fmt.Sprintf(`
		INSERT INTO kalo_migrations (name, checksum, applied_at)
		VALUES ('%s', '%s', now())
	`, escapeSQLString(name), checksum)

	if err := d.db.Exec([]byte(recordSQL)); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return nil
}

// ClearMigrations removes all migration records from the tracking table.
func (d *Database) ClearMigrations() error {
	return d.db.Exec([]byte(`DELETE FROM kalo_migrations`))
}

// DropAllTables drops all tables in the public schema.
func (d *Database) DropAllTables() error {
	return d.db.Exec([]byte(`
		DO $$ DECLARE
			r RECORD;
		BEGIN
			-- Drop all tables with CASCADE
			FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname = 'public') LOOP
				EXECUTE 'DROP TABLE IF EXISTS public.' || quote_ident(r.tablename) || ' CASCADE';
			END LOOP;
			
			-- Drop all types (enums)
			FOR r IN (SELECT typname FROM pg_type t JOIN pg_namespace n ON t.typnamespace = n.oid WHERE n.nspname = 'public' AND t.typtype = 'e') LOOP
				EXECUTE 'DROP TYPE IF EXISTS public.' || quote_ident(r.typname) || ' CASCADE';
			END LOOP;
		END $$;
	`))
}

// Exec executes arbitrary SQL (passthrough to SDK).
func (d *Database) Exec(sql []byte) error {
	return d.db.Exec(sql)
}

// ComputeChecksum computes a SHA256 checksum of the given data.
func ComputeChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// escapeSQLString escapes single quotes in a string for SQL.
func escapeSQLString(s string) string {
	result := ""
	for _, c := range s {
		if c == '\'' {
			result += "''"
		} else {
			result += string(c)
		}
	}
	return result
}
