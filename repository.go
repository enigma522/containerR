package main

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("record not found")

const databaseFile = containerRoot + "/containerr.db"

//go:embed schema.sql
var schemaFS embed.FS

type Entity interface {
	GetID() string
}

type Repository[T Entity] interface {
	Save(T) error
	GetByID(string) (T, error)
	All() ([]T, error)
	DeleteByID(string) error
}

type Database interface {
	Open() (*sql.DB, error)
}

type SQLiteDatabase struct {
	fileName string
	once     sync.Once
	db       *sql.DB
	err      error
}

func NewSQLiteDatabase(fileName string) *SQLiteDatabase {
	return &SQLiteDatabase{fileName: fileName}
}

func (d *SQLiteDatabase) Open() (*sql.DB, error) {

	d.once.Do(func() {
		if err := os.MkdirAll(filepath.Dir(d.fileName), 0755); err != nil {
			d.err = err
			return
		}

		d.db, d.err = sql.Open("sqlite", d.fileName+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
		if d.err != nil {
			return
		}

		schema, err := schemaFS.ReadFile("schema.sql")
		if err != nil {
			_ = d.db.Close()
			d.db = nil
			d.err = err
			return
		}

		_, d.err = d.db.Exec(string(schema))
		if d.err == nil {
			d.err = ensureContainerSchema(d.db)
		}
		if d.err != nil {
			_ = d.db.Close()
			d.db = nil
		}

	})

	return d.db, d.err
}

var sharedDatabase = NewSQLiteDatabase(databaseFile)

func SharedDatabase() Database {
	return sharedDatabase
}

// -----------------Container------------------ //

type ContainerRecord struct {
	ID, Name, ImageRef, ImageDigest, Rootfs                  string
	Cmd, Entrypoint, Env, WorkingDir, Hostname, User, Status string
	PID                                                      int
}

func (r ContainerRecord) GetID() string { return r.ID }

type ContainerRepository interface {
	Repository[ContainerRecord]
}

type SQLiteContainerRepository struct{ database Database }

func NewContainerRepository() ContainerRepository {
	return NewSQLiteContainerRepository(SharedDatabase())
}

func NewSQLiteContainerRepository(database Database) ContainerRepository {
	return &SQLiteContainerRepository{database: database}
}

func (r *SQLiteContainerRepository) Save(record ContainerRecord) error {
	db, err := r.database.Open()
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO CONTAINERS (id, pid, name, image_ref, image_digest, rootfs, cmd, entrypoint, env, working_dir, hostname, user, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET pid=excluded.pid, name=excluded.name, image_ref=excluded.image_ref, image_digest=excluded.image_digest,
		rootfs=excluded.rootfs, cmd=excluded.cmd, entrypoint=excluded.entrypoint, env=excluded.env,
		working_dir=excluded.working_dir, hostname=excluded.hostname, user=excluded.user, status=excluded.status`,
		record.ID, record.PID, record.Name, record.ImageRef, record.ImageDigest, record.Rootfs, record.Cmd,
		record.Entrypoint, record.Env, record.WorkingDir, record.Hostname, record.User, record.Status)
	if err != nil {
		return fmt.Errorf("save container %q: %w", record.ID, err)
	}
	return nil
}

func (r *SQLiteContainerRepository) GetByID(id string) (ContainerRecord, error) {
	db, err := r.database.Open()
	if err != nil {
		return ContainerRecord{}, err
	}
	record, err := scanContainer(db.QueryRow(`SELECT id, pid, name, image_ref, image_digest, rootfs, cmd, entrypoint, env, working_dir, hostname, user, status
		FROM CONTAINERS WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ContainerRecord{}, ErrNotFound
	}
	if err != nil {
		return ContainerRecord{}, fmt.Errorf("get container %q: %w", id, err)
	}
	return record, nil
}

func (r *SQLiteContainerRepository) All() ([]ContainerRecord, error) {
	db, err := r.database.Open()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id, pid, name, image_ref, image_digest, rootfs, cmd, entrypoint, env, working_dir, hostname, user, status
		FROM CONTAINERS ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	defer rows.Close()

	records := make([]ContainerRecord, 0)
	for rows.Next() {
		record, err := scanContainer(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	return records, nil
}

func (r *SQLiteContainerRepository) DeleteByID(id string) error {
	db, err := r.database.Open()
	if err != nil {
		return err
	}
	result, err := db.Exec("DELETE FROM CONTAINERS WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete container %q: %w", id, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete container %q: %w", id, err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(...any) error
}

func scanContainer(s scanner) (ContainerRecord, error) {
	var record ContainerRecord
	err := s.Scan(&record.ID, &record.PID, &record.Name, &record.ImageRef, &record.ImageDigest,
		&record.Rootfs, &record.Cmd, &record.Entrypoint, &record.Env, &record.WorkingDir, &record.Hostname, &record.User, &record.Status)
	return record, err
}

// ensureContainerSchema applies additive migrations for databases created by
// earlier versions of containerR.
func ensureContainerSchema(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(CONTAINERS)")
	if err != nil {
		return fmt.Errorf("inspect containers schema: %w", err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("inspect containers schema: %w", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect containers schema: %w", err)
	}
	if !columns["hostname"] {
		if _, err := db.Exec("ALTER TABLE CONTAINERS ADD COLUMN hostname TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("add container hostname column: %w", err)
		}
	}
	if !columns["user"] {
		if _, err := db.Exec("ALTER TABLE CONTAINERS ADD COLUMN user TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("add container user column: %w", err)
		}
	}
	if !columns["pid"] {
		if _, err := db.Exec("ALTER TABLE CONTAINERS ADD COLUMN pid INTEGER NOT NULL DEFAULT 0"); err != nil {
			return fmt.Errorf("add container pid column: %w", err)
		}
	}
	return nil
}
