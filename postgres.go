package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

type PostgresConfig struct{ DSN string }

type PostgresJobRepository struct{ db *sql.DB }

func NewPostgresJobRepository(config PostgresConfig) (*PostgresJobRepository, error) {
	db, err := sql.Open("postgres", config.DSN)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &PostgresJobRepository{db: db}, nil
}

func (r *PostgresJobRepository) Init() error {
	_, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS video_jobs (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		email TEXT NOT NULL DEFAULT '',
		object_key TEXT NOT NULL,
		status TEXT NOT NULL,
		error_message TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`ALTER TABLE video_jobs ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT ''`)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`ALTER TABLE video_jobs ADD COLUMN IF NOT EXISTS error_message TEXT NOT NULL DEFAULT ''`)
	return err
}

func (r *PostgresJobRepository) UpdateStatus(id, status, errorMessage string) error {
	_, err := r.db.Exec(`UPDATE video_jobs SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`, status, errorMessage, id)
	return err
}

func postgresConfig() PostgresConfig {
	return PostgresConfig{DSN: envOr("POSTGRES_DSN", fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		envOr("POSTGRES_HOST", "localhost"),
		envOr("POSTGRES_PORT", "5432"),
		envOr("POSTGRES_USER", "video_processor"),
		envOr("POSTGRES_PASSWORD", "video_processor"),
		envOr("POSTGRES_DB", "video_processor"),
	))}
}
