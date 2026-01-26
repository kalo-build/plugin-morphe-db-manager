package dbmigrate

import (
	"fmt"

	kalo "github.com/kalo-build/kalo-sdk-go"
)

// SDKMigrationExecutor executes migrations using a Kalo DBStore.
type SDKMigrationExecutor struct {
	db kalo.DBStore
}

// NewSDKMigrationExecutor creates a new SDKMigrationExecutor.
func NewSDKMigrationExecutor(db kalo.DBStore) *SDKMigrationExecutor {
	return &SDKMigrationExecutor{db: db}
}

// EnsureTrackingTable creates the migration tracking table if it doesn't exist.
func (e *SDKMigrationExecutor) EnsureTrackingTable() error {
	return e.db.EnsureTrackingTable()
}

// GetApplied returns all applied migrations from the database.
func (e *SDKMigrationExecutor) GetApplied() ([]AppliedMigration, error) {
	applied, err := e.db.GetAppliedMigrations()
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	result := make([]AppliedMigration, len(applied))
	for i, m := range applied {
		result[i] = AppliedMigration{
			Name:      m.Name,
			Checksum:  m.Checksum,
			AppliedAt: m.AppliedAt,
		}
	}

	return result, nil
}

// Apply executes a migration against the database.
func (e *SDKMigrationExecutor) Apply(name string, checksum string, sql []byte) error {
	return e.db.ApplyMigration(name, sql)
}

