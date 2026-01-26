package dbmigrate

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DirectDBExecutor executes migrations directly against PostgreSQL.
// This is used for the standalone CLI, not the WASM plugin.
type DirectDBExecutor struct {
	pool *pgxpool.Pool
	ctx  context.Context
}

// NewDirectDBExecutor creates a new DirectDBExecutor.
func NewDirectDBExecutor(ctx context.Context, pool *pgxpool.Pool) *DirectDBExecutor {
	return &DirectDBExecutor{
		pool: pool,
		ctx:  ctx,
	}
}

// EnsureTrackingTable creates the migration tracking table if it doesn't exist.
func (e *DirectDBExecutor) EnsureTrackingTable() error {
	_, err := e.pool.Exec(e.ctx, `
		CREATE TABLE IF NOT EXISTS kalo_migrations (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create tracking table: %w", err)
	}
	return nil
}

// GetApplied returns all applied migrations from the database.
func (e *DirectDBExecutor) GetApplied() ([]AppliedMigration, error) {
	rows, err := e.pool.Query(e.ctx, `
		SELECT name, checksum, EXTRACT(EPOCH FROM applied_at)::bigint as applied_at
		FROM kalo_migrations
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	var migrations []AppliedMigration
	for rows.Next() {
		var m AppliedMigration
		if err := rows.Scan(&m.Name, &m.Checksum, &m.AppliedAt); err != nil {
			return nil, fmt.Errorf("failed to scan migration row: %w", err)
		}
		migrations = append(migrations, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating migrations: %w", err)
	}

	return migrations, nil
}

// Apply executes a migration against the database.
func (e *DirectDBExecutor) Apply(name string, checksum string, sql []byte) error {
	// Begin transaction
	tx, err := e.pool.Begin(e.ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(e.ctx)

	// Execute the migration SQL
	_, err = tx.Exec(e.ctx, string(sql))
	if err != nil {
		return fmt.Errorf("migration SQL failed: %w", err)
	}

	// Record the migration
	_, err = tx.Exec(e.ctx, `
		INSERT INTO kalo_migrations (name, checksum, applied_at)
		VALUES ($1, $2, $3)
	`, name, checksum, time.Now())
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(e.ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

