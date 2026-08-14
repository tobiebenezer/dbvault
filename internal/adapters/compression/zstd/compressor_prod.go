//go:build !restricted

package zstd

import (
	"bytes"
	"context"
	"io"

	kzstd "github.com/klauspost/compress/zstd"
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
	level := kzstd.SpeedDefault
	if c.Level >= 7 {
		level = kzstd.SpeedBetterCompression
	}
	if c.Level <= 2 {
		level = kzstd.SpeedFastest
	}
	enc, err := kzstd.NewWriter(nil, kzstd.WithEncoderLevel(level))
	if err != nil {
		return err
	}
	compressed := enc.EncodeAll(data, nil)
	saved := 100 - (len(compressed) * 100 / max(1, len(data)))
	if saved < c.MinimumSavingsPercent {
		_, err = dst.Write(data)
		return err
	}
	_, err = dst.Write(compressed)
	return err
}

func (Compressor) Decompress(ctx context.Context, src io.Reader, dst io.Writer) error {
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	if bytes.HasPrefix(data, []byte{0x28, 0xb5, 0x2f, 0xfd}) {
		dec, err := kzstd.NewReader(nil)
		if err != nil {
			return err
		}
		defer dec.Close()
		out, err := dec.DecodeAll(data, nil)
		if err != nil {
			return err
		}
		_, err = dst.Write(out)
		return err
	}
	// Raw fallback for incompressible v1/v2 chunks. The caller authenticates the bytes.
	_, err = dst.Write(data)
	return err
}
