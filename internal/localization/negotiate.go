package localization

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/language"
)

var qualitySyntax = regexp.MustCompile(`^(?:0(?:\.[0-9]{0,3})?|1(?:\.0{0,3})?)$`)

type languageRange struct {
	locale                         Locale
	wildcard                       bool
	quality, specificity, position int
}

// NegotiateAcceptLanguage 使用有界的语言优先列表；BCP 47 交由成熟的
// x/text 解析。权重转为 0..1000 整数，避免 NaN、负数和浮点比较歧义。
// 同一语言最具体的范围优先，明确 q=0 不会被通配符重新选中。
func NegotiateAcceptLanguage(value string) (Locale, bool) {
	if len(value) > 8192 || strings.TrimSpace(value) == "" || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", false
	}
	parts := strings.Split(value, ",")
	if len(parts) > 64 {
		return "", false
	}
	ranges := make([]languageRange, 0, len(parts))
	seen := map[string]bool{}
	for position, part := range parts {
		fields := strings.Split(strings.TrimSpace(part), ";")
		if len(fields) > 2 {
			return "", false
		}
		tag := strings.TrimSpace(fields[0])
		canonical := strings.ToLower(strings.ReplaceAll(tag, "_", "-"))
		if tag == "" || seen[canonical] {
			return "", false
		}
		seen[canonical] = true
		quality := 1000
		if len(fields) == 2 {
			q := strings.Split(strings.TrimSpace(fields[1]), "=")
			if len(q) != 2 || !strings.EqualFold(strings.TrimSpace(q[0]), "q") {
				return "", false
			}
			weight := strings.TrimSpace(q[1])
			if !qualitySyntax.MatchString(weight) {
				return "", false
			}
			digits := strings.SplitN(weight, ".", 2)
			if digits[0] == "0" {
				quality = 0
				if len(digits) == 2 && digits[1] != "" {
					quality, _ = strconv.Atoi(digits[1] + strings.Repeat("0", 3-len(digits[1])))
				}
			}
		}
		if tag == "*" {
			ranges = append(ranges, languageRange{wildcard: true, quality: quality, position: position})
			continue
		}
		if _, err := language.Parse(strings.ReplaceAll(tag, "_", "-")); err != nil {
			return "", false
		}
		if locale, ok := Normalize(tag); ok {
			ranges = append(ranges, languageRange{locale: locale, quality: quality, specificity: strings.Count(canonical, "-") + 1, position: position})
		}
	}
	bestQuality, bestPosition := -1, 65
	var result Locale
	for _, locale := range []Locale{ZhCN, EnUS} {
		selected := languageRange{quality: -1, specificity: -1, position: 65}
		for _, candidate := range ranges {
			if !candidate.wildcard && candidate.locale != locale {
				continue
			}
			if candidate.specificity > selected.specificity || candidate.specificity == selected.specificity && (candidate.quality > selected.quality || candidate.quality == selected.quality && candidate.position < selected.position) {
				selected = candidate
			}
		}
		if selected.quality > 0 && (selected.quality > bestQuality || selected.quality == bestQuality && selected.position < bestPosition) {
			result, bestQuality, bestPosition = locale, selected.quality, selected.position
		}
	}
	return result, result != ""
}
