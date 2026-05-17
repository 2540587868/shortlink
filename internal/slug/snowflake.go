package slug

import (
	"sync"
	"time"
)

const epoch int64 = 1767225600000

type Snowflake struct {
	mu    sync.Mutex
	shard int64
	seq   int64
	last  int64
}

func NewSnowflake(shard int64) *Snowflake {
	return &Snowflake{shard: shard & 0xFF}
}

func (s *Snowflake) Next() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli() - epoch
	if now == s.last {
		s.seq = (s.seq + 1) & 0x1FFF
		if s.seq == 0 {
			for now <= s.last {
				now = time.Now().UnixMilli() - epoch
			}
		}
	} else {
		s.seq = 0
	}
	s.last = now

	return (now << 21) | (s.shard << 13) | s.seq
}
