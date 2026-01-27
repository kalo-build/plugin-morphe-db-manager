# Morphe Database Manager Plugin

A Kalo plugin for managing PostgreSQL database schemas through SQL migrations. Supports multiple operation modes for development and production workflows.

## Features

- **Multi-Mode Operation**: up, down, refresh, seed, reset modes
- **Migration Tracking**: Tracks applied migrations in a `kalo_migrations` table
- **Checksum Validation**: Detects if applied migrations have been modified
- **Dual Input**: Reads base schema files and diff migrations separately
- **Dual Use**: Works as both a Kalo WASM plugin and a standalone CLI

## Operation Modes

| Mode | Alias | Description | Production Safe |
|------|-------|-------------|-----------------|
| `up` | `up` | Apply pending migrations | Yes |
| `down` | `down` | Drop all tables | **No** |
| `refresh` | `refresh` | Drop and recreate all tables | **No** |
| `seed` | `seed` | Insert initial data | Yes (if idempotent) |
| `reset` | `reset` | Reset to base schema only (dev-only) | **No** |

## Usage

### As a Kalo Plugin

Configure in your `kalo.yaml`:

```yaml
stores:
  KA_MO_PSQL:
    format: "KA:MO1:PSQL1"
    type: "localFileSystem"
    options:
      path: "./internal/database/schema"

  KA_MIGRATIONS:
    format: "KA:PSQL:MIGRATION1"
    type: "localFileSystem"
    options:
      path: "./migrations"

  KA_SEED:
    format: "KA:PSQL:SEED1"
    type: "localFileSystem"
    options:
      path: "./seed"

  DB_MAIN:
    format: "KA:PSQL:LIVE"
    type: "cloudSqlDatabase"
    options:
      connection: "$DATABASE_URL"

plugins:
  "@kalo-build/plugin-morphe-db-manager":
    version: "v1.0.0"
    inputs:
      schema:
        format: "KA:MO1:PSQL1"
        store: "KA_MO_PSQL"
      migrations:
        format: "KA:PSQL:MIGRATION1"
        store: "KA_MIGRATIONS"
      seed:
        format: "KA:PSQL:SEED1"
        store: "KA_SEED"
    output:
      format: "KA:PSQL:LIVE"
      store: "DB_MAIN"

pipelines:
  migrate-up:
    description: "Apply pending migrations"
    alias: "up"
    stages:
    - name: "up"
      steps:
        - "plugin: @kalo-build/plugin-morphe-db-manager"
      config:
        mode: "up"

  migrate-down:
    description: "Drop all tables (DESTRUCTIVE)"
    alias: "down"
    stages:
    - name: "down"
      steps:
        - "plugin: @kalo-build/plugin-morphe-db-manager"
      config:
        mode: "down"

  migrate-refresh:
    description: "Drop and recreate all tables (DESTRUCTIVE)"
    alias: "refresh"
    stages:
    - name: "refresh"
      steps:
        - "plugin: @kalo-build/plugin-morphe-db-manager"
      config:
        mode: "refresh"

  migrate-seed:
    description: "Insert seed data"
    alias: "seed"
    stages:
    - name: "seed"
      steps:
        - "plugin: @kalo-build/plugin-morphe-db-manager"
      config:
        mode: "seed"
        seedStore: "KA_SEED"
```

Then run:

```bash
kalo run up       # Apply pending migrations
kalo run down     # Drop all tables
kalo run refresh  # Drop and recreate tables
kalo run seed     # Insert seed data
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

