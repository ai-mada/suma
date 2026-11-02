package main

import (
	"bytes"
	"testing"
)

func TestInstalledDiscoveryContinuousModeRejectsMixedEffectsBeforePrivateInputs(t *testing.T) {
	for _, args := range [][]string{
		{"--maintenance-only"},
		{"--allow-installed-tool-discovery", "--report-tools"},
		{"--maintenance-only", "--allow-installed-tool-discovery", "--report-tools=false"},
		{"--allow-installed-tool-discovery=false", "--prepare-sdk-stdin", "--prepare-tools=never.json"},
		{"--allow-installed-tool-discovery", "--discover-installed-tools"},
		{"--allow-installed-tool-discovery", "--pair-runner=never.json"},
	} {
		input := &setupReader191{}
		var out, stderr bytes.Buffer
		lookup := func(name string) (string, bool) {
			if name == "SUMA_RUNNER_TOKEN" || name == "SUMA_POOL_RUNNER_TOKEN" {
				t.Fatal("mixed maintenance mode read a machine credential")
			}
			return "", false
		}
		code := runWithRunnerSecretsInput(append([]string{"--language=en"}, args...), &out, &stderr, lookup, setupSecrets191{t}, input)
		if code != 1 || input.reads != 0 || out.Len() != 0 {
			t.Fatalf("invalid maintenance mode reached private input/effects: %v, exit %d reads %d", args, code, input.reads)
		}
	}
}
