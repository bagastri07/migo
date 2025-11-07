# 🧱 Go Database Migrator

A lightweight, file-based **database migration tool** written in Go — built for **CLI and GitHub Actions** use.  
Each `.sql` migration file contains both **UP** and **DOWN** sections in a single file, and the migrator ensures **checksum validation** for integrity.

---

## ✨ Features

- 🧩 **Single-file migrations** (`-- up` / `-- down` in the same `.sql`)
- 🔒 **Checksum validation** — prevents running modified old migrations
- 🕓 **Migration history tracking** (`version`, `name`, `checksum`, `applied_at`)
- ⚙️ **CLI commands**: `create`, `up`, `up-to`, `down`, `info`
- 🧰 **Ready for GitHub Actions** or local development
- 🐘 **PostgreSQL supported** (extendable for other drivers)

---

## 📦 Project Structure

```
.
├── migrations/
│   ├── 000001_create_users_table.sql
│   └── 000002_add_index_to_users.sql
├── main.go
├── go.mod
├── go.sum
├── docker-compose.yml
└── README.md
```

---

## 🚀 Getting Started

### 1️⃣ Run PostgreSQL with Docker

```bash
docker compose up -d
```

This will start PostgreSQL on port `5432`.

---

### 2️⃣ Set Your Environment Variable

```bash
export DATABASE_URL=postgres://postgres:postgres@localhost:5432/migrator_db?sslmode=disable
```

> 💡 You can also use the `--dsn` flag instead of setting an environment variable.

---

### 3️⃣ Create a New Migration

```bash
go run main.go create add_users_table
```

This generates a file like:

```
migrations/000001_add_users_table.sql
```

**Template:**
```sql
-- up
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

-- down
DROP TABLE users;
```

---

### 4️⃣ Apply Migrations

#### Apply all pending migrations
```bash
go run main.go up
```

#### Apply up to a specific version
```bash
go run main.go up-to 000002
```

#### Rollback last migration
```bash
go run main.go down
```

---

### 5️⃣ View Migration Info

```bash
go run main.go info
```

**Example Output:**
```
Version     Name                      Valid            Applied A
000001      create_users_table        true             2025-11-08 00:32:11
000002      add_index_to_users        true             2025-11-08 00:35:04
```

---

## 🔐 Checksum Validation

Before any migration is applied, the tool will:

1. Compute the SHA256 checksum of all existing migration files.
2. Compare them against stored checksums in the database.
3. Abort if any previously applied migration file has been changed.

✅ **Ensures migration immutability** — your database schema history is always safe.

---

## 🧠 Database Schema

The migrator automatically creates a table named `migration_history`:

| Column       | Type      | Description                     |
|---------------|-----------|---------------------------------|
| `version`     | BIGINT    | Sequential migration version    |
| `name`        | TEXT      | Migration name                  |
| `checksum`    | TEXT      | SHA256 hash of migration file   |
| `applied_at`  | TIMESTAMP | Time when migration was applied |

---

## 🧪 GitHub Actions Integration

You can run migrations automatically in CI/CD by adding this step to your workflow:

```yaml
- name: Run Database Migrations
  run: go run main.go up
  env:
    DATABASE_URL: ${{ secrets.DATABASE_URL }}
```

---

## 🧰 Commands Summary

| Command | Description |
|----------|-------------|
| `create <name>` | Create new migration file |
| `up` | Apply all pending migrations |
| `up-to <version>` | Apply migrations up to specific version |
| `down` | Rollback the last migration |
| `info` | Show migration state and checksum validation |

---

## 🧑‍💻 License

MIT License © 2025 — Built with ❤️ in Go
