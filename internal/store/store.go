package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ysqss/shortlink/internal/model"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA synchronous=NORMAL;
		PRAGMA busy_timeout=5000;

		CREATE TABLE IF NOT EXISTS links (
			id          INTEGER PRIMARY KEY,
			slug        TEXT NOT NULL UNIQUE,
			long_url    TEXT NOT NULL,
			password    TEXT DEFAULT '',
			expires_at  TEXT,
			is_deleted  INTEGER DEFAULT 0,
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_links_slug ON links(slug) WHERE is_deleted = 0;
		CREATE INDEX IF NOT EXISTS idx_links_created ON links(created_at DESC);

		CREATE TABLE IF NOT EXISTS clicks (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			slug     TEXT NOT NULL,
			time     TEXT NOT NULL DEFAULT (datetime('now')),
			referer  TEXT DEFAULT '',
			ua       TEXT DEFAULT '',
			ip_hash  TEXT DEFAULT ''
		);

		CREATE INDEX IF NOT EXISTS idx_clicks_slug ON clicks(slug, time DESC);
		CREATE INDEX IF NOT EXISTS idx_clicks_time ON clicks(time);

		CREATE TABLE IF NOT EXISTS click_aggr (
			slug  TEXT NOT NULL,
			date  TEXT NOT NULL,
			total INTEGER DEFAULT 0,
			PRIMARY KEY (slug, date)
		);
	`)
	return err
}

func (s *Store) InsertLink(link *model.ShortLink) error {
	var expiresAt any
	if link.ExpiresAt != nil {
		expiresAt = link.ExpiresAt.Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`INSERT INTO links (id, slug, long_url, password, expires_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		link.ID, link.Slug, link.LongURL, link.Password,
		expiresAt, link.CreatedAt.Format(time.RFC3339), link.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

func (s *Store) GetLinkBySlug(slug string) (*model.ShortLink, error) {
	row := s.db.QueryRow(
		`SELECT id, slug, long_url, password, expires_at, is_deleted, created_at, updated_at
		 FROM links WHERE slug = ? AND is_deleted = 0`, slug,
	)
	return s.scanLink(row)
}

func (s *Store) GetLinkByID(id int64) (*model.ShortLink, error) {
	row := s.db.QueryRow(
		`SELECT id, slug, long_url, password, expires_at, is_deleted, created_at, updated_at
		 FROM links WHERE id = ?`, id,
	)
	return s.scanLink(row)
}

func (s *Store) DeleteLink(slug string) error {
	_, err := s.db.Exec(
		`UPDATE links SET is_deleted = 1, updated_at = ? WHERE slug = ?`,
		time.Now().Format(time.RFC3339), slug,
	)
	return err
}

func (s *Store) ListLinks(page, pageSize int) ([]*model.ShortLink, int64, error) {
	var total int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM links WHERE is_deleted = 0`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	rows, err := s.db.Query(
		`SELECT id, slug, long_url, password, expires_at, is_deleted, created_at, updated_at
		 FROM links WHERE is_deleted = 0
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var links []*model.ShortLink
	for rows.Next() {
		link, err := s.scanLinkFromRows(rows)
		if err != nil {
			return nil, 0, err
		}
		links = append(links, link)
	}
	return links, total, rows.Err()
}

func (s *Store) SearchLinks(query string) ([]*model.ShortLink, error) {
	rows, err := s.db.Query(
		`SELECT id, slug, long_url, password, expires_at, is_deleted, created_at, updated_at
		 FROM links WHERE is_deleted = 0 AND (slug LIKE ? OR long_url LIKE ?)
		 ORDER BY created_at DESC LIMIT 50`,
		"%"+query+"%", "%"+query+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []*model.ShortLink
	for rows.Next() {
		link, err := s.scanLinkFromRows(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Store) UpdateLink(slug string, link *model.ShortLink) error {
	var expiresAt any
	if link.ExpiresAt != nil {
		expiresAt = link.ExpiresAt.Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`UPDATE links SET long_url = ?, password = ?, expires_at = ?, updated_at = ?
		 WHERE slug = ? AND is_deleted = 0`,
		link.LongURL, link.Password, expiresAt,
		time.Now().Format(time.RFC3339), slug,
	)
	return err
}

func (s *Store) InsertClick(slug, referer, ua, ipHash string) error {
	_, err := s.db.Exec(
		`INSERT INTO clicks (slug, referer, ua, ip_hash) VALUES (?, ?, ?, ?)`,
		slug, referer, ua, ipHash,
	)
	return err
}

func (s *Store) GetClickStats(slug string) (int64, error) {
	var total int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM clicks WHERE slug = ?`, slug).Scan(&total)
	return total, err
}

func (s *Store) GetDailyClickStats(slug string) (map[string]int64, error) {
	rows, err := s.db.Query(
		`SELECT date(time) as d, COUNT(*) FROM clicks WHERE slug = ?
		 GROUP BY d ORDER BY d DESC LIMIT 30`, slug,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var date string
		var count int64
		if err := rows.Scan(&date, &count); err != nil {
			return nil, err
		}
		result[date] = count
	}
	return result, rows.Err()
}

func (s *Store) SlugExists(slug string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM links WHERE slug = ?`, slug).Scan(&count)
	return count > 0, err
}

func (s *Store) GetTopReferers(slug string, limit int) ([]map[string]any, error) {
	rows, err := s.db.Query(
		`SELECT referer, COUNT(*) as cnt FROM clicks WHERE slug = ? AND referer != ''
		 GROUP BY referer ORDER BY cnt DESC LIMIT ?`, slug, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var referer string
		var count int64
		if err := rows.Scan(&referer, &count); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{"referer": referer, "count": count})
	}
	return results, rows.Err()
}

func (s *Store) GetTopUserAgents(slug string, limit int) ([]map[string]any, error) {
	rows, err := s.db.Query(
		`SELECT ua, COUNT(*) as cnt FROM clicks WHERE slug = ? AND ua != ''
		 GROUP BY ua ORDER BY cnt DESC LIMIT ?`, slug, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var ua string
		var count int64
		if err := rows.Scan(&ua, &count); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{"user_agent": ua, "count": count})
	}
	return results, rows.Err()
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) scanLink(row *sql.Row) (*model.ShortLink, error) {
	var link model.ShortLink
	var expiresAt sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&link.ID, &link.Slug, &link.LongURL, &link.Password,
		&expiresAt, &link.IsDeleted, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	link.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	link.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if expiresAt.Valid {
		t, err := time.Parse(time.RFC3339, expiresAt.String)
		if err == nil {
			link.ExpiresAt = &t
		}
	}
	return &link, nil
}

func (s *Store) scanLinkFromRows(rows *sql.Rows) (*model.ShortLink, error) {
	var link model.ShortLink
	var expiresAt sql.NullString
	var createdAt, updatedAt string
	err := rows.Scan(&link.ID, &link.Slug, &link.LongURL, &link.Password,
		&expiresAt, &link.IsDeleted, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	link.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	link.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if expiresAt.Valid {
		t, err := time.Parse(time.RFC3339, expiresAt.String)
		if err == nil {
			link.ExpiresAt = &t
		}
	}
	return &link, nil
}