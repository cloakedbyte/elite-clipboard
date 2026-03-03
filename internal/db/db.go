package db

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Item struct {
	ID            int64  `json:"id"`
	Content       string `json:"content"`
	MimeType      string `json:"mime_type"`
	Category      string `json:"category"`
	SourceApp     string `json:"source_app"`
	WorkspaceID   int    `json:"workspace_id"`
	Timestamp     int64  `json:"timestamp"`
	CharCount     int    `json:"character_count"`
	ByteSize      int    `json:"byte_size"`
	Tags          string `json:"tags"`
	SelectionType string `json:"selection_type"`
	Pinned        int    `json:"pinned"`
	Sensitive     int    `json:"sensitive"`
	Deleted       int    `json:"deleted"`
}

type DB struct {
	conn *sql.DB
	mu   sync.Mutex
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	d := &DB{conn: conn}
	return d, d.init()
}

func (d *DB) init() error {
	_, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS items (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			content        TEXT    NOT NULL,
			mime_type      TEXT    DEFAULT 'text/plain',
			category       TEXT    DEFAULT 'text',
			source_app     TEXT    DEFAULT '',
			workspace_id   INTEGER DEFAULT 0,
			timestamp      INTEGER NOT NULL,
			character_count INTEGER DEFAULT 0,
			byte_size      INTEGER DEFAULT 0,
			tags           TEXT    DEFAULT '',
			selection_type TEXT    DEFAULT 'clipboard',
			pinned         INTEGER DEFAULT 0,
			sensitive      INTEGER DEFAULT 0,
			deleted        INTEGER DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_ts ON items(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_ws ON items(workspace_id);
	`)
	return err
}

func (d *DB) Insert(item *Item) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	item.Timestamp = time.Now().UnixMilli()
	item.CharCount = len([]rune(item.Content))
	item.ByteSize = len(item.Content)

	res, err := d.conn.Exec(`
		INSERT INTO items
			(content, mime_type, category, source_app, workspace_id,
			 timestamp, character_count, byte_size, tags, selection_type, sensitive)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		item.Content, item.MimeType, item.Category, item.SourceApp,
		item.WorkspaceID, item.Timestamp, item.CharCount, item.ByteSize,
		item.Tags, item.SelectionType, item.Sensitive,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) Exists(content string) bool {
	var count int
	d.conn.QueryRow(
		`SELECT COUNT(*) FROM items WHERE content=? AND deleted=0`, content,
	).Scan(&count)
	return count > 0
}

func (d *DB) Search(query string, workspaceID *int, limit int, pinnedOnly bool) ([]Item, error) {
	q := `SELECT id,content,mime_type,category,source_app,workspace_id,
		         timestamp,character_count,byte_size,tags,selection_type,pinned,sensitive,deleted
		  FROM items WHERE deleted=0`
	args := []any{}

	if query != "" {
		q += ` AND content LIKE ?`
		args = append(args, "%"+query+"%")
	}
	if workspaceID != nil {
		q += ` AND workspace_id=?`
		args = append(args, *workspaceID)
	}
	if pinnedOnly {
		q += ` AND pinned=1`
	}
	q += ` ORDER BY pinned DESC, timestamp DESC LIMIT ?`
	args = append(args, limit)

	rows, err := d.conn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var i Item
		if err := rows.Scan(&i.ID, &i.Content, &i.MimeType, &i.Category,
			&i.SourceApp, &i.WorkspaceID, &i.Timestamp, &i.CharCount,
			&i.ByteSize, &i.Tags, &i.SelectionType, &i.Pinned,
			&i.Sensitive, &i.Deleted); err != nil {
			continue
		}
		items = append(items, i)
	}
	return items, nil
}

func (d *DB) Pin(id int64, pinned bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	v := 0
	if pinned {
		v = 1
	}
	_, err := d.conn.Exec(`UPDATE items SET pinned=? WHERE id=?`, v, id)
	return err
}

func (d *DB) Delete(id int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.conn.Exec(`UPDATE items SET deleted=1 WHERE id=?`, id)
	return err
}

func (d *DB) Clear(workspaceID *int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if workspaceID != nil {
		_, err := d.conn.Exec(
			`UPDATE items SET deleted=1 WHERE workspace_id=? AND pinned=0`, *workspaceID)
		return err
	}
	_, err := d.conn.Exec(`UPDATE items SET deleted=1 WHERE pinned=0`)
	return err
}

func (d *DB) EnforceLimits(maxItems int, workspaceLimits map[string]int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.conn.Exec(`
		UPDATE items SET deleted=1 WHERE id IN (
			SELECT id FROM items WHERE deleted=0 AND pinned=0
			ORDER BY timestamp DESC LIMIT -1 OFFSET ?)`, maxItems)
	for wsID, limit := range workspaceLimits {
		d.conn.Exec(`
			UPDATE items SET deleted=1 WHERE id IN (
				SELECT id FROM items WHERE deleted=0 AND pinned=0
				AND workspace_id=?
				ORDER BY timestamp DESC LIMIT -1 OFFSET ?)`, wsID, limit)
	}
}

func (d *DB) Close() error {
	return d.conn.Close()
}
