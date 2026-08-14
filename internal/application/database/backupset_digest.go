package database

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/dbvault/dbvault/internal/domain"
)

func BackupSetDigest(key []byte, set domain.BackupSet) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(set.Engine))
	h.Write([]byte{0})
	h.Write([]byte(set.Mode))
	h.Write([]byte{0})
	h.Write([]byte(set.Format))
	h.Write([]byte{0})
	arts := append([]domain.BackupArtifact(nil), set.Artifacts...)
	sort.Slice(arts, func(i, j int) bool { return arts[i].Sequence < arts[j].Sequence })
	for _, a := range arts {
		parts := []string{string(a.ID), string(a.Type), string(a.Format), a.RootDigest}
		if a.Required {
			parts = append(parts, "required")
		} else {
			parts = append(parts, "optional")
		}
		h.Write([]byte(strings.Join(parts, "|")))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
