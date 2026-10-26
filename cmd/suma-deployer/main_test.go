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
			os.Args = append([]string{"suma-deployer"}, os.Args[i+1:]...)
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
	cmd.Env = append(cmd.Env, "SUMA_COMMAND_TEST_HELPER=1")
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
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("help changed workspace: %v %v", entries, err)
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
		{"English alias", []string{"--config=private-secret", "--help", "--language=en_GB"}, nil, 0, "Language:"},
		{"environment", []string{"-h"}, map[string]string{"SUMA_LANGUAGE": "en-US"}, 0, "Language:"},
		{"POSIX", []string{"--help"}, map[string]string{"LC_MESSAGES": "en_US.UTF-8"}, 0, "Language:"},
		{"explicit wins", []string{"--language=zh", "--help"}, map[string]string{"SUMA_LANGUAGE": "en-US"}, 0, "语言优先级"},
		{"version", []string{"--config=private-secret", "--language=en", "--version"}, nil, 0, "suma-deployer 1.0.0\n"},
		{"empty", []string{"--language=", "--help"}, nil, 2, "--language"},
		{"unsupported", []string{"--language=zh-TW", "--version"}, nil, 2, "--language"},
		{"private flag", []string{"--language=en", "--unknown=private-secret"}, nil, 2, "invalid command flags"},
		{"missing flag value", []string{"--language=en", "--config"}, nil, 2, "invalid command flags"},
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

func TestMissingAndUnreadableConfigurationAreLocalized(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--language=en"}, "--config is required"},
		{[]string{"--language=zh"}, "必须提供 --config"},
		{[]string{"--language=en", "--config=private-secret"}, "provider configuration unavailable"},
		{[]string{"--language=zh", "--config=private-secret"}, "提供方配置不可用"},
	} {
		code, out, stderr, _ := commandProcess(t, nil, tc.args...)
		if code != 1 || out != "" || !strings.Contains(stderr, tc.want) || strings.Contains(stderr, "private-secret") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
		}
	}
}

func TestInvalidConfigurationAndExternalManagementAreRejectedBeforeListening(t *testing.T) {
	for _, tc := range []struct{ raw, lang, want string }{
		{`{"unknown":"private-secret"}`, "en", "invalid provider configuration"},
		{`{"management_address":"0.0.0.0:4350","root":"private-secret"}`, "en", "management_address must be an explicit loopback address"},
		{`{"management_address":"0.0.0.0:4350","root":"private-secret"}`, "zh", "management_address 必须是明确的回环地址"},
	} {
		file := filepath.Join(t.TempDir(), "provider.json")
		if err := os.WriteFile(file, []byte(tc.raw), 0600); err != nil {
			t.Fatal(err)
		}
		code, out, stderr, root := commandProcess(t, nil, "--language="+tc.lang, "--config="+file)
		if code != 1 || out != "" || !strings.Contains(stderr, tc.want) || strings.Contains(stderr, "private-secret") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("guard changed workspace: %v %v", entries, err)
		}
	}
}

func TestHelpDoesNotReadCredentials(t *testing.T) {
	var out, stderr bytes.Buffer
	lookup := func(key string) (string, bool) {
		if strings.Contains(key, "TOKEN") || strings.Contains(key, "KEY") {
			t.Fatalf("help read credential %s", key)
		}
		return "", false
	}
	if code := run([]string{"--config=private-secret", "--help"}, &out, &stderr, lookup); code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}
