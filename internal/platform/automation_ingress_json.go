package platform

import (
	"bytes"
	"encoding/json"
	"io"
)

// Bounds apply before decoding provider-specific structs, including fields we
// discard. They keep a signed but unusually wide/deep JSON object finite.
func validateIngressJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	fields, values := 0, 0
	var visit func(int) error
	visit = func(depth int) error {
		values++
		if depth > 128 || values > 32768 {
			return invalid("事件 JSON 结构超过解析预算")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				fields++
				if !ok || seen[name] || fields > 16384 {
					return invalid("事件 JSON 字段重复或超过解析预算")
				}
				seen[name] = true
				if err = visit(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err = visit(depth + 1); err != nil {
					return err
				}
			}
		default:
			return invalid("事件 JSON 结构无效")
		}
		_, err = d.Token()
		return err
	}
	if err := visit(0); err != nil {
		return invalid("事件 JSON 无效、重复或超过解析预算")
	}
	if _, err := d.Token(); err != io.EOF {
		return invalid("事件包含多余 JSON 内容")
	}
	return nil
}
