package local

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	Path string
	Key  []byte
	mu   sync.Mutex
}

type Record struct {
	Name       string `json:"name"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type fileFormat struct {
	Records map[string]Record `json:"records"`
}

func New(path string, key []byte) (*Store, error) {
	if len(key) != 32 {
		return nil, errors.New("secret-store master key must be 32 bytes")
	}
	return &Store{Path: path, Key: append([]byte(nil), key...)}, nil
}

func (s *Store) Put(name string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ff, err := s.read()
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(s.Key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nil, nonce, value, []byte(name))
	ff.Records[name] = Record{Name: name, Nonce: base64.StdEncoding.EncodeToString(nonce), Ciphertext: base64.StdEncoding.EncodeToString(ciphertext)}
	return s.write(ff)
}

func (s *Store) Get(name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ff, err := s.read()
	if err != nil {
		return nil, err
	}
	rec, ok := ff.Records[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	nonce, err := base64.StdEncoding.DecodeString(rec.Nonce)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(rec.Ciphertext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(s.Key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, []byte(name))
}

func (s *Store) read() (fileFormat, error) {
	ff := fileFormat{Records: map[string]Record{}}
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return ff, nil
	}
	if err != nil {
		return ff, err
	}
	if len(b) == 0 {
		return ff, nil
	}
	if err := json.Unmarshal(b, &ff); err != nil {
		return ff, err
	}
	if ff.Records == nil {
		ff.Records = map[string]Record{}
	}
	return ff, nil
}

func (s *Store) write(ff fileFormat) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(ff, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, b, 0600)
}
