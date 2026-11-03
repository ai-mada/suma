package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestManagedToolPreparationFlagRejectsOfflineModesBeforeIdentity(t *testing.T) {
	for _, args := range [][]string{
		{"--allow-tool-preparation"},
		{"--allow-tool-preparation=false", "--report-tools"},
		{"--allow-tool-preparation", "--allow-installed-tool-discovery", "--prepare-tools=unused"},
		{"--allow-tool-preparation=false", "--compose-commands=unused"},
		{"--allow-tool-preparation", "--allow-installed-tool-discovery", "--pair-runner=unused"},
		{"--allow-tool-preparation=false", "--check-pool-connection=false"},
		{"--allow-tool-preparation=false", "--describe-tools"},
	} {
		var out, stderr bytes.Buffer
		code := run(append([]string{"--language=en"}, args...), &out, &stderr, func(name string) (string, bool) {
			if strings.Contains(name, "TOKEN") {
				t.Fatal("invalid maintenance mode read identity")
			}
			return "", false
		})
		if code != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "tool preparation requires") {
			t.Fatal("new local opt-in fell through to offline/identity mode", args, code, out.String(), stderr.String())
		}
	}
}
