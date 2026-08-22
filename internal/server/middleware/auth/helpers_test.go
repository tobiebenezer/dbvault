package auth

import (
	"fmt"
	"os"
)

func statMode(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%04o", uint32(info.Mode().Perm())), nil
}
