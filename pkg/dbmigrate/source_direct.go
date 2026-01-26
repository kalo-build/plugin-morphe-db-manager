package dbmigrate

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirectFileSource reads migrations directly from the filesystem.
// This is used for the standalone CLI, not the WASM plugin.
type DirectFileSource struct {
	basePath string
}

// NewDirectFileSource creates a new DirectFileSource.
func NewDirectFileSource(basePath string) *DirectFileSource {
	return &DirectFileSource{basePath: basePath}
}

// ListMigrations returns all .sql files in the directory, sorted by name.
func (s *DirectFileSource) ListMigrations() ([]MigrationFile, error) {
	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrations []MigrationFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Read file to compute checksum
		fullPath := filepath.Join(s.basePath, entry.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration %s: %w", entry.Name(), err)
		}

		hash := sha256.Sum256(content)
		migrations = append(migrations, MigrationFile{
			Name:     entry.Name(),
			Checksum: fmt.Sprintf("%x", hash),
		})
	}

	// Sort by name (lexicographic)
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Name < migrations[j].Name
	})

	return migrations, nil
}

// ReadMigration reads the SQL content of a migration file.
func (s *DirectFileSource) ReadMigration(name string) ([]byte, error) {
	// Ensure name doesn't escape the base directory
	clean := filepath.Clean(name)
	if strings.HasPrefix(clean, "..") {
		return nil, fmt.Errorf("invalid migration path: %s", name)
	}

	fullPath := filepath.Join(s.basePath, name)
	return os.ReadFile(fullPath)
}

