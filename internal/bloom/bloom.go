package bloom

import (
	"hash/fnv"
	"sync"
)

type Filter struct {
	bits []uint64
	m    uint64
	k    int
	mu   sync.RWMutex
}

func New(expectedItems int, fpRate float64) *Filter {
	m := optimalM(expectedItems, fpRate)
	k := optimalK(expectedItems, m)
	size := (m + 63) / 64
	return &Filter{
		bits: make([]uint64, size),
		m:    m,
		k:    k,
	}
}

func NewWithSize(size int) *Filter {
	m := uint64(size)
	k := 3
	bitsSize := (m + 63) / 64
	return &Filter{
		bits: make([]uint64, bitsSize),
		m:    m,
		k:    k,
	}
}

func (f *Filter) Add(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h1, h2 := f.hash(key)
	for i := 0; i < f.k; i++ {
		pos := (h1 + uint64(i)*h2) % f.m
		f.bits[pos/64] |= 1 << (pos % 64)
	}
}

func (f *Filter) Contains(key string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	h1, h2 := f.hash(key)
	for i := 0; i < f.k; i++ {
		pos := (h1 + uint64(i)*h2) % f.m
		if f.bits[pos/64]&(1<<(pos%64)) == 0 {
			return false
		}
	}
	return true
}

func (f *Filter) hash(key string) (uint64, uint64) {
	h := fnv.New64a()
	h.Write([]byte(key))
	h1 := h.Sum64()
	h.Write([]byte{0})
	h2 := h.Sum64()
	return h1, h2
}

func optimalM(n int, p float64) uint64 {
	return uint64(float64(-n) * 0.6931471805599453 / (1 - 1/float64(n)) + 0.5)
}

func optimalK(n int, m uint64) int {
	k := int(float64(m) / float64(n) * 0.6931471805599453)
	if k < 1 {
		k = 1
	}
	if k > 30 {
		k = 30
	}
	return k
}