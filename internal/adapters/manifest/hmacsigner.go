package manifest

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
)

type HMACSigner struct{ key []byte }

func NewHMACSigner(key []byte) HMACSigner { return HMACSigner{key: key} }
func (s HMACSigner) Sign(payload []byte) ([]byte, error) {
	h := hmac.New(sha256.New, s.key)
	h.Write(payload)
	return h.Sum(nil), nil
}
func (s HMACSigner) Verify(payload, sig []byte) error {
	exp, _ := s.Sign(payload)
	if !hmac.Equal(exp, sig) {
		return fmt.Errorf("invalid manifest signature")
	}
	return nil
}
