package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestPoolConfigPreparationModeRejectsOtherFlagsBeforePrivateInput(t *testing.T) {
	for _, other := range []string{"--prepare-tools=x", "--control=http://127.0.0.1:4310", "--pool=x", "--pool-config=x", "--pool-identity=x", "--pair-pool-runner=x", "--project=x", "--identity=x", "--commands=x", "--allow-exec=false", "--maintenance-only=false", "--prepare-opencode-stdin=false", "--report-tools=false"} {
		input := &setupReader191{}
		var out, diagnostic bytes.Buffer
		code := runWithRunnerSecretsInput([]string{"--language=en", "--prepare-pool-config=x", other}, &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, input)
		if code != 1 || input.reads != 0 || out.Len() != 0 {
			t.Fatalf("Pool config preparation mixed flags reached private effects: %s exit=%d reads=%d", other, code, input.reads)
		}
	}
	input := &setupReader191{}
	var out, diagnostic bytes.Buffer
	if code := runWithRunnerSecretsInput([]string{"--language=en", "--prepare-pool-config", filepath.Join(t.TempDir(), "absent", "pool.json")}, &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, input); code != 1 || input.reads != 0 {
		t.Fatal("output guard did not precede private input", code, input.reads)
	}
	input = &setupReader191{}
	if code := runWithRunnerSecretsInput([]string{"--language=en", "--prepare-pool-config", "relative-pool.json"}, &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, input); code != 1 || input.reads != 0 {
		t.Fatal("relative output reached private input", code, input.reads)
	}
}
