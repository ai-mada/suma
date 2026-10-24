package platform

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAutomationIngressMasterKeyConfigurationFailsClosed(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	for _, tc := range []struct {
		name, dialect, value string
		wantError            bool
	}{
		{"environment", "sqlite", base64.StdEncoding.EncodeToString(key), false},
		{"shared postgres", "postgres", base64.StdEncoding.EncodeToString(key), false},
		{"missing shared key", "postgres", "", true},
		{"missing oceanbase key", "oceanbase", "", true},
		{"malformed key", "sqlite", "not-base64", true},
		{"short key", "sqlite", base64.StdEncoding.EncodeToString(key[:31]), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "data")
			got, err := LoadAutomationIngressKey(dir, tc.dialect, func(string) string { return tc.value })
			if (err != nil) != tc.wantError {
				t.Fatalf("key configuration result: %v", err)
			}
			if err == nil && !bytes.Equal(got, key) {
				t.Fatal("configured key changed")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("environment configuration unexpectedly wrote local key")
			}
		})
	}
}

func TestAutomationIngressLocalMasterKeyPersistsAndRejectsPartialFile(t *testing.T) {
	dir := t.TempDir()
	getenv := func(string) string { return "" }
	one, err := LoadAutomationIngressKey(dir, "sqlite", getenv)
	if err != nil || len(one) != 32 {
		t.Fatal("local key not created", err)
	}
	two, err := LoadAutomationIngressKey(dir, "sqlite", getenv)
	if err != nil || !bytes.Equal(one, two) {
		t.Fatal("restart changed key", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("unexpected local key files", err)
	}
	path := filepath.Join(dir, entries[0].Name())
	if err = os.WriteFile(path, []byte("partial-crash"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadAutomationIngressKey(dir, "sqlite", getenv); err == nil {
		t.Fatal("corrupt key silently replaced")
	}
	content, _ := os.ReadFile(path)
	if string(content) != "partial-crash" {
		t.Fatal("failed load changed persisted evidence")
	}
}

func TestAutomationIngressConcurrentLocalKeyNeverReturnsDifferentKeys(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	keys := make(chan []byte, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key, err := LoadAutomationIngressKey(dir, "sqlite", func(string) string { return "" })
			if err == nil {
				keys <- key
			}
		}()
	}
	wg.Wait()
	close(keys)
	persisted, err := LoadAutomationIngressKey(dir, "sqlite", func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for key := range keys {
		count++
		if !bytes.Equal(key, persisted) {
			t.Fatal("concurrent initialization issued different usable keys")
		}
	}
	if count == 0 {
		t.Fatal("no creator returned a usable key")
	}
}
