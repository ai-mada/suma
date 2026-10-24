package commandlocale

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/localization"
)

func lookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) { value, ok := values[key]; return value, ok }
}

func TestLanguagePrecedenceAliasesAndPerInvocationIsolation(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		env  map[string]string
		want localization.Locale
	}{
		{"default", nil, nil, localization.ZhCN},
		{"explicit", []string{"--language=zh_Hans"}, map[string]string{"SUMA_LANGUAGE": "en-US", "LC_ALL": "en_US.UTF-8"}, localization.ZhCN},
		{"application", nil, map[string]string{"SUMA_LANGUAGE": "en_GB", "LC_ALL": "zh_CN.UTF-8"}, localization.EnUS},
		{"all", nil, map[string]string{"LC_ALL": "en_US.UTF-8", "LC_MESSAGES": "zh_CN.UTF-8"}, localization.EnUS},
		{"messages", nil, map[string]string{"LC_MESSAGES": "en_US.UTF-8", "LANG": "zh_CN.UTF-8"}, localization.EnUS},
		{"lang", nil, map[string]string{"LANG": "en_AU.UTF-8"}, localization.EnUS},
		{"neutral", nil, map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"}, localization.ZhCN},
		{"unsupported environment", nil, map[string]string{"SUMA_LANGUAGE": "fr-FR", "LANG": "en_US.UTF-8"}, localization.EnUS},
		{"invalid environment", nil, map[string]string{"SUMA_LANGUAGE": "private\nsecret", "LC_ALL": "zh_TW.UTF-8"}, localization.ZhCN},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			c := New("test-command", "测试程序", "test program", &out, &stderr, lookup(tc.env))
			if stop, code := c.Parse(tc.args, "dev"); stop || code != 0 || c.Locale != tc.want {
				t.Fatalf("stop=%v code=%d locale=%s stderr=%q", stop, code, c.Locale, stderr.String())
			}
		})
	}
}

func TestParserRejectsInvalidFlagsAndNeverEchoesPrivateValues(t *testing.T) {
	for _, args := range [][]string{
		{"--language="}, {"--language"}, {"--language=zh-TW"}, {"--language=fr-FR"}, {"--language=private\nsecret"}, {"--language=" + strings.Repeat("x", 129)},
		{"--unknown=private-secret"}, {"--enabled=private-secret"}, {"--path"}, {"private-secret"}, {"--", "private-secret"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			c := New("test-command", "测试程序", "test program", &out, &stderr, lookup(map[string]string{"SUMA_LANGUAGE": "en-US"}))
			c.Bool("enabled", false, "启用", "enable")
			c.String("path", "", "路径", "path")
			stop, code := c.Parse(args, "dev")
			if !stop || code != 2 || out.Len() != 0 || !strings.Contains(stderr.String(), "--") || strings.Contains(stderr.String(), "private") {
				t.Fatalf("stop=%v code=%d stdout=%q stderr=%q", stop, code, out.String(), stderr.String())
			}
		})
	}
}

func TestHelpUsesSelectedLanguageWithoutPrivateDefaults(t *testing.T) {
	for _, args := range [][]string{{"--help", "--language=en"}, {"--language", "en-GB", "-h"}} {
		var out, stderr bytes.Buffer
		c := New("test-command", "测试程序", "test program", &out, &stderr, nil)
		c.String("path", "private-default", "路径", "path")
		stop, code := c.Parse(args, "dev")
		if !stop || code != 0 || !strings.Contains(out.String(), "Language:") || strings.Contains(out.String(), "private-default") || stderr.Len() != 0 {
			t.Fatalf("stop=%v code=%d stdout=%q stderr=%q", stop, code, out.String(), stderr.String())
		}
	}
}

func TestStringValuesAreNotMistakenForLanguageFlags(t *testing.T) {
	var out, stderr bytes.Buffer
	c := New("test-command", "测试程序", "test program", &out, &stderr, nil)
	path := c.String("path", "", "路径", "path")
	stop, code := c.Parse([]string{"--path", "--language=en", "--help"}, "dev")
	if !stop || code != 0 || *path != "--language=en" || !strings.Contains(out.String(), "语言优先级") {
		t.Fatalf("stop=%v code=%d path=%q stdout=%q stderr=%q", stop, code, *path, out.String(), stderr.String())
	}
}

func TestVersionIsStableAcrossLanguages(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		var out, stderr bytes.Buffer
		c := New("test-command", "测试程序", "test program", &out, &stderr, nil)
		stop, code := c.Parse([]string{"--version", "--language", lang}, "1.2.3")
		if !stop || code != 0 || out.String() != "test-command 1.2.3\n" || stderr.Len() != 0 {
			t.Fatalf("stop=%v code=%d stdout=%q stderr=%q", stop, code, out.String(), stderr.String())
		}
	}
}

func TestChineseHelpLocalizesHumanPlaceholders(t *testing.T) {
	var out, stderr bytes.Buffer
	c := New("test-command", "测试程序", "test program", &out, &stderr, nil)
	stop, code := c.Parse([]string{"--help"}, "dev")
	if !stop || code != 0 || !strings.Contains(out.String(), "[参数]") || !strings.Contains(out.String(), "--language 值") || strings.Contains(out.String(), "[options]") || strings.Contains(out.String(), "VALUE") {
		t.Fatalf("stop=%v code=%d stdout=%q stderr=%q", stop, code, out.String(), stderr.String())
	}
}
