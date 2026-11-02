package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestPoolConnectionCheckModeRejectsOtherOptionsBeforeSecrets(t *testing.T) {
	base := []string{"--language=en", "--check-pool-connection", "--control=https://control.example.test", "--pool-config=config212.json", "--pool-identity=identity212.json"}
	for _, option := range []string{"--allow-exec=false", "--allow-installed-tool-discovery=false", "--maintenance-only=false", "--state=state212", "--describe-tools=false", "--prepare-pool-config=new212.json", "--pool=other", "--pair-pool-runner=other", "--identity=other", "--project=other", "--report-tools=false", "--commands=other"} {
		var out, diagnostic bytes.Buffer
		code := runWithRunnerSecrets(append(append([]string{}, base...), option), &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t})
		if code != 1 || out.Len() != 0 {
			t.Fatal("check mode accepted other options or reached identity", option, code)
		}
	}
	for _, args := range [][]string{
		{"--check-pool-connection=false"},
		{"--check-pool-connection", "--pool-config=x", "--pool-identity=x"},
		{"--check-pool-connection", "--control=https://control.example.test", "--pool-config=x"},
		{"--check-pool-connection", "--control=https://control.example.test", "--pool-identity=x"},
	} {
		var out, diagnostic bytes.Buffer
		if code := runWithRunnerSecrets(append([]string{"--language=en"}, args...), &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}); code != 1 || out.Len() != 0 {
			t.Fatal("incomplete check mode reached effects", code)
		}
	}
	var out, diagnostic bytes.Buffer
	if code := runWithRunnerSecrets([]string{"--language=en", "--check-pool-connection", "--control=https://control.example.test", "--pool-config", filepath.Join(t.TempDir(), "private-missing212.json"), "--pool-identity=private-identity212"}, &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}); code != 1 || out.Len() != 0 || bytes.Contains(diagnostic.Bytes(), []byte("private-")) {
		t.Fatal("missing config reached secrets or leaked path", code)
	}
}
