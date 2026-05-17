package cache

import (
	"strconv"
	"sync"
	"testing"
)

func TestShardedLRUBasic(t *testing.T) {
	c := NewShardedLRU[string, string](100)

	c.Set("key1", "value1")
	v, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected key1 to exist")
	}
	if v != "value1" {
		t.Errorf("expected value1, got %q", v)
	}

	_, ok = c.Get("key2")
	if ok {
		t.Error("expected key2 to not exist")
	}
}

func TestShardedLRUEviction(t *testing.T) {
	c := NewShardedLRU[int, int](10)

	for i := 0; i < 20; i++ {
		c.Set(i, i*10)
	}

	if c.Len() > 10 {
		t.Errorf("expected <= 10 items after eviction, got %d", c.Len())
	}

	c.Set(0, 0)
	v, ok := c.Get(0)
	if !ok || v != 0 {
		t.Errorf("expected key 0 to be refreshed, ok=%v v=%d", ok, v)
	}
}

func TestShardedLRUDelete(t *testing.T) {
	c := NewShardedLRU[string, int](100)

	c.Set("a", 1)
	c.Set("b", 2)
	c.Delete("a")

	_, ok := c.Get("a")
	if ok {
		t.Error("expected 'a' to be deleted")
	}

	v, ok := c.Get("b")
	if !ok || v != 2 {
		t.Error("expected 'b' to still exist")
	}
}

func TestShardedLRUConcurrency(t *testing.T) {
	c := NewShardedLRU[string, int](1000)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := strconv.Itoa(idx)
			for j := 0; j < 100; j++ {
				c.Set(key, j)
				c.Get(key)
			}
		}(i)
	}
	wg.Wait()
}

func TestShardedLRUSmallCapacity(t *testing.T) {
	c := NewShardedLRU[string, string](5)

	c.Set("a", "1")
	c.Set("b", "2")
	c.Set("c", "3")
	c.Set("d", "4")
	c.Set("e", "5")
	c.Set("f", "6")

	if c.Len() > 5 {
		t.Errorf("expected <= 5 items, got %d", c.Len())
	}
}

func BenchmarkShardedLRUGet(b *testing.B) {
	c := NewShardedLRU[string, string](10000)
	for i := 0; i < 10000; i++ {
		c.Set(strconv.Itoa(i), "value")
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(strconv.Itoa(i % 10000))
			i++
		}
	})
}

func BenchmarkShardedLRUSet(b *testing.B) {
	c := NewShardedLRU[string, string](10000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Set(strconv.Itoa(i%100000), "value")
			i++
		}
	})
}
