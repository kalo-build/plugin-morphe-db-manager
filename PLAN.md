## High-Level Target

We want to end up with:

* A **single, reusable plugin**: `@kalo-build/plugin-morphe-db-manager`
* Backed by a **shared library** that:

  * Knows how to:

    * Read SQL migration files from a Kalo **store**
    * Track applied migrations in a **migration table**
    * Apply both **schema migrations** and **MorpheDiff-generated migrations**
  * Is used by:

    * This **Kalo plugin** (primary)
    * A thin **`dbmanager` CLI** wrapper (for standalone/manual use)

No more “wrap external executable.”
The executable becomes a **dumb front-end** to the same core.

---

## Phase 0 — Prerequisites in the Kalo CLI (Foundations)

Before touching `dbmanager`, make sure the CLI supports:

1. **Store types in `kalo.yaml`**:

   * `localFileSystem` for:

     * Morphe YAML
     * MorpheDiff YAML
     * PSQL migrations
   * `gitRepository` for:

     * Base Morphe models from git
   * `cloudSqlDatabase` for:

     * Target DB (GCP Cloud SQL for now)

2. **Basic `kalo run`**:

   * Can read `pipelines.<name>`
   * Can execute the full pipeline (ie. `kalo run morphe-migrate`)
   * Can also execute individual plugins directly (ie. `kalo run @kalo-build/plugin-morphediff-psql`)
   * Can resolve input/output stores for a plugin in a pipeline or standalone mode

> Claude: This phase is mostly wiring. No DB logic yet, but needed for the final UX.

---

## Phase 1 — Extract `dbmanager` Core into a Shared Library

**Goal:** Turn `dbmanager` into a very thin CLI that calls a shared library.

### 1.1 Create a core package

* In `kalo-plugin-registry` (or a shared repo), create something like:

```go
// pkg/dbmigrate/dbmigrate.go

type MigrationSource interface {
    ListMigrations(ctx context.Context) ([]MigrationFile, error)
    OpenMigration(ctx context.Context, f MigrationFile) (io.ReadCloser, error)
}

type MigrationFile struct {
    Name     string // e.g. "001_initial_schema.sql"
    Checksum string // content hash
}

type DBConfig struct {
    DSN string // or structured host/port/user/db
}

type Executor struct {
    DB DBConfig
}

func (e *Executor) ApplyMigrations(ctx context.Context, src MigrationSource) error {
    // 1. ensure migration tracking table
    // 2. list migrations from src
    // 3. for each:
    //    - check if already applied in tracking table
    //    - if not: apply + record in tracking table
}
```

* Implement **v0 behavior**:

  * Migration tracking table:

    ```sql
    CREATE TABLE IF NOT EXISTS kalo_migrations (
      id SERIAL PRIMARY KEY,
      name TEXT NOT NULL,
      checksum TEXT NOT NULL,
      applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
    );
    ```
  * Schema migrations only (whatever `dbmanager` currently runs).

### 1.2 Refactor the existing `dbmanager` CLI to use this library

* Replace ad-hoc code with:

```go
func main() {
    // parse flags: --dsn, --migrations-dir
    // create a filesystem-based MigrationSource
    // call dbmigrate.Executor.ApplyMigrations(...)
}
```

> Claude: The existing schema-migration behavior should be preserved, but implemented through `pkg/dbmigrate`.

---

## Phase 2 — Extend Core Library to Support MorpheDiff-Generated Migrations

**Goal:** Core library can handle **any set of ordered `.sql` files** — schema *and* diff-based.

### 2.1 Standardize migration file semantics

Decide & implement:

* Migration naming convention: e.g. `001_initial_schema.sql`, `002_add_email_to_user.sql`
* Ordering:

  * Sort lexicographically by filename
* Checksum:

  * Simple content hash (SHA256 of file content)

### 2.2 Make `dbmigrate.Executor` agnostic

The library should **not care** whether migrations come from:

* Initial schema plugin
* MorpheDiff plugin
* Manual SQL additions

It just:

1. Gets a list of `MigrationFile` from `MigrationSource`
2. Sorts them
3. Checks tracking table
4. Applies missing ones

> Claude: At this point, the library is “complete enough” to handle both current and future migrations.

---

## Phase 3 — Kalo Store Integration Model (Conceptual/Structural)

**Goal:** Define how the plugin will see DB + migrations as Kalo stores.

We won’t implement the plugin yet, just define the contract.

### 3.1 Define typical stores in `kalo.yaml`

Example:

```yaml
stores:
  KA_MIGRATIONS:
    format: "KA:PSQL:MIGRATION1"
    type: "localFileSystem"
    path: "./migrations"

  DB_MAIN:
    format: "KA:PSQL:LIVE"
    type: "cloudSqlDatabase"
    provider: "gcp"
    connection: "$DATABASE_URL"
```

### 3.2 Plugin contract (conceptual)

`@kalo-build/plugin-morphe-db-manager` will expect:

* Input migrations store: `KA_MIGRATIONS`
* Target DB store: `DB_MAIN`

The Kalo CLI will:

* Resolve:

  * Path of `KA_MIGRATIONS`
  * DSN/connection for `DB_MAIN`
* Provide those to the plugin using a simple config struct.

> Claude: Keep this in mind while you implement the plugin—don’t let it reach into env vars directly except as passed config.

---

## Phase 4 — Implement `@kalo-build/plugin-morphe-db-manager` Using the Library

