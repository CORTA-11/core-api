package tenancy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	tenantmigrations "github.com/CORTA-11/core-api/db/migrations/tenant"
)

// Migration is one immutable tenant schema transition and its source checksum.
type Migration struct {
	Version  int64
	Name     string
	SQL      string
	Checksum string
}

// MigrationSet is the ordered desired tenant schema state embedded in a binary.
// Checksum identifies the complete ordered set, not only the latest migration.
type MigrationSet struct {
	Migrations []Migration
	Version    int64
	Checksum   string
}

// EmbeddedMigrations loads the tenant up migrations, requires contiguous
// versions starting at one, and computes per-migration and aggregate checksums.
func EmbeddedMigrations() (MigrationSet, error) {
	entries, err := fs.ReadDir(tenantmigrations.Files, ".")
	if err != nil {
		return MigrationSet{}, fmt.Errorf("read embedded tenant migrations: %w", err)
	}
	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			return MigrationSet{}, fmt.Errorf("invalid tenant migration name %q", entry.Name())
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil || version <= 0 {
			return MigrationSet{}, fmt.Errorf("invalid tenant migration version in %q", entry.Name())
		}
		body, err := tenantmigrations.Files.ReadFile(entry.Name())
		if err != nil {
			return MigrationSet{}, fmt.Errorf("read tenant migration %q: %w", entry.Name(), err)
		}
		sum := sha256.Sum256(body)
		migrations = append(migrations, Migration{
			Version: version, Name: entry.Name(), SQL: string(body), Checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	if len(migrations) == 0 {
		return MigrationSet{}, fmt.Errorf("tenant migration source is empty")
	}
	aggregate := sha256.New()
	for i, migration := range migrations {
		want := int64(i + 1)
		if migration.Version != want {
			return MigrationSet{}, fmt.Errorf("tenant migration versions must be contiguous: expected %d, found %d", want, migration.Version)
		}
		_, _ = fmt.Fprintf(aggregate, "%d:%s\n", migration.Version, migration.Checksum)
	}
	return MigrationSet{
		Migrations: migrations,
		Version:    migrations[len(migrations)-1].Version,
		Checksum:   hex.EncodeToString(aggregate.Sum(nil)),
	}, nil
}

// CanonicalSchema derives the only permitted schema name for a server-owned
// organization UUID. Callers must validate or load the UUID before using it.
func CanonicalSchema(publicID string) string {
	return "org_" + strings.ReplaceAll(strings.ToLower(publicID), "-", "")
}
