package platform

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LoadAutomationIngressKey loads the administrator's shared encryption key.
// SQLite may initialize a local key; an incomplete existing file is never
// replaced. A racing reader may fail startup while the creator finishes,
// but cannot return a different usable key or fall back to plaintext.
func LoadAutomationIngressKey(dataDir, dialect string, getenv func(string) string) ([]byte, error) {
	if value := getenv("SUMA_INGRESS_MASTER_KEY"); value != "" {
		key, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(key) != 32 || base64.StdEncoding.EncodeToString(key) != value {
			return nil, fmt.Errorf("SUMA_INGRESS_MASTER_KEY must be canonical base64 encoding of 32 bytes")
		}
		return key, nil
	}
	if dialect != "sqlite" {
		return nil, fmt.Errorf("SUMA_INGRESS_MASTER_KEY is required for shared databases")
	}
	if dataDir == "" {
		return nil, fmt.Errorf("ingress encryption requires a persistent data directory")
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("create ingress key directory: %w", err)
	}
	path := filepath.Join(dataDir, "automation-ingress.key")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return readAutomationIngressKey(path)
	}
	if err != nil {
		return nil, fmt.Errorf("create ingress key: %w", err)
	}
	key := make([]byte, 32)
	if _, err = io.ReadFull(rand.Reader, key); err == nil {
		_, err = f.Write(key)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, fmt.Errorf("persist ingress key: %w", err)
	}
	return key, nil
}

func readAutomationIngressKey(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != 32 {
		return nil, fmt.Errorf("existing ingress key must be a regular file of exactly 32 bytes")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open ingress key: %w", err)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("ingress key changed while opening")
	}
	key, err := io.ReadAll(io.LimitReader(f, 33))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("existing ingress key is incomplete or unreadable")
	}
	return key, nil
}
