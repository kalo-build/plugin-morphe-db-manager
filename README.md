# Morphe Database Manager Plugin

A Kalo plugin for applying SQL migrations to PostgreSQL databases. This plugin reads migration files from a filesystem store and applies them to a database store, tracking which migrations have been applied.

## Features

- **Migration Tracking**: Tracks applied migrations in a `kalo_migrations` table
- **Checksum Validation**: Detects if applied migrations have been modified
- **Dry Run Mode**: Preview what migrations would be applied
- **Dual Use**: Works as both a Kalo WASM plugin and a standalone CLI

## Usage

### As a Kalo Plugin

Configure in your `kalo.yaml`:

```yaml
stores:
  KA_MIGRATIONS:
    format: "KA:PSQL:MIGRATION1"
    type: "localFileSystem"
    options:
      path: "./migrations"

  DB_MAIN:
    format: "KA:PSQL:LIVE"
    type: "cloudSqlDatabase"
    options:
      provider: "gcp"
      connection: "$DATABASE_URL"

plugins:
  "@kalo-build/plugin-morphe-db-manager":
    version: "v1.0.0"
    input:
      format: "KA:PSQL:MIGRATION1"
      store: "KA_MIGRATIONS"
    output:
      format: "KA:PSQL:LIVE"
      store: "DB_MAIN"

pipelines:
  migrate:
    stages:
      - name: "apply"
        steps:
          - "plugin: @kalo-build/plugin-morphe-db-manager"
```

Then run:

```bash
kalo run migrate
```

### As a Standalone CLI

```bash
# Using environment variable
export DATABASE_URL="postgres://user:pass@localhost/mydb"
./dbmanager -m ./migrations

# Using --dsn flag
./dbmanager --dsn "postgres://user:pass@localhost/mydb" -m ./migrations

# Dry run
./dbmanager --dry-run

# Show status
./dbmanager --status

# Verbose output
./dbmanager -v
```

## Building

### WASM Plugin

```bash
./scripts/build.sh
# Output: dist/plugin-morphe-db-manager.wasm
```

### Standalone CLI

```bash
go build -o dist/dbmanager ./cmd/dbmanager
```

### Both

```bash
./scripts/build.sh
# Outputs:
#   dist/plugin-morphe-db-manager.wasm
#   dist/dbmanager
```

## Migration File Format

Migrations are `.sql` files in the migrations directory. They are applied in lexicographic order by filename.

Recommended naming convention:
```
001_initial_schema.sql
002_add_users_table.sql
003_add_email_column.sql
```

## Migration Tracking

The plugin creates a `kalo_migrations` table to track applied migrations:

```sql
CREATE TABLE kalo_migrations (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    checksum TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Configuration Options

When used as a Kalo plugin, you can configure behavior via the `config` section:

```yaml
config:
  "@kalo-build/plugin-morphe-db-manager":
    dryRun: false
    verbose: true
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                 plugin-morphe-db-manager                │
├─────────────────────────────────────────────────────────┤
│  cmd/plugin/       WASM plugin entry point              │
│  cmd/dbmanager/    Standalone CLI                       │
│  pkg/dbmigrate/    Core migration library               │
│    ├── migrate.go       Core Migrate() function         │
│    ├── types.go         Interfaces and types            │
│    ├── source_sdk.go    SDK-based file source           │
│    ├── source_direct.go Direct filesystem source        │
│    ├── executor_sdk.go  SDK-based DB executor           │
│    └── executor_direct.go Direct pgx executor           │
└─────────────────────────────────────────────────────────┘
```

## License

MIT License

