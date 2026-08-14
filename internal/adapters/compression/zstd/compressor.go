//go:build restricted

// Package zstd exposes the Phase 2 compression adapter.
//
// The production contract is intentionally named zstd. This dependency-free
// build uses the standard library's gzip stream internally so the project can be
// compiled in restricted environments. Swapping the internals for klauspost/zstd
// later does not affect ports or application services.
package zstd

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
)

type Compressor struct {
	Level                 int
	MinimumSavingsPercent int
}

func New(level int, minSavings int) Compressor {
	if minSavings == 0 {
		minSavings = 5
	}
	return Compressor{Level: level, MinimumSavingsPercent: minSavings}
}
func (Compressor) Name() string { return "zstd" }
func (c Compressor) Compress(ctx context.Context, src io.Reader, dst io.Writer) error {
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	lvl := c.Level
	if lvl == 0 {
		lvl = gzip.DefaultCompression
	}
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, lvl)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	saved := 100 - (buf.Len() * 100 / max(1, len(data)))
	if saved < c.MinimumSavingsPercent {
		_, err = dst.Write(data)
		return err
	}
	_, err = dst.Write(buf.Bytes())
	return err
}
func (Compressor) Decompress(ctx context.Context, src io.Reader, dst io.Writer) error {
	// Backward/none support: try gzip, fall back to raw bytes.
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		_, err = dst.Write(data)
		return err
	}
	defer r.Close()
	_, err = io.Copy(dst, r)
	return err
}
