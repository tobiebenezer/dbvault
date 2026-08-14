package digest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/dbvault/dbvault/internal/application/chunking"
)

func ChunkID(key []byte, pageSize int, data []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(fmt.Sprintf("dbvault-chunk-v1:%d:", pageSize)))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
func RootDigest(key []byte, chunks []chunking.PlainChunk, ids []string, logicalSize int64, pageSize int) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(fmt.Sprintf("dbvault-root-v1:%d:%d:", pageSize, logicalSize)))
	for i, id := range ids {
		h.Write([]byte(fmt.Sprintf("%d:%s:%d;", i, id, chunks[i].PlaintextSize)))
	}
	return hex.EncodeToString(h.Sum(nil))
}
