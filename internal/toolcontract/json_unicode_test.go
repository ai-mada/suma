package toolcontract

import (
	"fmt"
	"testing"
)

func TestValidJSONUnicodeChecksEscapesWithoutRejectingReplacementCharacters(t *testing.T) {
	for _, raw := range []string{`"�"`, `"\ufffd"`, `"😀"`, `"\ud83d\ude00"`, `"\uD800\uDC00"`, `"\udbff\udfff"`, `"\\ud800"`, `"\/\\\""`, `{"nested":["\ud83d\ude00",true,123,null]}`} {
		if !ValidJSONUnicode([]byte(raw)) {
			t.Errorf("valid Unicode or escape rejected: %s", raw)
		}
	}
	for _, raw := range []string{`"\ud800"`, `"\udc00"`, `"\ud800x\udc00"`, `"\ud800\u0041"`, `"\udc00\ud800"`, `"\ud800\\udc00"`, `"\ud800\udc00\ud800"`, `{"\ud800":true}`, `"\u000"`, `"\xFF"`, `"unterminated`, string([]byte{'"', 0xff, '"'})} {
		if ValidJSONUnicode([]byte(raw)) {
			t.Errorf("malformed Unicode or JSON accepted: %q", raw)
		}
	}
	for unit := 0xd800; unit <= 0xdfff; unit++ {
		if ValidJSONUnicode([]byte(fmt.Sprintf(`"\u%04x"`, unit))) {
			t.Fatalf("unpaired surrogate accepted: %04x", unit)
		}
	}
}
