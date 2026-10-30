package main

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/platform"
)

func TestWorkBuddyControlConfiguration(t *testing.T) {
	getenv := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}
	t.Run("disabled_server_does_not_require_or_create_key", func(t *testing.T) {
		dir := t.TempDir()
		connections, err := configureWorkBuddyConnections(&platform.Service{Dialect: "oceanbase", DataDir: dir}, "", getenv(nil))
		if err != nil || connections == nil {
			t.Fatalf("disabled application: %v", err)
		}
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != 0 {
			t.Fatal("disabled configuration created credential files")
		}
	})
	t.Run("operator_application_and_callback_before_key", func(t *testing.T) {
		dir := t.TempDir()
		values := map[string]string{"SUMA_WORKBUDDY_CLIENT_ID": "owned-app", "SUMA_WORKBUDDY_CLIENT_SECRET": "private-fixture-credential", "SUMA_WORKBUDDY_SCOPES": "user.task.readable user.task.invokable"}
		for _, origin := range []string{"", "http://untrusted.example", "https://platform.example/path", "https://platform.example/path?callback=other"} {
			if _, err := configureWorkBuddyConnections(&platform.Service{Dialect: "sqlite", DataDir: dir}, origin, getenv(values)); err == nil || strings.Contains(err.Error(), values["SUMA_WORKBUDDY_CLIENT_SECRET"]) {
				t.Fatal("invalid callback accepted or credential leaked")
			}
		}
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != 0 {
			t.Fatal("invalid application created a key")
		}
	})
	t.Run("shared_database_requires_explicit_shared_key", func(t *testing.T) {
		values := map[string]string{"SUMA_WORKBUDDY_CLIENT_ID": "owned-app", "SUMA_WORKBUDDY_CLIENT_SECRET": "private-fixture-credential", "SUMA_WORKBUDDY_SCOPES": "user.task.readable user.task.invokable"}
		service := &platform.Service{Dialect: "oceanbase", DataDir: t.TempDir()}
		if _, err := configureWorkBuddyConnections(service, "https://platform.example", getenv(values)); err == nil {
			t.Fatal("shared database accepted an implicit local encryption key")
		}
		values["SUMA_WORKBUDDY_MASTER_KEY"] = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
		if _, err := configureWorkBuddyConnections(service, "https://platform.example", getenv(values)); err != nil {
			t.Fatalf("explicit shared application: %v", err)
		}
	})
	t.Run("invalid_or_unsupported_scopes_are_not_widened", func(t *testing.T) {
		values := map[string]string{"SUMA_WORKBUDDY_CLIENT_ID": "owned-app", "SUMA_WORKBUDDY_CLIENT_SECRET": "private-fixture-credential", "SUMA_WORKBUDDY_SCOPES": "user.task.readable,user.task.readable"}
		for _, scopes := range []string{values["SUMA_WORKBUDDY_SCOPES"], "admin.all", " "} {
			values["SUMA_WORKBUDDY_SCOPES"] = scopes
			if _, err := configureWorkBuddyConnections(&platform.Service{Dialect: "sqlite", DataDir: t.TempDir()}, "http://127.0.0.1:4310", getenv(values)); err == nil {
				t.Fatal("invalid scope list accepted")
			}
		}
	})
}
