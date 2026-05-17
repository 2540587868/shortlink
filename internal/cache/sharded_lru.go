package cache

import (
	"sync"
)

type entry[K comparable, V any] struct {
	key   K
	value V
	prev  *entry[K, V]
	next  *entry[K, V]
}

type lruShard[K comparable, V any] struct {
	mu       sync.Mutex
	items    map[K]*entry[K, V]
	head     *entry[K, V]
	tail     *entry[K, V]
	capacity int
}

func newLRUShard[K comparable, V any](capacity int) *lruShard[K, V] {
	s := &lruShard[K, V]{
		items:    make(map[K]*entry[K, V], capacity),
		capacity: capacity,
	}
	s.head = &entry[K, V]{}
	s.tail = &entry[K, V]{}
	s.head.next = s.tail
	s.tail.prev = s.head
	return s
}

func (s *lruShard[K, V]) get(key K) (V, bool) {
	e, ok := s.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	s.moveToFront(e)
	return e.value, true
}

func (s *lruShard[K, V]) set(key K, value V) {
	if e, ok := s.items[key]; ok {
		e.value = value
		s.moveToFront(e)
		return
	}
	e := &entry[K, V]{key: key, value: value}
	s.items[key] = e
	s.addToFront(e)
	if len(s.items) > s.capacity {
		s.removeOldest()
	}
}

func (s *lruShard[K, V]) del(key K) {
	if e, ok := s.items[key]; ok {
		s.remove(e)
		delete(s.items, key)
	}
}

func (s *lruShard[K, V]) len() int {
	return len(s.items)
}

func (s *lruShard[K, V]) addToFront(e *entry[K, V]) {
	e.prev = s.head
	e.next = s.head.next
	s.head.next.prev = e
	s.head.next = e
}

func (s *lruShard[K, V]) remove(e *entry[K, V]) {
	e.prev.next = e.next
	e.next.prev = e.prev
}

func (s *lruShard[K, V]) moveToFront(e *entry[K, V]) {
	s.remove(e)
	s.addToFront(e)
}

func (s *lruShard[K, V]) removeOldest() {
	if s.tail.prev == s.head {
		return
	}
	e := s.tail.prev
	s.remove(e)
	delete(s.items, e.key)
}

type ShardedLRU[K comparable, V any] struct {
	shards    []*lruShard[K, V]
	shardMask uint32
}

func NewShardedLRU[K comparable, V any](totalCapacity int) *ShardedLRU[K, V] {
	shardCount := 256
	if totalCapacity < shardCount {
		shardCount = totalCapacity
	}
	perShard := totalCapacity / shardCount
	if perShard < 1 {
		perShard = 1
	}

	shards := make([]*lruShard[K, V], shardCount)
	for i := range shards {
		shards[i] = newLRUShard[K, V](perShard)
	}

	return &ShardedLRU[K, V]{
		shards:    shards,
		shardMask: uint32(shardCount - 1),
	}
}

func (c *ShardedLRU[K, V]) getShard(key K) *lruShard[K, V] {
	h := hashKey(key)
	return c.shards[h&c.shardMask]
}

func (c *ShardedLRU[K, V]) Get(key K) (V, bool) {
	s := c.getShard(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(key)
}

func (c *ShardedLRU[K, V]) Set(key K, value V) {
	s := c.getShard(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.set(key, value)
}

func (c *ShardedLRU[K, V]) Delete(key K) {
	s := c.getShard(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.del(key)
}

func (c *ShardedLRU[K, V]) Len() int {
	total := 0
	for _, s := range c.shards {
		s.mu.Lock()
		total += s.len()
		s.mu.Unlock()
	}
	return total
}

func hashKey[K comparable](key K) uint32 {
	s := any(key)
	switch v := s.(type) {
	case string:
		return fnv32(v)
	case int:
		return uint32(v)
	case int64:
		return uint32(v) ^ uint32(v>>32)
	default:
		return fnv32(anyToString(key))
	}
}

func fnv32(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func anyToString[K comparable](key K) string {
	return anyToStringSlow(key)
}

func anyToStringSlow(key any) string {
	switch v := key.(type) {
	case string:
		return v
	default:
		return ""
	}
}
