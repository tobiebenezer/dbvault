package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type Generator struct{}

func (Generator) NewID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%d_%s", prefix, time.Now().UTC().UnixNano(), hex.EncodeToString(b))
}
