package codexchat

import (
	"encoding/json"
	"testing"
)

func TestRawIDValuePreservesServerRequestIDType(t *testing.T) {
	numeric := rawIDValue(json.RawMessage(`0`))
	if _, ok := numeric.(int64); !ok {
		t.Fatalf("numeric rawIDValue type = %T, want int64", numeric)
	}
	if numeric != int64(0) {
		t.Fatalf("numeric rawIDValue = %#v, want int64(0)", numeric)
	}

	stringID := rawIDValue(json.RawMessage(`"0"`))
	if _, ok := stringID.(string); !ok {
		t.Fatalf("string rawIDValue type = %T, want string", stringID)
	}
	if stringID != "0" {
		t.Fatalf("string rawIDValue = %#v, want %q", stringID, "0")
	}
}
