package toolcontract

import (
	"encoding/json"
	"unicode/utf8"
)

// ValidJSONUnicode rejects escaped, unpaired UTF-16 surrogates before Go's JSON
// decoder can replace them with U+FFFD. Call it only for selected wire fields;
// ignored vendor presentation must not affect execution facts.
func ValidJSONUnicode(raw []byte) bool {
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return false
	}
	hex := func(value []byte) uint16 {
		var result uint16
		for _, b := range value {
			result <<= 4
			switch {
			case b >= '0' && b <= '9':
				result += uint16(b - '0')
			case b >= 'a' && b <= 'f':
				result += uint16(b - 'a' + 10)
			case b >= 'A' && b <= 'F':
				result += uint16(b - 'A' + 10)
			}
		}
		return result
	}
	inString := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || raw[i] != '\\' {
			continue
		}
		i++ // JSON syntax already proves an escape and its payload are present.
		if raw[i] != 'u' {
			continue
		}
		unit := hex(raw[i+1 : i+5])
		i += 4
		if unit >= 0xdc00 && unit <= 0xdfff {
			return false
		}
		if unit < 0xd800 || unit > 0xdbff {
			continue
		}
		if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
			return false
		}
		low := hex(raw[i+3 : i+7])
		if low < 0xdc00 || low > 0xdfff {
			return false
		}
		i += 6
	}
	return true
}
