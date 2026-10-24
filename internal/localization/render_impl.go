package localization

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"
)

const maxSafeInteger = uint64(1<<53 - 1)

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,95}$`)
var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,100}$`)
var integerPattern = regexp.MustCompile(`^(?:0|[1-9][0-9]*)$`)
var placeholderPattern = regexp.MustCompile(`\{\{\.([a-z][a-z0-9_]*)\}\}`)

// Format 只把经过类型验证的显示参数放入静态消息模板。参数永远不作为模板
// 解析；错误不回显可能包含秘密的原值。每次调用明确传入语言，无共享切换状态。
func Format(locale Locale, key string, params Params) (Rendered, error) {
	locale = resolvedLocale(locale)
	out := Rendered{Key: key, Locale: locale}
	if !keyPattern.MatchString(key) {
		out.Fallback, out.Text = "invalid_key", fallbackText(locale, "")
		return out, ErrInvalidKey
	}
	definition, ok := definitions[key]
	if !ok {
		out.Fallback, out.Text = "unknown_key", fallbackText(locale, key)
		return out, nil
	}
	checked, err := validateParameters(definition.Parameters, params)
	if err != nil {
		out.Fallback, out.Text = "invalid_parameters", fallbackText(locale, key)
		return out, err
	}
	var b bytes.Buffer
	if err := templates[key][locale].Execute(&b, checked); err != nil {
		out.Fallback, out.Text = "template_error", fallbackText(locale, key)
		return out, errors.New("localization template failed")
	}
	out.Text, out.Params, out.Localized = b.String(), checked, true
	return out, nil
}

// WithSource 用于渐进迁移旧错误。未覆盖的具体诊断保留原文，不能拿泛化错误
// 掩盖原本可用的校验原因；source 必须是调用方原已公开的文案。
func WithSource(locale Locale, key string, params Params, source string) (Rendered, error) {
	out, err := Format(locale, key, params)
	if source != "" && !out.Localized {
		out.Text, out.Params, out.Fallback = source, nil, "source_untranslated"
	}
	return out, err
}

// Catalog 深复制目录以支持生成文档和覆盖检查，调用方不能修改共享模板。
func Catalog() []Definition {
	out := make([]Definition, 0, len(catalog))
	for _, d := range catalog {
		out = append(out, Definition{Key: d.Key, Parameters: append([]Parameter{}, d.Parameters...), Texts: map[Locale]string{ZhCN: d.Texts[ZhCN], EnUS: d.Texts[EnUS]}, SourceTemplate: d.SourceTemplate})
	}
	return out
}

// LookupSource 只匹配维护过的完整原文，不做前后缀、模糊或用户内容推断。
func LookupSource(source string) (string, bool) { key, ok := sources[source]; return key, ok }

// LookupCode 仅用于没有具体原文的错误，HTTP 适配层应优先匹配完整原文。
func LookupCode(code string) (string, bool) { key, ok := codes[code]; return key, ok }

func resolvedLocale(locale Locale) Locale {
	if normalized, ok := Normalize(string(locale)); ok {
		return normalized
	}
	return DefaultLocale
}
func fallbackText(locale Locale, key string) string {
	text := "消息暂时无法显示"
	if locale == EnUS {
		text = "This message is not available in this language."
	}
	if key != "" {
		text += " (" + key + ")"
	}
	return text
}
func validateParameters(spec []Parameter, params Params) (Params, error) {
	if len(spec) != len(params) {
		return nil, ErrInvalidParameters
	}
	out := Params{}
	for _, p := range spec {
		v, exists := params[p.Name]
		if !exists || v == nil {
			return nil, ErrInvalidParameters
		}
		switch p.Kind {
		case Text, Identifier:
			s, ok := v.(string)
			if !ok || !utf8.ValidString(s) || len(s) == 0 || len(s) > 512 || strings.IndexFunc(s, unicode.IsControl) >= 0 || p.Kind == Identifier && !identifierPattern.MatchString(s) {
				return nil, ErrInvalidParameters
			}
			out[p.Name] = s
		case Integer:
			n, ok := safeInteger(v)
			if !ok {
				return nil, ErrInvalidParameters
			}
			out[p.Name] = int64(n)
		default:
			return nil, ErrInvalidParameters
		}
	}
	return out, nil
}
func safeInteger(v any) (uint64, bool) {
	var raw string
	switch n := v.(type) {
	case int:
		raw = strconv.FormatInt(int64(n), 10)
	case int8:
		raw = strconv.FormatInt(int64(n), 10)
	case int16:
		raw = strconv.FormatInt(int64(n), 10)
	case int32:
		raw = strconv.FormatInt(int64(n), 10)
	case int64:
		raw = strconv.FormatInt(n, 10)
	case uint:
		raw = strconv.FormatUint(uint64(n), 10)
	case uint8:
		raw = strconv.FormatUint(uint64(n), 10)
	case uint16:
		raw = strconv.FormatUint(uint64(n), 10)
	case uint32:
		raw = strconv.FormatUint(uint64(n), 10)
	case uint64:
		raw = strconv.FormatUint(n, 10)
	case json.Number:
		raw = n.String()
	default:
		return 0, false
	}
	if len(raw) > 16 || !integerPattern.MatchString(raw) {
		return 0, false
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	return n, err == nil && n <= maxSafeInteger
}

var definitions = map[string]Definition{}
var templates = map[string]map[Locale]*template.Template{}
var sourceTemplates = map[string]*template.Template{}
var sources = map[string]string{}

func init() {
	for _, d := range catalog {
		if !keyPattern.MatchString(d.Key) || definitions[d.Key].Key != "" {
			panic("invalid or duplicate localization key")
		}
		definitions[d.Key] = d
		source := d.SourceTemplate
		if source == "" {
			source = d.Texts[ZhCN]
		}
		sourceTemplates[d.Key] = compileTemplate(d, source)
		templates[d.Key] = map[Locale]*template.Template{}
		for _, locale := range []Locale{ZhCN, EnUS} {
			if d.Texts[locale] == "" {
				panic("incomplete localization catalog")
			}
			// 对外发布的目录只支持 {{.name}} 纯文本替换。即使底层使用成熟的
			// text/template，也不允许目录逐渐引入函数、条件或任意模板操作。
			templates[d.Key][locale] = compileTemplate(d, d.Texts[locale])
		}
		if len(d.Parameters) == 0 {
			if prior, exists := sources[d.Texts[ZhCN]]; exists && prior != d.Key {
				panic("ambiguous source localization")
			}
			sources[d.Texts[ZhCN]] = d.Key
		}
	}
	for source, key := range sourceAliases {
		if definitions[key].Key == "" {
			panic("source alias references unknown key")
		}
		sources[source] = key
	}
}

// Canonical source and display translations share the same deliberately small
// grammar: plain text and declared named parameters, without executable logic.
func compileTemplate(d Definition, source string) *template.Template {
	remaining := placeholderPattern.ReplaceAllString(source, "")
	if strings.Contains(remaining, "{{") || strings.Contains(remaining, "}}") {
		panic("unsupported localization template syntax")
	}
	for _, match := range placeholderPattern.FindAllStringSubmatch(source, -1) {
		known := false
		for _, p := range d.Parameters {
			if p.Name == match[1] {
				known = true
				break
			}
		}
		if !known {
			panic("undeclared localization template parameter")
		}
	}
	return template.Must(template.New(d.Key).Option("missingkey=error").Parse(source))
}
