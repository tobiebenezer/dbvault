package gzip

import (
	"compress/gzip"
	"context"
	"io"
)

type Compressor struct{ Level int }

func New(level int) Compressor  { return Compressor{Level: level} }
func (Compressor) Name() string { return "gzip" }
func (c Compressor) Compress(ctx context.Context, src io.Reader, dst io.Writer) error {
	lvl := c.Level
	if lvl == 0 {
		lvl = gzip.DefaultCompression
	}
	w, err := gzip.NewWriterLevel(dst, lvl)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, src); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}
func (Compressor) Decompress(ctx context.Context, src io.Reader, dst io.Writer) error {
	r, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	_, err = io.Copy(dst, r)
	return err
}
