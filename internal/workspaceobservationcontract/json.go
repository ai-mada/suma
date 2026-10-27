package workspaceobservationcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/ai-mada/suma/internal/toolcontract"
)

// Decode is closed and case-sensitive. encoding/json alone would accept case
// aliases, null counters and duplicate keys before validating the result.
func Decode(raw []byte, out any) error {
	bad := func() error { return fmt.Errorf("invalid workspace observation JSON") }
	if len(raw) > 2<<20 || !toolcontract.ValidJSONUnicode(raw) {
		return bad()
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	values := 0
	var walk func(int) error
	walk = func(depth int) error {
		values++
		if depth > 64 || values > 65536 {
			return bad()
		}
		t, err := d.Token()
		if err != nil {
			return bad()
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				token, e := d.Token()
				key, valid := token.(string)
				if e != nil || !valid || seen[key] {
					return bad()
				}
				seen[key] = true
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
			token, e := d.Token()
			if e != nil || token != json.Delim('}') {
				return bad()
			}
		case '[':
			for d.More() {
				if e := walk(depth + 1); e != nil {
					return e
				}
			}
			token, e := d.Token()
			if e != nil || token != json.Delim(']') {
				return bad()
			}
		default:
			return bad()
		}
		return nil
	}
	if walk(0) != nil {
		return bad()
	}
	if _, err := d.Token(); err != io.EOF {
		return bad()
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value any
	if d.Decode(&value) != nil {
		return bad()
	}
	var shape func(any, reflect.Type) bool
	shape = func(value any, t reflect.Type) bool {
		if t.Kind() == reflect.Pointer {
			if value == nil {
				return true
			}
			return shape(value, t.Elem())
		}
		if value == nil {
			return t.Kind() == reflect.Map || t.Kind() == reflect.Slice || t.Kind() == reflect.Interface
		}
		switch t.Kind() {
		case reflect.Struct:
			object, ok := value.(map[string]any)
			if !ok {
				return false
			}
			fields := map[string]reflect.Type{}
			var collect func(reflect.Type)
			collect = func(owner reflect.Type) {
				for i := 0; i < owner.NumField(); i++ {
					f := owner.Field(i)
					if f.PkgPath != "" {
						continue
					}
					tag := strings.Split(f.Tag.Get("json"), ",")[0]
					if tag == "-" {
						continue
					}
					if f.Anonymous && tag == "" {
						base := f.Type
						if base.Kind() == reflect.Pointer {
							base = base.Elem()
						}
						if base.Kind() == reflect.Struct {
							collect(base)
							continue
						}
					}
					if tag == "" {
						tag = f.Name
					}
					fields[tag] = f.Type
				}
			}
			collect(t)
			for key, item := range object {
				field, ok := fields[key]
				if !ok || !shape(item, field) {
					return false
				}
			}
			return true
		case reflect.Slice, reflect.Array:
			array, ok := value.([]any)
			if !ok {
				return false
			}
			for _, item := range array {
				if !shape(item, t.Elem()) {
					return false
				}
			}
			return true
		case reflect.Map, reflect.Interface:
			return true
		default:
			return value != nil
		}
	}
	t := reflect.TypeOf(out)
	if t == nil || t.Kind() != reflect.Pointer || value == nil || !shape(value, t.Elem()) {
		return bad()
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return bad()
	}
	return nil
}
