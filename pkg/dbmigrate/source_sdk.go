package dbmigrate

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	kalo "github.com/kalo-build/kalo-sdk-go"
)

// SDKMigrationSource reads migrations from a Kalo FileStore.
type SDKMigrationSource struct {
	fs kalo.FileStore
}

// NewSDKMigrationSource creates a new SDKMigrationSource.
func NewSDKMigrationSource(fs kalo.FileStore) *SDKMigrationSource {
	return &SDKMigrationSource{fs: fs}
}

// ListMigrations returns all .sql files in the store, sorted by name.
func (s *SDKMigrationSource) ListMigrations() ([]MigrationFile, error) {
	entries, err := s.fs.ListDir(".")
	if err != nil {
		return nil, fmt.Errorf("failed to list migrations directory: %w", err)
	}

	var migrations []MigrationFile
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		if !strings.HasSuffix(entry.Name, ".sql") {
			continue
		}

		// Read file to compute checksum
		content, err := s.fs.ReadFile(entry.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration %s: %w", entry.Name, err)
		}

		migrations = append(migrations, MigrationFile{
			Name:     entry.Name,
			Checksum: computeChecksum(content),
		})
	}

	// Sort by name (lexicographic)
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Name < migrations[j].Name
	})

	return migrations, nil
}

// ReadMigration reads the SQL content of a migration file.
func (s *SDKMigrationSource) ReadMigration(name string) ([]byte, error) {
	// Ensure name doesn't escape the base directory
	clean := filepath.Clean(name)
	if strings.HasPrefix(clean, "..") {
		return nil, fmt.Errorf("invalid migration path: %s", name)
	}

	return s.fs.ReadFile(name)
}

// computeChecksum computes a SHA256 checksum of the given data.
func computeChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

