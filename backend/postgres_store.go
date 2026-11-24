package main

import (
	"database/sql"
	"errors"

	_ "github.com/jackc/pgx/v5/stdlib" // registra el driver "pgx"
)

// PostgresStore guarda las tareas en PostgreSQL.
type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	// Crea la tabla si todavía no existe.
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
			id    SERIAL PRIMARY KEY,
			title TEXT    NOT NULL,
			done  BOOLEAN NOT NULL DEFAULT FALSE
		)`)
	if err != nil {
		return nil, err
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) List() ([]Task, error) {
	rows, err := s.db.Query(`SELECT id, title, done FROM tasks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []Task{} // lista vacía => JSON [] en lugar de null
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Done); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (s *PostgresStore) Create(title string) (Task, error) {
	t := Task{Title: title}
	err := s.db.QueryRow(
		`INSERT INTO tasks (title) VALUES ($1) RETURNING id, done`, title,
	).Scan(&t.ID, &t.Done)
	return t, err
}

func (s *PostgresStore) Toggle(id int) (Task, error) {
	var t Task
	err := s.db.QueryRow(
		`UPDATE tasks SET done = NOT done WHERE id = $1 RETURNING id, title, done`, id,
	).Scan(&t.ID, &t.Title, &t.Done)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	return t, err
}

func (s *PostgresStore) Delete(id int) error {
	res, err := s.db.Exec(`DELETE FROM tasks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
