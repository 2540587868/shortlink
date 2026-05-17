package analytics

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ysqss/shortlink/internal/model"
)

type Store interface {
	InsertClick(slug, referer, ua, ipHash string) error
}

type Analytics struct {
	ch       chan *model.ClickEvent
	store    Store
	dropped  atomic.Int64
	inflight atomic.Int64
	wg       sync.WaitGroup
}

func New(st Store, bufferSize int) *Analytics {
	a := &Analytics{
		ch:    make(chan *model.ClickEvent, bufferSize),
		store: st,
	}
	for i := 0; i < 5; i++ {
		a.wg.Add(1)
		go a.worker()
	}
	return a
}

func (a *Analytics) Record(event *model.ClickEvent) {
	select {
	case a.ch <- event:
		a.inflight.Add(1)
	default:
		a.dropped.Add(1)
		slog.Warn("analytics buffer full, dropping event")
	}
}

func (a *Analytics) Dropped() int64 {
	return a.dropped.Load()
}

func (a *Analytics) Inflight() int64 {
	return a.inflight.Load()
}

func (a *Analytics) Shutdown(timeout time.Duration) {
	close(a.ch)
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("analytics shutdown complete")
	case <-time.After(timeout):
		slog.Warn("analytics shutdown timed out")
	}
}

func (a *Analytics) worker() {
	defer a.wg.Done()
	for event := range a.ch {
		if err := a.store.InsertClick(
			event.Slug,
			truncate(event.Referer, 2000),
			truncate(event.UserAgent, 500),
			event.IP,
		); err != nil {
			slog.Error("failed to record click", "error", err)
		}
		a.inflight.Add(-1)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
