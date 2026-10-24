// Package localization 提供无进程全局语言状态的本地化基础。
package localization

import (
	"strings"
	"unicode"

	"golang.org/x/text/language"
)

type Locale string

const (
	ZhCN          Locale = "zh-CN"
	EnUS          Locale = "en-US"
	DefaultLocale        = ZhCN
)

type Source string

const (
	SourceExplicit     Source = "explicit"
	SourceUser         Source = "user"
	SourceOrganization Source = "organization"
	SourceRequest      Source = "request"
	SourceDefault      Source = "default"
)

type Preferences struct{ Explicit, User, Organization, AcceptLanguage string }
type Selection struct {
	Locale Locale `json:"locale"`
	Source Source `json:"source"`
}

// Normalize 只归一到确实有目录的语言。x/text 处理 BCP 47 别名和脚本推断，
// 我们额外限制输入大小及控制字符；繁体中文绝不标记为已提供繁体翻译。
func Normalize(value string) (Locale, bool) {
	if len(value) > 128 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", false
	}
	value = strings.ReplaceAll(strings.TrimSpace(value), "_", "-")
	if value == "" {
		return "", false
	}
	tag, err := language.Parse(value)
	if err != nil {
		return "", false
	}
	base, _ := tag.Base()
	script, _ := tag.Script()
	switch base.String() {
	case "zh":
		if script.String() == "Hans" {
			return ZhCN, true
		}
	case "en":
		if script.String() == "Latn" {
			return EnUS, true
		}
	}
	return "", false
}

// Resolve 不读取全局变量或环境。组织默认仅在确实配置时生效，缺失时才继续
// 请求协商；显式选择和已保存的用户偏好始终可以覆盖组织的显示默认值。
func Resolve(p Preferences) Selection {
	for _, entry := range []struct {
		value  string
		source Source
	}{{p.Explicit, SourceExplicit}, {p.User, SourceUser}, {p.Organization, SourceOrganization}} {
		if locale, ok := Normalize(entry.value); ok {
			return Selection{locale, entry.source}
		}
	}
	if locale, ok := NegotiateAcceptLanguage(p.AcceptLanguage); ok {
		return Selection{locale, SourceRequest}
	}
	return Selection{DefaultLocale, SourceDefault}
}

// EnvironmentLocale 仅供 CLI 显式调用。它只读注入的查找器，避免服务器请求
// 意外继承宿主语言；最高优先的非空 POSIX 变量决定候选，C/POSIX 保持中性。
func EnvironmentLocale(lookup func(string) (string, bool)) (Locale, bool) {
	if lookup == nil {
		return "", false
	}
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		value, ok := lookup(name)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		if i := strings.IndexAny(value, ".@"); i >= 0 {
			value = value[:i]
		}
		return Normalize(value)
	}
	return "", false
}
