package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrUsernameTaken  = errors.New("username taken")
	ErrQueueDuplicate = errors.New("already in queue")
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
}

type Title struct {
	ID             int64
	Source         string
	ExternalID     int
	Name           string
	Year           int
	EpisodeCount   int
	AverageMinutes int
	Poster         string
}

type QueueRow struct {
	ID       int64
	Position int
	Title    Title
}

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	dsn := (&url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)",
	}).String()
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	db := &DB{sql: sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate db: %w", err)
	}
	return db, nil
}

func (db *DB) Close() error { return db.sql.Close() }

func (db *DB) migrate() error {
	_, err := db.sql.Exec(`
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
    token TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS titles (
    id INTEGER PRIMARY KEY,
    external_id INTEGER NOT NULL,
    source TEXT NOT NULL DEFAULT 'tvmaze',
    name TEXT NOT NULL,
    year INTEGER NOT NULL,
    episode_count INTEGER NOT NULL,
    average_minutes INTEGER NOT NULL,
    poster TEXT NOT NULL DEFAULT '',
    fetched_at TEXT NOT NULL,
    UNIQUE(source, external_id)
);
CREATE TABLE IF NOT EXISTS queue_items (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    title_id INTEGER NOT NULL REFERENCES titles(id),
    position INTEGER NOT NULL,
    UNIQUE(user_id, title_id)
);`)
	if err != nil {
		return err
	}
	_, err = db.sql.Exec(`ALTER TABLE titles ADD COLUMN poster TEXT NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return err
	}
	return db.migrateTitleSource()
}

func (db *DB) migrateTitleSource() error {
	rows, err := db.sql.Query(`PRAGMA table_info(titles)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	hasSource := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == "source" {
			hasSource = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasSource {
		return nil
	}
	if _, err := db.sql.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer db.sql.Exec(`PRAGMA foreign_keys = ON`)
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
CREATE TABLE titles_new (
    id INTEGER PRIMARY KEY,
    external_id INTEGER NOT NULL,
    source TEXT NOT NULL DEFAULT 'tvmaze',
    name TEXT NOT NULL,
    year INTEGER NOT NULL,
    episode_count INTEGER NOT NULL,
    average_minutes INTEGER NOT NULL,
    poster TEXT NOT NULL DEFAULT '',
    fetched_at TEXT NOT NULL,
    UNIQUE(source, external_id)
);
INSERT INTO titles_new (id, external_id, source, name, year, episode_count, average_minutes, poster, fetched_at)
SELECT id, external_id, 'tvmaze', name, year, episode_count, average_minutes, poster, fetched_at FROM titles;
DROP TABLE titles;
ALTER TABLE titles_new RENAME TO titles;`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) CreateUser(ctx context.Context, username, passwordHash string) (User, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := db.sql.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username, passwordHash, now)
	if err != nil {
		if isUnique(err) {
			return User{}, ErrUsernameTaken
		}
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return User{ID: id, Username: username, PasswordHash: passwordHash}, nil
}

func (db *DB) UserByUsername(ctx context.Context, username string) (User, error) {
	return db.oneUser(ctx, `SELECT id, username, password_hash FROM users WHERE username = ?`, username)
}

func (db *DB) CreateSession(ctx context.Context, userID int64, token string, expires time.Time) error {
	_, err := db.sql.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`,
		token, userID, expires.UTC().Format(time.RFC3339Nano))
	return err
}

func (db *DB) DeleteSession(ctx context.Context, token string) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func (db *DB) UserByToken(ctx context.Context, token string, now time.Time) (User, error) {
	return db.oneUser(ctx, `
SELECT u.id, u.username, u.password_hash
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token = ? AND s.expires_at > ?`, token, now.UTC().Format(time.RFC3339Nano))
}

func (db *DB) oneUser(ctx context.Context, query string, args ...any) (User, error) {
	var user User
	err := db.sql.QueryRowContext(ctx, query, args...).Scan(&user.ID, &user.Username, &user.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (db *DB) UpsertTitle(ctx context.Context, title Title) (Title, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if title.Source == "" {
		title.Source = "tvmaze"
	}
	err := db.sql.QueryRowContext(ctx, `
INSERT INTO titles (external_id, source, name, year, episode_count, average_minutes, poster, fetched_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(source, external_id) DO UPDATE SET
    name = excluded.name,
    year = excluded.year,
    episode_count = excluded.episode_count,
    average_minutes = excluded.average_minutes,
    poster = excluded.poster,
    fetched_at = excluded.fetched_at
RETURNING id, external_id, name, year, episode_count, average_minutes, poster`,
		title.ExternalID, title.Source, title.Name, title.Year, title.EpisodeCount, title.AverageMinutes, title.Poster, now,
	).Scan(&title.ID, &title.ExternalID, &title.Name, &title.Year, &title.EpisodeCount, &title.AverageMinutes, &title.Poster)
	return title, err
}

func (db *DB) TitleByID(ctx context.Context, id int64) (Title, error) {
	var title Title
	err := db.sql.QueryRowContext(ctx, `
SELECT id, external_id, name, year, episode_count, average_minutes, poster
FROM titles WHERE id = ?`, id).Scan(
		&title.ID, &title.ExternalID, &title.Name, &title.Year, &title.EpisodeCount, &title.AverageMinutes, &title.Poster)
	if errors.Is(err, sql.ErrNoRows) {
		return Title{}, ErrNotFound
	}
	return title, err
}

func (db *DB) AddQueueItem(ctx context.Context, userID, titleID int64) (QueueRow, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return QueueRow{}, err
	}
	defer tx.Rollback()

	if err := tx.QueryRowContext(ctx, `SELECT id FROM titles WHERE id = ?`, titleID).Scan(new(int64)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return QueueRow{}, ErrNotFound
		}
		return QueueRow{}, err
	}
	var position int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) + 1 FROM queue_items WHERE user_id = ?`, userID).Scan(&position); err != nil {
		return QueueRow{}, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO queue_items (user_id, title_id, position) VALUES (?, ?, ?)`, userID, titleID, position)
	if err != nil {
		if isUnique(err) {
			return QueueRow{}, ErrQueueDuplicate
		}
		return QueueRow{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return QueueRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return QueueRow{}, err
	}
	return db.queueRow(ctx, userID, id)
}

