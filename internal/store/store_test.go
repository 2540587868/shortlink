package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ysqss/shortlink/internal/model"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestStoreInsertAndGet(t *testing.T) {
	db := newTestDB(t)
	s, err := New(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	link := &model.ShortLink{
		ID:        1,
		Slug:      "abc123",
		LongURL:   "https://example.com/long",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.InsertLink(link); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.GetLinkBySlug("abc123")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LongURL != link.LongURL {
		t.Errorf("expected %q, got %q", link.LongURL, got.LongURL)
	}
}

func TestStoreDelete(t *testing.T) {
	db := newTestDB(t)
	s, err := New(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	link := &model.ShortLink{
		ID:        1,
		Slug:      "xyz789",
		LongURL:   "https://example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.InsertLink(link)

	if err := s.DeleteLink("xyz789"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = s.GetLinkBySlug("xyz789")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows after delete, got %v", err)
	}
}

func TestStoreList(t *testing.T) {
	db := newTestDB(t)
	s, err := New(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	for i := int64(1); i <= 5; i++ {
		s.InsertLink(&model.ShortLink{
			ID:        i,
			Slug:      "slug" + string(rune('0'+i)),
			LongURL:   "https://example.com/" + string(rune('0'+i)),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
	}

	links, total, err := s.ListLinks(1, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total < 5 {
		t.Errorf("expected at least 5 total, got %d", total)
	}
	if len(links) < 5 {
		t.Errorf("expected at least 5 links, got %d", len(links))
	}
}

func TestStoreClickStats(t *testing.T) {
	db := newTestDB(t)
	s, err := New(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	for i := 0; i < 5; i++ {
		s.InsertClick("test", "https://ref.com", "GoTest", "hash")
	}

	total, err := s.GetClickStats("test")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if total != 5 {
		t.Errorf("expected 5 clicks, got %d", total)
	}
}

func TestStoreSlugExists(t *testing.T) {
	db := newTestDB(t)
	s, err := New(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	exists, err := s.SlugExists("nonexistent")
	if err != nil {
		t.Fatalf("slug exists: %v", err)
	}
	if exists {
		t.Error("expected slug to not exist")
	}

	s.InsertLink(&model.ShortLink{
		ID:        1,
		Slug:      "exists1",
		LongURL:   "https://example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	exists, err = s.SlugExists("exists1")
	if err != nil {
		t.Fatalf("slug exists: %v", err)
	}
	if !exists {
		t.Error("expected slug to exist")
	}
}

func TestStoreExpiresAt(t *testing.T) {
	db := newTestDB(t)
	s, err := New(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	expiry := time.Now().Add(24 * time.Hour)
	link := &model.ShortLink{
		ID:        1,
		Slug:      "exp123",
		LongURL:   "https://example.com",
		ExpiresAt: &expiry,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.InsertLink(link)

	got, err := s.GetLinkBySlug("exp123")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ExpiresAt == nil {
		t.Fatal("expected expires_at to be set")
	}
	if got.ExpiresAt.Unix() != expiry.Unix() {
		t.Errorf("expiry mismatch: %v != %v", got.ExpiresAt, expiry)
	}
}