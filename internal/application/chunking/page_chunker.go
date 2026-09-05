package chunking

import (
	"context"
	"io"
	"os"

	"github.com/dbvault/dbvault/internal/ports"
)

type PlainChunk struct {
	Index          int
	Sequence       int
	PageStart      int64
	PageCount      int
	PlaintextSize  int64
	CompressedSize int64
	Data           []byte
}

type PageChunker struct{}

func NewPageChunker() PageChunker { return PageChunker{} }

func PagesPerChunk(pageSize int, targetBytes int64) int {
	if pageSize <= 0 {
		return 1
	}
	p := int(targetBytes / int64(pageSize))
	if p < 1 {
		return 1
	}
	return p
}

func (PageChunker) Chunks(ctx context.Context, art ports.SnapshotArtifact, targetBytes int64) ([]PlainChunk, error) {
	meta := art.Metadata()
	pagesPer := PagesPerChunk(meta.PageSize, targetBytes)
	f, err := os.Open(art.Path())
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := []PlainChunk{}
	bufSize := pagesPer * meta.PageSize
	seq := 0
	pageStart := int64(0)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		buf := make([]byte, bufSize)
		n, err := io.ReadFull(f, buf)
		if err == io.EOF {
			break
		}
		if err == io.ErrUnexpectedEOF {
			buf = buf[:n]
		} else if err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
		pc := n / meta.PageSize
		if n%meta.PageSize != 0 {
			pc++
		}
		out = append(out, PlainChunk{
			Index:          seq,
			Sequence:       seq,
			PageStart:      pageStart,
			PageCount:      pc,
			PlaintextSize:  int64(n),
			CompressedSize: int64(n),
			Data:           buf[:n],
		})
		seq++
		pageStart += int64(pc)
		if err == io.ErrUnexpectedEOF {
			break
		}
	}
	return out, nil
}
