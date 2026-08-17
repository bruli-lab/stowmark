package snapshot

const (
	ChunkThreshold int64 = 8 * 1024 * 1024
	ChunkSize      int64 = 4 * 1024 * 1024
)

type Chunk struct {
	hash   string
	offset int64
	size   int64
}

func (c Chunk) Hash() string {
	return c.hash
}

func (c Chunk) Offset() int64 {
	return c.offset
}

func (c Chunk) Size() int64 {
	return c.size
}

func NewChunk(hash string, offset, size int64) *Chunk {
	return &Chunk{
		hash:   hash,
		offset: offset,
		size:   size,
	}
}
