package analytics

import (
	"sync"
	"testing"
	"time"

	"github.com/ysqss/shortlink/internal/model"
)

type mockStore struct {
	mu     sync.Mutex
	clicks []string
}

func (m *mockStore) InsertClick(slug, referer, ua, ipHash string) error {
	m.mu.Lock()
	m.clicks = append(m.clicks, slug)
	m.mu.Unlock()
	return nil
}

func TestAnalyticsRecordAndShutdown(t *testing.T) {
	m := &mockStore{}
	a := New(m, 100)

	for i := 0; i < 10; i++ {
		a.Record(&model.ClickEvent{
			Slug:      "test-slug",
			Timestamp: time.Now(),
		})
	}

	a.Shutdown(5 * time.Second)

	m.mu.Lock()
	count := len(m.clicks)
	m.mu.Unlock()

	if count != 10 {
		t.Errorf("expected 10 clicks recorded, got %d", count)
	}
}

func TestAnalyticsBufferFull(t *testing.T) {
	m := &mockStore{}
	a := New(m, 1)

	for i := 0; i < 100; i++ {
		a.Record(&model.ClickEvent{Slug: "test", Timestamp: time.Now()})
	}

	dropped := a.Dropped()
	if dropped == 0 {
		t.Error("expected some events to be dropped with buffer size 1")
	}
}