**Goal:** Replace standalone `dbmanager` usage with a proper Kalo plugin, but reuse the library.

### 4.1 Plugin manifest (high level)

We want something like:

```yaml
plugins:
  "@kalo-build/plugin-morphe-db-manager":
    version: "v1.0.0"
    role: "deploy"
    input:
      format: "KA:PSQL:MIGRATION1"
      store: "KA_MIGRATIONS"
    target:
      store: "DB_MAIN"
```

(Exact schema may vary, but the idea is: one migrations store + one DB store.)

### 4.2 Plugin implementation

In Go, roughly:

```go
// plugin-morphe-db-manager/main.go

type Config struct {
    MigrationsDir string
    DSN           string
}

func Run(ctx context.Context, cfg Config) error {
    src := NewFilesystemSource(cfg.MigrationsDir)
    exec := dbmigrate.Executor{
        DB: dbmigrate.DBConfig{DSN: cfg.DSN},
    }
    return exec.ApplyMigrations(ctx, src)
}
```

Kalo CLI responsibilities:

* Resolve `KA_MIGRATIONS` to `MigrationsDir`.
* Resolve `DB_MAIN` to `DSN`.
* Construct `Config` and pass it to plugin inside the WASM host call.

> Claude: No spawning `dbmanager` here. We import the *library* that `dbmanager` also uses.

### 4.3 Keep `dbmanager` as a thin CLI wrapper

`dbmanager` now becomes:

```go
func main() {
    // parse flags
    cfg := Config{MigrationsDir: dir, DSN: dsn}
    if err := Run(context.Background(), cfg); err != nil {
        os.Exit(1)
    }
}
```

This ensures:

* **One core implementation** (library)
* Two frontends:

  * Kalo plugin
  * Standalone CLI

---

## Phase 5 — Wire the MorpheDiff → Migrations → DB Pipeline in Kalo

**Goal:** End-to-end flow from Morphe + git → MorpheDiff → SQL migrations → DB.

### 5.1 Stores in `kalo.yaml`

```yaml
stores:
  KA_MO_YAML:
    format: "KA:MO1:YAML1"
    type: "localFileSystem"
    path: "./morphe/registry"

  KA_GIT_MAIN:
    format: "KA:MO1:YAML1"
    type: "gitRepository"
    repoRoot: "."
    ref: "main"
    subPath: "morphe/registry"

  KA_MORPHE_DIFF:
    format: "KA:MD1:YAML1"
    type: "localFileSystem"
    path: "./morphe-diffs"

  KA_MIGRATIONS:
    format: "KA:PSQL:MIGRATION1"
    type: "localFileSystem"
    path: "./migrations"

  DB_MAIN:
    format: "KA:PSQL:LIVE"
    type: "cloudSqlDatabase"
    provider: "gcp"
    connection: "$DATABASE_URL"
```

### 5.2 Plugins expected

* `@kalo-build/plugin-morphe-git-morphediff`

  * base: `KA_GIT_MAIN`
  * head: `KA_MO_YAML`
  * output: `KA_MORPHE_DIFF`

* `@kalo-build/plugin-morphediff-psql`

  * input: `KA_MORPHE_DIFF`
  * output: `KA_MIGRATIONS`

* `@kalo-build/plugin-morphe-db-manager`

  * input: `KA_MIGRATIONS`
  * target: `DB_MAIN`

### 5.3 Pipeline definition

```yaml
pipelines:
  morphe-diff-and-migrate:
    stages:
      - name: "diff"
        steps:
          - "plugin: @kalo-build/plugin-morphe-git-morphediff"
      - name: "migrations"
        steps:
          - "plugin: @kalo-build/plugin-morphediff-psql"
      - name: "apply"
        steps:
          - "plugin: @kalo-build/plugin-morphe-db-manager"
```

Usage:

```bash
kalo run morphe-diff-and-migrate --base main
```

> Claude: The `--base` flag should be forwarded to the git-based diff plugin to override `ref`.

---

## Phase 6 — Optional Hardening & Future Work

Once the basic flow works:

1. **Better migration metadata**

   * Add columns: `batch_id`, `applied_by`, `execution_time`, etc.
2. **Rollback strategy (later)**

   * Track reversible migrations
   * Add a `db-rollback` plugin or flag
3. **Multi-DB support**

   * Extend `cloudSqlDatabase` to other providers, or new store types
4. **Test harness**

   * A test pipeline that:

     * Provisions a temp DB
     * Runs `morphe-diff-and-migrate`
     * Validates schema state

---

## TL;DR for Claude

You can treat this as the implementation checklist:

1. **Extract** existing `dbmanager` logic into `pkg/dbmigrate` with:

   * `Executor.ApplyMigrations(ctx, MigrationSource)`
   * Migration tracking table
2. **Refactor** `dbmanager` CLI to use `pkg/dbmigrate` exclusively.
3. **Implement** `@kalo-build/plugin-morphe-db-manager` as a Go/WASM plugin that:

   * Accepts `MigrationsDir` + `DSN` config
   * Uses `pkg/dbmigrate` under the hood
4. **Wire** Kalo CLI to:

   * Resolve `KA_MIGRATIONS` and `DB_MAIN` stores
   * Pass correct config to the plugin
5. **Define** the `morphe-diff-and-migrate` pipeline and test the full flow:

   * Morphe + git → MorpheDiff → PSQL migrations → DB apply
