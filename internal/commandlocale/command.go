// Package commandlocale provides per-invocation display language for runtime commands.
// It does not translate external errors or change process environment variables.
package commandlocale

import (
	"flag"
	"fmt"
	"io"
	"slices"

	"github.com/ai-mada/suma/internal/localization"
)

type description struct{ zh, en string }

type Command struct {
	Locale                   localization.Locale
	Out, Stderr              io.Writer
	lookup                   func(string) (string, bool)
	flags                    *flag.FlagSet
	descriptions             map[string]description
	title                    description
	language                 *string
	help, shortHelp, version *bool
}

func New(name, zh, en string, out, stderr io.Writer, lookup func(string) (string, bool)) *Command {
	c := &Command{Locale: localization.DefaultLocale, Out: out, Stderr: stderr, lookup: lookup, flags: flag.NewFlagSet(name, flag.ContinueOnError), descriptions: map[string]description{}, title: description{zh, en}}
	c.flags.SetOutput(io.Discard)
	c.flags.Usage = func() {}
	if locale, ok := localization.EnvironmentLocale(lookup); ok {
		c.Locale = locale
	}
	if locale, ok := localization.Normalize(c.Getenv("SUMA_LANGUAGE")); ok {
		c.Locale = locale
	}
	c.language = c.String("language", "", "显示语言：zh-CN 或 en-US（支持别名）", "display language: zh-CN or en-US (aliases accepted)")
	c.help = c.Bool("help", false, "显示帮助后退出", "show help and exit")
	c.shortHelp = c.Bool("h", false, "显示帮助后退出", "show help and exit")
	c.version = c.Bool("version", false, "显示程序版本后退出", "show program version and exit")
	return c
}

func (c *Command) Getenv(name string) string {
	if c.lookup != nil {
		value, _ := c.lookup(name)
		return value
	}
	return ""
}
func (c *Command) Env(name, fallback string) string {
	if value := c.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func (c *Command) Text(zh, en string) string {
	if c.Locale == localization.EnUS {
		return en
	}
	return zh
}
func (c *Command) Failure(zh, en string) int { fmt.Fprintln(c.Stderr, c.Text(zh, en)); return 1 }
func (c *Command) String(name, value, zh, en string) *string {
	c.descriptions[name] = description{zh, en}
	return c.flags.String(name, value, en)
}
func (c *Command) Bool(name string, value bool, zh, en string) *bool {
	c.descriptions[name] = description{zh, en}
	return c.flags.Bool(name, value, en)
}

// FlagProvided distinguishes an explicit empty/false mode from an omitted flag.
// Values and diagnostics remain owned by the original parser.
func (c *Command) FlagProvided(name string) bool {
	provided := false
	c.flags.Visit(func(f *flag.Flag) {
		if f.Name == name {
			provided = true
		}
	})
	return provided
}

// OnlyProvidedFlags checks an explicit mode allowlist, including flags set to
// false or an empty string. Call it after Parse and before accessing resources.
func (c *Command) OnlyProvidedFlags(allowed ...string) bool {
	ok := true
	c.flags.Visit(func(f *flag.Flag) {
		if !slices.Contains(allowed, f.Name) {
			ok = false
		}
	})
	return ok
}

// Parse resolves language before emitting diagnostics. The standard parser owns
// value handling; help and version are ordinary flags so parsing remains read-only.
// Raw parser errors are never echoed, because they can contain secret values.
func (c *Command) Parse(args []string, version string) (stop bool, code int) {
	parseErr := c.flags.Parse(args)
	var languageSet bool
	c.flags.Visit(func(f *flag.Flag) {
		if f.Name == "language" {
			languageSet = true
		}
	})
	if languageSet {
		locale, ok := localization.Normalize(*c.language)
		if !ok {
			fmt.Fprintln(c.Stderr, c.Text("--language 必须为 zh-CN 或 en-US，也支持已声明的语言别名", "--language must be zh-CN or en-US (supported aliases are accepted)"))
			return true, 2
		}
		c.Locale = locale
	}
	if parseErr != nil || c.flags.NArg() != 0 {
		fmt.Fprintln(c.Stderr, c.Text("命令参数无效，请查看 --help", "invalid command flags; use --help"))
		return true, 2
	}
	if *c.help || *c.shortHelp {
		c.Usage()
		return true, 0
	}
	if *c.version {
		fmt.Fprintf(c.Out, "%s %s\n", c.flags.Name(), version)
		return true, 0
	}
	return false, 0
}

func (c *Command) Usage() {
	fmt.Fprintf(c.Out, "%s — %s\n\n%s: %s %s\n", c.flags.Name(), c.Text(c.title.zh, c.title.en), c.Text("用法", "Usage"), c.flags.Name(), c.Text("[参数]", "[options]"))
	c.flags.VisitAll(func(f *flag.Flag) {
		d := c.descriptions[f.Name]
		value := c.Text(" 值", " VALUE")
		if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
			value = ""
		}
		fmt.Fprintf(c.Out, "  --%s%s\n      %s\n", f.Name, value, c.Text(d.zh, d.en))
	})
	fmt.Fprintln(c.Out, c.Text("语言优先级：--language > SUMA_LANGUAGE > LC_ALL/LC_MESSAGES/LANG > zh-CN", "Language: --language > SUMA_LANGUAGE > LC_ALL/LC_MESSAGES/LANG > zh-CN"))
	fmt.Fprintln(c.Out, c.Text("退出码：0 成功，1 运行失败，2 用法错误。", "Exit codes: 0 success, 1 runtime failure, 2 usage error."))
}
