package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUpdateIndexesNestedSourcePaths(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	goSrc := `package example

func Exported() {}
`
	if err := os.WriteFile(filepath.Join(srcDir, "example.go"), []byte(goSrc), 0644); err != nil {
		t.Fatal(err)
	}

	forgeSrc := `forge Example {
  source: ["src/example.go"]
}
`
	forgePath := filepath.Join(dir, "example.forge")
	if err := os.WriteFile(forgePath, []byte(forgeSrc), 0644); err != nil {
		t.Fatal(err)
	}

	if err := runUpdate(forgePath, false); err != nil {
		t.Fatalf("runUpdate failed: %v", err)
	}

	updated, err := os.ReadFile(forgePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	if !strings.Contains(text, "// file: src/example.go") {
		t.Fatalf("expected nested source path in index, got:\n%s", text)
	}
	if !strings.Contains(text, "// func Exported()") {
		t.Fatalf("expected function signature in nested source index, got:\n%s", text)
	}
}
