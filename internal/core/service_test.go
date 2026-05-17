package core

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ysqss/shortlink/internal/cache"
	"github.com/ysqss/shortlink/internal/model"
	"github.com/ysqss/shortlink/internal/store"
)

func newTestService(t *testing.T) Service {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	st, err := store.New(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	c := cache.NewShardedLRU[string, *model.ShortLink](1000)
	return NewService(st, c)
}

func TestServiceCreateAndLookup(t *testing.T) {
	svc := newTestService(t)

	link, err := svc.Create(model.CreateOptions{}, "https://example.com/very/long/path")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if link.Slug == "" {
		t.Fatal("expected non-empty slug")
	}

	got, err := svc.Lookup(link.Slug)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.LongURL != "https://example.com/very/long/path" {
		t.Errorf("expected original URL, got %q", got.LongURL)
	}
}

func TestServiceCustomSlug(t *testing.T) {
	svc := newTestService(t)

	link, err := svc.Create(model.CreateOptions{CustomSlug: "my-link"}, "https://example.com")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if link.Slug != "my-link" {
		t.Errorf("expected 'my-link', got %q", link.Slug)
	}

	_, err = svc.Create(model.CreateOptions{CustomSlug: "my-link"}, "https://other.com")
	if err != model.ErrSlugExists {
		t.Errorf("expected ErrSlugExists, got %v", err)
	}
}

func TestServiceDelete(t *testing.T) {
	svc := newTestService(t)

	link, _ := svc.Create(model.CreateOptions{}, "https://example.com")
	if err := svc.Delete(link.Slug); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := svc.Lookup(link.Slug)
	if err != model.ErrSlugNotFound {
		t.Errorf("expected ErrSlugNotFound after delete, got %v", err)
	}
}

func TestServiceTTL(t *testing.T) {
	svc := newTestService(t)

	link, err := svc.Create(model.CreateOptions{TTL: 1 * time.Second}, "https://example.com")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	_, err = svc.Lookup(link.Slug)
	if err != model.ErrLinkExpired {
		t.Errorf("expected ErrLinkExpired, got %v", err)
	}
}

func TestServiceList(t *testing.T) {
	svc := newTestService(t)

	for i := 0; i < 10; i++ {
		svc.Create(model.CreateOptions{}, "https://example.com/"+string(rune('a'+i)))
	}

	links, total, err := svc.List(1, 5)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total < 10 {
		t.Errorf("expected total >= 10, got %d", total)
	}
	if len(links) != 5 {
		t.Errorf("expected 5 links on first page, got %d", len(links))
	}
}

func TestServiceSearch(t *testing.T) {
	svc := newTestService(t)

	svc.Create(model.CreateOptions{CustomSlug: "hello-world"}, "https://hello.example.com")
	svc.Create(model.CreateOptions{CustomSlug: "goodbye"}, "https://bye.example.com")

	results, err := svc.Search("hello")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) < 1 {
		t.Error("expected at least 1 search result")
	}
}

func TestServiceNotFound(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Lookup("nonexistent")
	if err != model.ErrSlugNotFound {
		t.Errorf("expected ErrSlugNotFound, got %v", err)
	}
}

func TestServiceInvalidCreate(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Create(model.CreateOptions{}, "")
	if err == nil {
		t.Error("expected error for empty long_url")
	}
}

func BenchmarkServiceCreate(b *testing.B) {
	svc := newTestService(&testing.T{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Create(model.CreateOptions{}, "https://example.com/bench")
	}
}

func BenchmarkServiceLookup(b *testing.B) {
	svc := newTestService(&testing.T{})
	link, _ := svc.Create(model.CreateOptions{CustomSlug: "bench1"}, "https://example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Lookup(link.Slug)
	}
}