func (db *DB) ListQueue(ctx context.Context, userID int64) ([]QueueRow, error) {
	rows, err := db.sql.QueryContext(ctx, `
SELECT q.id, q.position, t.id, t.external_id, t.name, t.year, t.episode_count, t.average_minutes, t.poster
FROM queue_items q
JOIN titles t ON t.id = q.title_id
WHERE q.user_id = ?
ORDER BY q.position, q.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []QueueRow
	for rows.Next() {
		var row QueueRow
		if err := rows.Scan(&row.ID, &row.Position, &row.Title.ID, &row.Title.ExternalID, &row.Title.Name, &row.Title.Year, &row.Title.EpisodeCount, &row.Title.AverageMinutes, &row.Title.Poster); err != nil {
			return nil, err
		}
		list = append(list, row)
	}
	if list == nil {
		list = []QueueRow{}
	}
	return list, rows.Err()
}

func (db *DB) MoveQueueItem(ctx context.Context, userID, itemID int64, delta int) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var position int
	err = tx.QueryRowContext(ctx, `SELECT position FROM queue_items WHERE id = ? AND user_id = ?`, itemID, userID).Scan(&position)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	neighborPos := position + delta
	if neighborPos < 1 {
		return nil
	}
	var neighborID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM queue_items WHERE user_id = ? AND position = ?`, userID, neighborPos).Scan(&neighborID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_items SET position = -1 WHERE id = ?`, itemID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_items SET position = ? WHERE id = ?`, position, neighborID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_items SET position = ? WHERE id = ?`, neighborPos, itemID); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) DeleteQueueItem(ctx context.Context, userID, itemID int64) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var position int
	err = tx.QueryRowContext(ctx, `SELECT position FROM queue_items WHERE id = ? AND user_id = ?`, itemID, userID).Scan(&position)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM queue_items WHERE id = ?`, itemID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_items SET position = position - 1 WHERE user_id = ? AND position > ?`, userID, position); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) queueRow(ctx context.Context, userID, itemID int64) (QueueRow, error) {
	var row QueueRow
	err := db.sql.QueryRowContext(ctx, `
SELECT q.id, q.position, t.id, t.external_id, t.name, t.year, t.episode_count, t.average_minutes, t.poster
FROM queue_items q
JOIN titles t ON t.id = q.title_id
WHERE q.user_id = ? AND q.id = ?`, userID, itemID).Scan(
		&row.ID, &row.Position, &row.Title.ID, &row.Title.ExternalID, &row.Title.Name, &row.Title.Year, &row.Title.EpisodeCount, &row.Title.AverageMinutes, &row.Title.Poster)
	if errors.Is(err, sql.ErrNoRows) {
		return QueueRow{}, ErrNotFound
	}
	return row, err
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
