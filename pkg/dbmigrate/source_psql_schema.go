package dbmigrate

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	kalo "github.com/kalo-build/kalo-sdk-go"
)

// PSQLSchemaSubdirs defines the order in which subdirectories should be processed.
// This order ensures dependencies are created before dependents:
// 1. enums - lookup tables with no dependencies
// 2. models - main tables that may reference enums
// 3. structures - optional persistence tables
// 4. entities - views that reference models
var PSQLSchemaSubdirs = []string{"enums", "models", "structures", "entities"}

// PSQLSchemaMigrationSource reads migrations from a Kalo FileStore
// that follows the plugin-morphe-psql-types output structure.
// It processes subdirectories in the correct dependency order.
type PSQLSchemaMigrationSource struct {
	fs     kalo.FileStore
	prefix string // prefix for migration names (e.g., "schema:" or "diff:")
}

// NewPSQLSchemaMigrationSource creates a new PSQLSchemaMigrationSource.
func NewPSQLSchemaMigrationSource(fs kalo.FileStore, prefix string) *PSQLSchemaMigrationSource {
	return &PSQLSchemaMigrationSource{fs: fs, prefix: prefix}
}

// ListMigrations returns all .sql files in the correct dependency order.
// Files are grouped by subdirectory and sorted within each subdirectory.
func (s *PSQLSchemaMigrationSource) ListMigrations() ([]MigrationFile, error) {
	var allMigrations []MigrationFile

	// First, check if subdirectories exist
	hasSubdirs := false
	for _, subdir := range PSQLSchemaSubdirs {
		entries, err := s.fs.ListDir(subdir)
		if err == nil && len(entries) > 0 {
			hasSubdirs = true
			break
		}
	}

	if hasSubdirs {
		// Process each subdirectory in order
		for _, subdir := range PSQLSchemaSubdirs {
			migrations, err := s.listMigrationsInDir(subdir)
			if err != nil {
				// Directory might not exist, which is fine
				continue
			}
			allMigrations = append(allMigrations, migrations...)
		}
	} else {
		// Fallback: no subdirectories, just list root
		migrations, err := s.listMigrationsInDir(".")
		if err != nil {
			return nil, err
		}
		allMigrations = migrations
	}

	return allMigrations, nil
}

// listMigrationsInDir lists SQL files in a specific directory
func (s *PSQLSchemaMigrationSource) listMigrationsInDir(dir string) ([]MigrationFile, error) {
	entries, err := s.fs.ListDir(dir)
	if err != nil {
		return nil, err
	}

	var migrations []MigrationFile
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		if !strings.HasSuffix(entry.Name, ".sql") {
			continue
		}
		// Diff migrations use .up.sql / .down.sql pairs. Only include .up.sql when migrating up.
		if s.prefix == "diff:" && !strings.HasSuffix(entry.Name, ".up.sql") {
			continue
		}

		// Construct the full path
		var filePath string
		if dir == "." {
			filePath = entry.Name
		} else {
			filePath = filepath.Join(dir, entry.Name)
		}

		// Read file to compute checksum
		content, err := s.fs.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration %s: %w", filePath, err)
		}

		// Apply prefix to name for tracking
		name := filePath
		if s.prefix != "" {
			name = s.prefix + filePath
		}

		migrations = append(migrations, MigrationFile{
			Name:     name,
			Path:     filePath, // Store the actual path for reading
			Checksum: computeSchemaChecksum(content),
		})
	}

	// Sort by name within directory (lexicographic - works with numeric prefixes)
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Name < migrations[j].Name
	})

	return migrations, nil
}

// ReadMigration reads the SQL content of a migration file.
func (s *PSQLSchemaMigrationSource) ReadMigration(name string) ([]byte, error) {
	// Strip the prefix if present to get the actual path
	path := name
	if s.prefix != "" && strings.HasPrefix(name, s.prefix) {
		path = strings.TrimPrefix(name, s.prefix)
	}

	// Ensure path doesn't escape the base directory
	clean := filepath.Clean(path)
	if strings.HasPrefix(clean, "..") {
		return nil, fmt.Errorf("invalid migration path: %s", name)
	}

	return s.fs.ReadFile(path)
}

// computeSchemaChecksum computes a SHA256 checksum of the given data.
func computeSchemaChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}
