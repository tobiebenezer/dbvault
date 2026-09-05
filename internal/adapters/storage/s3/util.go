package s3

import (
	"fmt"
	"os"
	"path"
	"strings"
)

func JoinObjectKey(prefix string, parts ...string) (string, error) {
	for _, part := range parts {
		for _, seg := range strings.Split(part, "/") {
			if seg == ".." {
				return "", fmt.Errorf("invalid object key: path traversal detected")
			}
		}
	}
	all := []string{}
	if prefix != "" {
		all = append(all, prefix)
	}
	all = append(all, parts...)
	joined := path.Clean(strings.Join(all, "/"))
	joined = strings.TrimPrefix(joined, "/")
	if joined == "." {
		return "", nil
	}
	for _, seg := range strings.Split(joined, "/") {
		if seg == ".." || seg == "" {
			return "", fmt.Errorf("invalid object key")
		}
	}
	return joined, nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func readSecret(v string) (string, error) {
	if strings.HasPrefix(v, "file:") {
		b, err := os.ReadFile(strings.TrimPrefix(v, "file:"))
		return strings.TrimSpace(string(b)), err
	}
	if strings.HasPrefix(v, "env:") {
		return os.Getenv(strings.TrimPrefix(v, "env:")), nil
	}
	return v, nil
}
