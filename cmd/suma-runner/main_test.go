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
			os.Args = append([]string{"suma-runner"}, os.Args[i+1:]...)
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
		{"English alias", []string{"--commands=private-secret", "--help", "--language=en_GB"}, nil, 0, "Language:"},
		{"environment", []string{"-h"}, map[string]string{"SUMA_LANGUAGE": "en-US"}, 0, "Language:"},
		{"POSIX", []string{"--help"}, map[string]string{"LANG": "en_US.UTF-8"}, 0, "Language:"},
		{"explicit wins", []string{"--language=zh", "--help"}, map[string]string{"SUMA_LANGUAGE": "en-US"}, 0, "语言优先级"},
		{"version", []string{"--pool-config=private-secret", "--language=en", "--version"}, nil, 0, "suma-runner 1.0.0\n"},
		{"empty", []string{"--language=", "--help"}, nil, 2, "--language"},
		{"unsupported", []string{"--language=zh-TW", "--version"}, nil, 2, "--language"},
		{"private flag", []string{"--language=en", "--unknown=private-secret"}, nil, 2, "invalid command flags"},
		{"invalid boolean", []string{"--language=en", "--allow-exec=private-secret"}, nil, 2, "invalid command flags"},
		{"missing flag value", []string{"--language=en", "--project"}, nil, 2, "invalid command flags"},
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

func TestMissingCredentialsAndPoolExclusionRemainLocalized(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--language=en"}, "--project, --worker and SUMA_RUNNER_TOKEN are required"},
		{[]string{"--language=zh"}, "必须提供 --project、--worker 和 SUMA_RUNNER_TOKEN"},
		{[]string{"--language=en", "--pool-config=private-secret", "--project=private-secret"}, "pool mode is independent"},
		{[]string{"--language=zh", "--pool-config=private-secret", "--worker=private-secret"}, "资源池模式不能"},
		{[]string{"--language=en", "--pool-config=private-secret"}, "pool local configuration unavailable"},
		{[]string{"--language=zh", "--pool-config=private-secret"}, "资源池本地配置不可用"},
	} {
		code, out, stderr, root := commandProcess(t, nil, tc.args...)
		if code != 1 || out != "" || !strings.Contains(stderr, tc.want) || strings.Contains(stderr, "private-secret") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("guard created durable state: %v %v", entries, err)
		}
	}
}

func TestDescribeToolsMachineJSONIsLanguageIndependent(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		code, out, stderr, root := commandProcess(t, nil, "--language="+lang, "--describe-tools")
		if code != 0 || out != "{\"execution_enabled\":false,\"items\":[]}\n" || stderr != "" {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("describe created durable state: %v %v", entries, err)
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
	if code := run([]string{"--commands=private-secret", "--pool-config=private-secret", "--help"}, &out, &stderr, lookup); code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestExternalOSDiagnosticRetainsOriginalText(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-commands.json")
	_, err := os.ReadFile(missing)
	if err == nil {
		t.Fatal("fixture unexpectedly exists")
	}
	for _, lang := range []string{"en", "zh"} {
		code, out, stderr, _ := commandProcess(t, nil, "--language="+lang, "--commands="+missing)
		if code != 1 || out != "" || strings.TrimSpace(stderr) != err.Error() {
			t.Fatalf("code=%d stdout=%q stderr=%q want=%q", code, out, stderr, err.Error())
		}
	}
}

func TestPoolDescribeRetainsMachineIdentityAndDoesNotReadCredentials(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pool.json")
	raw := `{"schema_version":1,"pool_id":"pool-stable","runner_id":"runner-stable","slots":1,"scopes":[{"tenant_id":"tenant-stable","project_id":"project-stable","workspace":".","commands":{}}]}`
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	var baseline string
	for _, lang := range []string{"en", "zh"} {
		var out, stderr bytes.Buffer
		lookup := func(key string) (string, bool) {
			if strings.Contains(key, "TOKEN") || strings.Contains(key, "KEY") {
				t.Fatalf("pool describe read credential %s", key)
			}
			return "", false
		}
		code := run([]string{"--language=" + lang, "--pool-config=" + file, "--describe-tools"}, &out, &stderr, lookup)
		if code != 0 || stderr.Len() != 0 || !strings.Contains(out.String(), `"pool_id":"pool-stable"`) || !strings.Contains(out.String(), `"runner_id":"runner-stable"`) || !strings.Contains(out.String(), `"project_id":"project-stable"`) {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
		}
		if baseline == "" {
			baseline = out.String()
		} else if out.String() != baseline {
			t.Fatalf("locale changed pool machine JSON: %q != %q", out.String(), baseline)
		}
	}
	for _, tc := range []struct{ lang, want string }{{"en", "pool mode requires SUMA_POOL_RUNNER_TOKEN"}, {"zh", "资源池模式需要 SUMA_POOL_RUNNER_TOKEN"}} {
		code, out, stderr, root := commandProcess(t, nil, "--language="+tc.lang, "--pool-config="+file)
		if code != 1 || out != "" || !strings.Contains(stderr, tc.want) {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("missing credential created state: %v %v", entries, err)
		}
	}
}

func TestPoolProjectCredentialExclusionIsPreserved(t *testing.T) {
	code, out, stderr, root := commandProcess(t, map[string]string{"SUMA_RUNNER_TOKEN": "private-secret"}, "--language=en", "--pool-config=missing.json")
	if code != 1 || out != "" || !strings.Contains(stderr, "pool mode is independent") || strings.Contains(stderr, "private-secret") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("credential exclusion created state: %v %v", entries, err)
	}
}
