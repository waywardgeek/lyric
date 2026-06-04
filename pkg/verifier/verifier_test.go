package verifier

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root")
		}
		dir = parent
	}
}

func TestVerifyParserGrok(t *testing.T) {
	dir := findProjectRoot(t)

	grokFile := filepath.Join(dir, "pkg", "parser", "parser.grok")
	result, err := Verify(grokFile)
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

func TestVerifyAstGrok(t *testing.T) {
	dir := findProjectRoot(t)
	grokFile := filepath.Join(dir, "pkg", "ast", "ast.grok")
	result, err := Verify(grokFile)
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

func TestTypeDriftDetected(t *testing.T) {
	// Create a temporary Go file and .grok file with deliberate type mismatches
	dir := t.TempDir()

	// Write a Go source file
	goSrc := `package example

type Widget struct {
	Name    string
	Count   int
	Tags    []string
	Options map[string]bool
}

func NewWidget(name string, count int) *Widget {
	return &Widget{Name: name, Count: count}
}
`
	goFile := filepath.Join(dir, "widget.go")
	if err := os.WriteFile(goFile, []byte(goSrc), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a .grok file with deliberate drift
	grokSrc := `grok Example {
  struct Widget {
    name:    string
    count:   string
    tags:    [int]
    options: map[string]string
    missing: bool
  }

  func new_widget(name: string) -> Widget

  source: ["widget.go"]
}
`
	grokFile := filepath.Join(dir, "example.grok")
	if err := os.WriteFile(grokFile, []byte(grokSrc), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Verify(grokFile)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	// Collect error messages
	var errors []string
	for _, f := range result.Findings {
		if f.Severity == Error {
			errors = append(errors, f.Message)
		}
	}

	// Expected errors:
	// 1. count type mismatch (string vs int)
	// 2. tags type mismatch ([]int vs []string)
	// 3. options type mismatch (map[string]string vs map[string]bool)
	// 4. missing field not in Go
	// 5. new_widget param count mismatch (1 vs 2)
	if len(errors) < 5 {
		t.Errorf("expected at least 5 errors, got %d:", len(errors))
		for _, e := range errors {
			t.Logf("  %s", e)
		}
	}

	// Verify specific drift was caught
	assertContains := func(substr string) {
		for _, e := range errors {
			if strings.Contains(e, substr) {
				return
			}
		}
		t.Errorf("expected error containing %q, not found in: %v", substr, errors)
	}

	assertContains("count")
	assertContains("type mismatch")
	assertContains("missing")
	assertContains("param count mismatch")
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


func TestCompletenessCheck(t *testing.T) {
	dir := t.TempDir()

	// Go file with several exported symbols
	goSrc := `package example

type Widget struct {
	Name string
}

type Config struct {
	Debug bool
}

type unexported struct {
	x int
}

func NewWidget() *Widget { return nil }
func ParseConfig() *Config { return nil }
func helper() {}
`
	if err := os.WriteFile(filepath.Join(dir, "example.go"), []byte(goSrc), 0644); err != nil {
		t.Fatal(err)
	}

	// .grok only documents Widget — Config and ParseConfig are missing
	grokSrc := `grok Example {
  struct Widget {
    Name: string
  }

  func NewWidget() -> Widget?

  source: ["example.go"]
}
`
	grokFile := filepath.Join(dir, "example.grok")
	if err := os.WriteFile(grokFile, []byte(grokSrc), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Verify(grokFile)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	var warnings []string
	for _, f := range result.Findings {
		if f.Severity == Warning {
			warnings = append(warnings, f.Message)
		}
	}

	// Should warn about Config (exported type) and ParseConfig (exported func)
	// Should NOT warn about unexported or helper
	assertWarning := func(substr string) {
		for _, w := range warnings {
			if strings.Contains(w, substr) {
				return
			}
		}
		t.Errorf("expected warning containing %q, got: %v", substr, warnings)
	}
	assertNoWarning := func(substr string) {
		for _, w := range warnings {
			if strings.Contains(w, substr) {
				t.Errorf("unexpected warning containing %q: %s", substr, w)
			}
		}
	}

	assertWarning("Config")
	assertWarning("ParseConfig")
	assertNoWarning("unexported")
	assertNoWarning("helper")
}
