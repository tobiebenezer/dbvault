package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/dbvault/dbvault/internal/ports"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Store struct{ Root string }

func New(root string) *Store                        { return &Store{Root: root} }
func (s *Store) Name() string                       { return "filesystem" }
func (s *Store) Validate(ctx context.Context) error { return os.MkdirAll(s.Root, 0700) }
func (s *Store) Capabilities(ctx context.Context) (ports.StorageCapabilities, error) {
	return ports.StorageCapabilities{ConditionalCreate: true, BatchDelete: true, UserMetadata: false, RangeReads: true}, nil
}
func (s *Store) safePath(key string) (string, error) {
	clean := filepath.Clean("/" + key)
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid key")
	}
	return filepath.Join(s.Root, clean), nil
}
func (s *Store) Put(ctx context.Context, req ports.PutObjectRequest) (ports.StoredObject, error) {
	p, err := s.safePath(req.Key)
	if err != nil {
		return ports.StoredObject{}, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return ports.StoredObject{}, err
	}
	if req.IfNotExists {
		if _, err := os.Stat(p); err == nil {
			return s.Head(ctx, req.Key)
		}
	}
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return ports.StoredObject{}, err
	}
	h := sha256.New()
	n, err := io.Copy(f, io.TeeReader(req.Body, h))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return ports.StoredObject{}, err
	}
	if err := os.Rename(tmp, p); err != nil {
		return ports.StoredObject{}, err
	}
	st, _ := os.Stat(p)
	return ports.StoredObject{Key: req.Key, Size: n, ETag: hex.EncodeToString(h.Sum(nil)), LastModified: st.ModTime().UTC()}, nil
}
func (s *Store) Get(ctx context.Context, req ports.GetObjectRequest) (ports.ObjectReader, ports.StoredObject, error) {
	p, err := s.safePath(req.Key)
	if err != nil {
		return nil, ports.StoredObject{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, ports.StoredObject{}, err
	}
	info, err := s.Head(ctx, req.Key)
	if err != nil {
		_ = f.Close()
		return nil, ports.StoredObject{}, err
	}
	return f, info, nil
}
func (s *Store) Head(ctx context.Context, key string) (ports.StoredObject, error) {
	p, err := s.safePath(key)
	if err != nil {
		return ports.StoredObject{}, err
	}
	st, err := os.Stat(p)
	if err != nil {
		return ports.StoredObject{}, err
	}
	return ports.StoredObject{Key: key, Size: st.Size(), LastModified: st.ModTime().UTC()}, nil
}
func (s *Store) List(ctx context.Context, req ports.ListObjectsRequest) (ports.ListObjectsResult, error) {
	out := []ports.StoredObject{}
	root := filepath.Join(s.Root, filepath.Clean(req.Prefix))
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return ports.ListObjectsResult{}, nil
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(s.Root, path)
		rel = filepath.ToSlash(rel)
		st, _ := d.Info()
		out = append(out, ports.StoredObject{Key: rel, Size: st.Size(), LastModified: st.ModTime().UTC()})
		return nil
	})
	return ports.ListObjectsResult{Objects: out}, err
}
func (s *Store) Delete(ctx context.Context, key string) error {
	p, err := s.safePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); os.IsNotExist(err) {
		return nil
	} else {
		return err
	}
}
