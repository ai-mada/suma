package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCommandProcessHelper(t *testing.T) {
	if os.Getenv("SUMA_COMMAND_TEST_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"suma-control"}, os.Args[i+1:]...)
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
			main()
			os.Exit(0)
		}
	}
	os.Exit(97)
}

func commandProcess(t *testing.T, vars map[string]string, args ...string) (int, string, string, string) {
	t.Helper()
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestCommandProcessHelper$", "--"}, args...)...)
	cmd.Dir = root
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "SUMA_") && key != "LC_ALL" && key != "LC_MESSAGES" && key != "LANG" {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "SUMA_COMMAND_TEST_HELPER=1", "SUMA_DATA_DIR="+filepath.Join(root, "data"))
	for key, value := range vars {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			code = e.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	if ctx.Err() != nil {
		t.Fatal("command did not exit before timeout")
	}
	return code, out.String(), stderr.String(), root
}

func TestDefaultHelpIsChineseAndReadOnly(t *testing.T) {
	code, out, stderr, root := commandProcess(t, nil, "--help")
	if code != 0 || !strings.Contains(out+stderr, "语言") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "data")); !os.IsNotExist(err) {
		t.Fatalf("help created data: %v", err)
	}
}

func TestReadOnlyFlagsResolveLanguageBeforeAnyStartup(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		env  map[string]string
		code int
		want string
	}{
		{"English alias", []string{"--help", "--language=en_GB"}, nil, 0, "Language:"},
		{"environment", []string{"-h"}, map[string]string{"SUMA_LANGUAGE": "en-US"}, 0, "Language:"},
		{"POSIX", []string{"--help"}, map[string]string{"LC_ALL": "en_US.UTF-8"}, 0, "Language:"},
		{"explicit wins", []string{"--language=zh", "--help"}, map[string]string{"SUMA_LANGUAGE": "en-US"}, 0, "语言优先级"},
		{"version", []string{"--language=en", "--version"}, nil, 0, "suma-control 1.0.0\n"},
		{"empty", []string{"--language=", "--help"}, nil, 2, "--language"},
		{"unsupported", []string{"--language=zh-TW", "--version"}, nil, 2, "--language"},
		{"private flag", []string{"--language=en", "--unknown=private-secret"}, nil, 2, "invalid command flags"},
		{"missing flag value", []string{"--language=en", "--addr"}, nil, 2, "invalid command flags"},
		{"private positional", []string{"--language=en", "private-secret"}, nil, 2, "invalid command flags"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out, stderr, root := commandProcess(t, tc.env, tc.args...)
			if code != tc.code || !strings.Contains(out+stderr, tc.want) || strings.Contains(out+stderr, "private-secret") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatalf("read-only path changed directory: %v %v", entries, err)
			}
		})
	}
}

func TestHelpDoesNotReadStartupSecretsOrPrintEnvironmentAddress(t *testing.T) {
	var out, stderr bytes.Buffer
	lookup := func(key string) (string, bool) {
		if key == "SUMA_ADDR" {
			return "private-address-value", true
		}
		if strings.Contains(key, "TOKEN") || strings.Contains(key, "KEY") || key == "SUMA_DB_DSN" || key == "SUMA_DATA_DIR" {
			t.Fatalf("help read startup configuration %s", key)
		}
		return "", false
	}
	if code := run([]string{"--help"}, &out, &stderr, lookup); code != 0 || strings.Contains(out.String(), "private-address-value") || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
}

func TestStartupGuardsRemainLocalizedAndDoNotLeakValues(t *testing.T) {
	for _, tc := range []struct {
		name        string
		args        []string
		env         map[string]string
		want, event string
	}{
		{"external", []string{"--language=en", "--addr=0.0.0.0:4310"}, nil, "external binding requires SUMA_SETUP_TOKEN", "control.external_binding_guard"},
		{"external Chinese", []string{"--language=zh", "--addr=0.0.0.0:4310"}, nil, "外部监听需要 SUMA_SETUP_TOKEN", "control.external_binding_guard"},
		{"invalid address", []string{"--language=en", "--addr=private-secret"}, nil, "invalid address", "control.address_invalid"},
		{"database", []string{"--language=en"}, map[string]string{"SUMA_DB_DRIVER": "postgres"}, "SUMA_DB_DSN required", "control.database_dsn_required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out, stderr, _ := commandProcess(t, tc.env, tc.args...)
			if code != 1 || out != "" || !strings.Contains(stderr, tc.want) || !strings.Contains(stderr, "event="+tc.event) || strings.Contains(stderr, "private-secret") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
			}
		})
	}
}
