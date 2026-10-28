package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"
)

// Native directory approvals apply only to the desktop's own local SQLite
// control service. A supplied key must never silently enable the same trust
// boundary on a network listener or a shared server database.
func desktopWorkspaceKey(raw, driver string, address net.IP) ([]byte, error) {
	if raw == "" {
		return nil, nil
	}
	if address == nil || !address.IsLoopback() || driver != "sqlite" || len(raw) != 64 || raw != strings.ToLower(raw) {
		return nil, fmt.Errorf("desktop workspace configuration requires a local SQLite service and a valid key")
	}
	key, err := hex.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("desktop workspace configuration requires a local SQLite service and a valid key")
	}
	return key, nil
}
