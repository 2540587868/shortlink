package slug

const base62Chars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

var encodeTable [62]byte
var decodeTable [256]byte

func init() {
	for i := 0; i < 62; i++ {
		encodeTable[i] = base62Chars[i]
	}
	for i := range decodeTable {
		decodeTable[i] = 255
	}
	for i := 0; i < 62; i++ {
		decodeTable[base62Chars[i]] = byte(i)
	}
}

type Generator struct {
	sf     *Snowflake
	lastID int64
}

func NewGenerator() *Generator {
	return &Generator{sf: NewSnowflake(0)}
}

func (g *Generator) Generate() string {
	g.lastID = g.sf.Next()
	return g.Encode6(g.lastID & 0xFFFFFFFFF)
}

func (g *Generator) LastID() int64 {
	return g.lastID
}

func (g *Generator) Encode6(id int64) string {
	var buf [6]byte
	n := id
	buf[5] = encodeTable[n%62]
	n /= 62
	buf[4] = encodeTable[n%62]
	n /= 62
	buf[3] = encodeTable[n%62]
	n /= 62
	buf[2] = encodeTable[n%62]
	n /= 62
	buf[1] = encodeTable[n%62]
	n /= 62
	buf[0] = encodeTable[n%62]
	return string(buf[:])
}

func Decode6(slug string) int64 {
	if len(slug) != 6 {
		return -1
	}
	var id int64
	for i := 0; i < 6; i++ {
		v := decodeTable[slug[i]]
		if v == 255 {
			return -1
		}
		id = id*62 + int64(v)
	}
	return id
}