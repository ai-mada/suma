// Package strictjson provides bounded, closed JSON decoding for wire contracts.
// It uses encoding/json while rejecting its permissive duplicate-name and
// unpaired-surrogate behavior before typed decoding can lose that evidence.
package strictjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

func validUnicode(raw []byte) bool {
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return false
	}
	hex := func(v []byte) uint16 {
		var n uint16
		for _, c := range v {
			n <<= 4
			switch {
			case c >= '0' && c <= '9':
				n += uint16(c - '0')
			case c >= 'a' && c <= 'f':
				n += uint16(c - 'a' + 10)
			case c >= 'A' && c <= 'F':
				n += uint16(c - 'A' + 10)
			}
		}
		return n
	}
	inside := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			inside = !inside
			continue
		}
		if !inside || raw[i] != '\\' {
			continue
		}
		i++
		if raw[i] != 'u' {
			continue
		}
		n := hex(raw[i+1 : i+5])
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return false
		}
		if n < 0xd800 || n > 0xdbff {
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

func fields(t reflect.Type, out map[string]reflect.StructField) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if f.Anonymous && tag == "" {
			nested := f.Type
			if nested.Kind() == reflect.Pointer {
				nested = nested.Elem()
			}
			if nested.Kind() == reflect.Struct {
				fields(nested, out)
				continue
			}
		}
		name := strings.Split(tag, ",")[0]
		if name != "-" && f.IsExported() {
			if name == "" {
				name = f.Name
			}
			out[name] = f
		}
	}
}

// Presence is checked recursively, including explicitly false booleans. An
// optional property may be omitted, but explicit null is never a valid grant.
func presence(raw []byte, t reflect.Type) error {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("null contract value")
	}
	switch t.Kind() {
	case reflect.Struct:
		var values map[string]json.RawMessage
		if json.Unmarshal(raw, &values) != nil || values == nil {
			return fmt.Errorf("contract object required")
		}
		declared := map[string]reflect.StructField{}
		fields(t, declared)
		for name, f := range declared {
			value, ok := values[name]
			if !ok {
				if !strings.Contains(f.Tag.Get("json"), ",omitempty") {
					return fmt.Errorf("required contract property omitted")
				}
				continue
			}
			if err := presence(value, f.Type); err != nil {
				return err
			}
		}
		for name := range values {
			if _, ok := declared[name]; !ok {
				return fmt.Errorf("unknown contract property")
			}
		}
	case reflect.Slice, reflect.Array:
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil || values == nil {
			return fmt.Errorf("contract array required")
		}
		for _, value := range values {
			if err := presence(value, t.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

func Decode(raw []byte, out any, maximum int) error {
	t := reflect.TypeOf(out)
	if maximum <= 0 || len(raw) > maximum || t == nil || t.Kind() != reflect.Pointer || !validUnicode(raw) {
		return fmt.Errorf("invalid bounded contract JSON")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	count := 0
	var walk func(int) error
	walk = func(depth int) error {
		count++
		if depth > 24 || count > 262144 {
			return fmt.Errorf("contract JSON nesting or token limit")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := token.(json.Delim); ok {
			if delimiter != '{' && delimiter != '[' {
				return fmt.Errorf("invalid contract JSON delimiter")
			}
			seen := map[string]bool{}
			for d.More() {
				if delimiter == '{' {
					key, err := d.Token()
					if err != nil {
						return err
					}
					name, ok := key.(string)
					if !ok || seen[name] {
						return fmt.Errorf("duplicate contract JSON property")
					}
					seen[name] = true
				}
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = d.Token()
		}
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("trailing contract JSON")
	}
	if err := presence(raw, t.Elem()); err != nil {
		return err
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(out)
}
