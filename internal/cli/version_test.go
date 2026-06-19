package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteVersionText(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := writeVersion(&out, versionInfo{
		Version: "0.1.0",
		Commit:  "abc1234",
		Date:    "2026-06-19T00:00:00Z",
	}, false)
	if err != nil {
		t.Fatalf("write version: %v", err)
	}

	want := "vpsm 0.1.0\ncommit: abc1234\nbuilt: 2026-06-19T00:00:00Z\n"
	if out.String() != want {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestWriteVersionJSON(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := writeVersion(&out, versionInfo{
		Version: "0.1.0",
		Commit:  "abc1234",
		Date:    "2026-06-19T00:00:00Z",
	}, true)
	if err != nil {
		t.Fatalf("write version: %v", err)
	}

	var got versionInfo
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode version json: %v", err)
	}
	if got.Version != "0.1.0" || got.Commit != "abc1234" || got.Date != "2026-06-19T00:00:00Z" {
		t.Fatalf("unexpected version info: %#v", got)
	}
	if !strings.Contains(out.String(), "\n  \"version\":") {
		t.Fatalf("expected indented json, got %q", out.String())
	}
}
