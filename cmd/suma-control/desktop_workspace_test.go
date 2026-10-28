package main

import (
	"net"
	"strings"
	"testing"
)

func TestDesktopWorkspaceKeyIsLocalSQLiteOnly(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1"} {
		key, err := desktopWorkspaceKey(strings.Repeat("ab", 32), "sqlite", net.ParseIP(ip))
		if err != nil || len(key) != 32 {
			t.Fatal(ip, err)
		}
	}
	for _, value := range []struct{ key, driver, ip string }{
		{strings.Repeat("ab", 32), "sqlite", "0.0.0.0"},
		{strings.Repeat("ab", 32), "sqlite", "192.0.2.1"},
		{strings.Repeat("ab", 32), "oceanbase", "127.0.0.1"},
		{strings.Repeat("ab", 32), "postgres", "127.0.0.1"},
		{strings.Repeat("AB", 32), "sqlite", "127.0.0.1"},
		{strings.Repeat("g", 64), "sqlite", "127.0.0.1"},
		{"short", "sqlite", "127.0.0.1"},
	} {
		if _, err := desktopWorkspaceKey(value.key, value.driver, net.ParseIP(value.ip)); err == nil || strings.Contains(err.Error(), value.key) {
			t.Fatal("invalid desktop trust configuration was accepted or leaked its key")
		}
	}
	if key, err := desktopWorkspaceKey("", "postgres", nil); err != nil || key != nil {
		t.Fatal("ordinary server mode changed", err)
	}
}
