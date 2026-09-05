package ports

import (
	"context"
	"io"
)

type Compressor interface {
	Name() string
	Compress(ctx context.Context, src io.Reader, dst io.Writer) error
	Decompress(ctx context.Context, src io.Reader, dst io.Writer) error
}

type Encryptor interface {
	EncryptChunk(ctx context.Context, chunkID string, plaintext []byte) ([]byte, error)
	DecryptChunk(ctx context.Context, chunkID string, ciphertext []byte) ([]byte, error)
}

type ManifestSigner interface {
	Sign(payload []byte) ([]byte, error)
	Verify(payload, signature []byte) error
}

type ChunkMetadata struct {
	Index          int
	Sequence       int
	PlaintextSize  int64
	CompressedSize int64
	ChunkID        string
	ObjectKey      string
}
