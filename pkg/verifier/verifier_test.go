package verifier

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyParserGrok(t *testing.T) {
	// Find project root (contains go.mod)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root")
		}
		dir = parent
	}

	grokFile := filepath.Join(dir, "grok", "parser.grok")
	result, err := Verify(grokFile, dir)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	for _, f := range result.Findings {
		switch f.Severity {
		case Error:
			t.Errorf("%s", f)
		case Warning:
			t.Logf("%s", f)
		case Info:
			t.Logf("%s", f)
		}
	}
}

func TestSnakeToPascal(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"name", "Name"},
		{"type_params", "TypeParams"},
		{"is_many", "IsMany"},
		{"return_type", "ReturnType"},
		{"parse_grok_block", "ParseGrokBlock"},
	}
	for _, tt := range tests {
		got := snakeToPascal(tt.in)
		if got != tt.want {
			t.Errorf("snakeToPascal(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
