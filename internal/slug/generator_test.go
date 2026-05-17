package slug

import (
	"testing"
)

func TestEncodeDecode6(t *testing.T) {
	g := NewGenerator()

	for i := 0; i < 1000; i++ {
		slug := g.Generate()
		if len(slug) != 6 {
			t.Fatalf("expected slug length 6, got %d for %q", len(slug), slug)
		}

		id := Decode6(slug)
		if id < 0 {
			t.Fatalf("decode failed for slug %q", slug)
		}

		reEncoded := g.Encode6(id)
		if reEncoded != slug {
			t.Errorf("re-encode mismatch: %q != %q (id=%d)", reEncoded, slug, id)
		}
	}
}

func TestGenerateUniqueness(t *testing.T) {
	g := NewGenerator()
	seen := make(map[string]bool, 10000)
	for i := 0; i < 10000; i++ {
		slug := g.Generate()
		if seen[slug] {
			t.Fatalf("duplicate slug generated: %q at iteration %d", slug, i)
		}
		seen[slug] = true
	}
}

func TestDecode6InvalidInput(t *testing.T) {
	tests := []struct {
		input string
	}{
		{""},
		{"ab"},
		{"@#$%^&"},
		{"      "},
	}

	for _, tt := range tests {
		id := Decode6(tt.input)
		if id >= 0 {
			t.Errorf("expected negative result for input %q, got %d", tt.input, id)
		}
	}
}

func TestSnowflakeMonotonic(t *testing.T) {
	sf := NewSnowflake(0)
	var last int64
	for i := 0; i < 1000; i++ {
		id := sf.Next()
		if id <= last {
			t.Fatalf("snowflake not monotonic: %d <= %d", id, last)
		}
		last = id
	}
}

func BenchmarkGenerate(b *testing.B) {
	g := NewGenerator()
	for i := 0; i < b.N; i++ {
		g.Generate()
	}
}

func BenchmarkDecode6(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Decode6("abc123")
	}
}