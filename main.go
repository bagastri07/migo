package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

const migrationDir = "./migrations"

type Migration struct {
	Version  int64
	Name     string
	UpSQL    string
	DownSQL  string
	Checksum string
}

func main() {
	var dsn string
	flag.StringVar(&dsn, "dsn", os.Getenv("DATABASE_URL"), "PostgreSQL DSN (can use env DATABASE_URL)")
	flag.Parse()

	if len(flag.Args()) < 1 {
		log.Fatal("Usage: migrator [create|up|down|up-to|info]")
	}

	cmd := flag.Arg(0)

	// CREATE command doesn't require DB
	if cmd == "create" {
		if len(flag.Args()) < 2 {
			log.Fatal("Usage: migrator create <name>")
		}
		createMigrationFile(flag.Arg(1))
		return
	}

	if dsn == "" {
		log.Fatal("Missing DATABASE_URL or --dsn flag")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("DB connect error: %v", err)
	}
	defer db.Close()

	if err := ensureMigrationTable(db); err != nil {
		log.Fatalf("failed to ensure migration table: %v", err)
	}

	switch cmd {
	case "up":
		applyMigrations(db, false)
	case "up-to":
		if len(flag.Args()) < 2 {
			log.Fatal("Usage: migrator up-to <version>")
		}
		var version int64
		fmt.Sscanf(flag.Arg(1), "%d", &version)
		applyMigrations(db, true, version)
	case "down":
		rollbackLastMigration(db)
	case "info":
		showMigrationInfo(db)
	default:
		log.Fatalf("Unknown command: %s", cmd)
	}
}

func ensureMigrationTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMP NOT NULL
		);
	`)
	return err
}

func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func parseMigrationFile(path string) (*Migration, error) {
	content, err := readFile(path)
	if err != nil {
		return nil, err
	}

	filename := filepath.Base(path)
	re := regexp.MustCompile(`^(\d+)_([^.]+)\.sql$`)
	matches := re.FindStringSubmatch(filename)
	if len(matches) != 3 {
		return nil, fmt.Errorf("invalid filename: %s", filename)
	}

	version := parseInt64(matches[1])
	name := matches[2]

	split := strings.SplitN(string(content), "-- +down", 2)
	if len(split) != 2 {
		return nil, fmt.Errorf("missing '-- +down' section in %s", filename)
	}
	upPart := strings.ReplaceAll(split[0], "-- +up", "")
	downPart := split[1]

	hash := sha256.Sum256(content)

	return &Migration{
		Version:  version,
		Name:     name,
		UpSQL:    strings.TrimSpace(upPart),
		DownSQL:  strings.TrimSpace(downPart),
		Checksum: hex.EncodeToString(hash[:]),
	}, nil
}

func parseInt64(s string) int64 {
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}

func loadMigrations() ([]*Migration, error) {
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		return nil, err
	}

	var migrations []*Migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		path := filepath.Join(migrationDir, e.Name())
		m, err := parseMigrationFile(path)
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, m)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations, nil
}

func appliedMigrations(db *sql.DB) map[int64]string {
	rows, err := db.Query("SELECT version, checksum FROM schema_migrations")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	applied := make(map[int64]string)
	for rows.Next() {
		var version int64
		var checksum string
		rows.Scan(&version, &checksum)
		applied[version] = checksum
	}
	return applied
}

func applyMigrations(db *sql.DB, upTo bool, target ...int64) {
	migrations, err := loadMigrations()
	if err != nil {
		log.Fatalf("failed to load migrations: %v", err)
	}

	applied := appliedMigrations(db)

	// Validate checksums before applying anything
	for _, m := range migrations {
		if oldChecksum, ok := applied[m.Version]; ok {
			if oldChecksum != m.Checksum {
				log.Fatalf("Checksum mismatch detected for version %d_%s — migration file changed after apply", m.Version, m.Name)
			}
		}
	}

	for _, m := range migrations {
		if _, ok := applied[m.Version]; ok {
			continue // already applied
		}
		if upTo && len(target) > 0 && m.Version > target[0] {
			break
		}

		log.Printf("Applying migration %d_%s...", m.Version, m.Name)
		if _, err := db.Exec(m.UpSQL); err != nil {
			log.Fatalf("failed to apply migration %d: %v", m.Version, err)
		}

		_, err = db.Exec(`INSERT INTO schema_migrations (version, name, checksum, applied_at)
			VALUES ($1, $2, $3, $4)`,
			m.Version, m.Name, m.Checksum, time.Now())
		if err != nil {
			log.Fatalf("failed to record migration %d: %v", m.Version, err)
		}
	}

	log.Println("Migrations applied successfully")
}

func rollbackLastMigration(db *sql.DB) {
	row := db.QueryRow(`SELECT version, name FROM schema_migrations ORDER BY version DESC LIMIT 1`)
	var version int64
	var name string
	err := row.Scan(&version, &name)
	if err == sql.ErrNoRows {
		log.Println("No migrations to rollback")
		return
	} else if err != nil {
		log.Fatal(err)
	}

	path := filepath.Join(migrationDir, fmt.Sprintf("%d_%s.sql", version, name))
	m, err := parseMigrationFile(path)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Rolling back migration %d_%s...", version, name)
	if _, err := db.Exec(m.DownSQL); err != nil {
		log.Fatalf("failed to rollback migration %d: %v", m.Version, err)
	}

	_, err = db.Exec(`DELETE FROM schema_migrations WHERE version = $1`, version)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Rollback successful")
}

func createMigrationFile(name string) {
	ts := time.Now().Format("20060102150405")
	safeName := strings.ReplaceAll(name, " ", "_")
	filename := fmt.Sprintf("%s_%s.sql", ts, safeName)
	path := filepath.Join(migrationDir, filename)

	content := `-- +up
-- SQL statements for migration UP go here

-- +down
-- SQL statements for migration DOWN go here
`

	if err := os.MkdirAll(migrationDir, 0755); err != nil {
		log.Fatalf("failed to create migrations directory: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		log.Fatalf("failed to create migration file: %v", err)
	}

	log.Printf("Created migration file: %s", path)
}

func showMigrationInfo(db *sql.DB) {
	migrations, err := loadMigrations()
	if err != nil {
		log.Fatalf("failed to load migrations: %v", err)
	}

	rows, err := db.Query(`SELECT version, name, checksum, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	applied := make(map[int64]struct {
		Name      string
		Checksum  string
		AppliedAt time.Time
	})
	for rows.Next() {
		var version int64
		var name, checksum string
		var appliedAt time.Time
		rows.Scan(&version, &name, &checksum, &appliedAt)
		applied[version] = struct {
			Name      string
			Checksum  string
			AppliedAt time.Time
		}{name, checksum, appliedAt}
	}

	fmt.Println("Migration Info:")
	fmt.Println("---------------------------------------------------------------")
	fmt.Printf("%-16s %-25s %-8s %-20s\n", "Version", "Name", "Valid", "Applied At")
	fmt.Println("---------------------------------------------------------------")

	for _, m := range migrations {
		status := "NO"
		appliedAt := "-"
		if a, ok := applied[m.Version]; ok {
			if a.Checksum == m.Checksum {
				status = "YES"
			} else {
				status = "CHANGED"
			}
			appliedAt = a.AppliedAt.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("%-16d %-25s %-8s %-20s\n", m.Version, m.Name, status, appliedAt)
	}
	fmt.Println("---------------------------------------------------------------")
}
