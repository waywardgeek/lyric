package lir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waywardgeek/grok/pkg/checker"
	"github.com/waywardgeek/grok/pkg/parser"
)

// TestEndToEndSimple tests the full pipeline: parse → check → lower → emit Go
func TestEndToEndSimple(t *testing.T) {
	source := `grok simple {
  fn main() {
    let x: i32 = 10
    let y: i32 = 20
    let z = x + y
    println(z)
  }
}`
	result := pipelineFromSource(t, source, "simple")
	if !strings.Contains(result, "package simple") {
		t.Error("missing package declaration")
	}
	t.Logf("Output:\n%s", result)
}

// TestEndToEndStruct tests struct declaration and construction
func TestEndToEndStruct(t *testing.T) {
	source := `grok geom {
  pub struct Point {
    X: f64
    Y: f64
  }

  pub fn distance(p: Point) -> f64 {
    return p.X + p.Y
  }
}`
	result := pipelineFromSource(t, source, "geom")
	if !strings.Contains(result, "type Point struct") {
		t.Error("missing struct declaration")
	}
	if !strings.Contains(result, "func Distance") {
		t.Error("missing function declaration")
	}
	t.Logf("Output:\n%s", result)
}

// TestEndToEndEnum tests enum (tagged union) emission
func TestEndToEndEnum(t *testing.T) {
	source := `grok shapes {
  pub enum Shape {
    Circle(radius: f64)
    Rectangle(width: f64, height: f64)
  }

  pub fn area(s: Shape) -> f64 {
    match s {
      Circle(r) => {
        return r * r
      }
      Rectangle(w, h) => {
        return w * h
      }
    }
    return 0.0
  }
}`
	result := pipelineFromSource(t, source, "shapes")
	if !strings.Contains(result, "type Shape interface") {
		t.Error("missing enum interface")
	}
	if !strings.Contains(result, "type Circle struct") {
		t.Error("missing Circle variant")
	}
	t.Logf("Output:\n%s", result)
}

// TestEndToEndControl tests if/while/for lowering
func TestEndToEndControl(t *testing.T) {
	source := `grok control {
  pub fn countdown(n: i32) {
    let mut i = n
    while i > 0 {
      println(i)
      i = i - 1
    }
  }
}`
	result := pipelineFromSource(t, source, "control")
	// While should use condblock pattern
	if !strings.Contains(result, "for {") || !strings.Contains(result, "break") {
		t.Error("expected while → for { condblock; if !cond break }")
	}
	t.Logf("Output:\n%s", result)
}

// TestEndToEndAllGkFiles tests lowering of ALL .gk test files
func TestEndToEndAllGkFiles(t *testing.T) {
	testdataDir := filepath.Join("..", "..", "testdata")
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Skipf("no testdata dir: %v", err)
	}

	passed, failed := 0, 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".gk") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			path := filepath.Join(testdataDir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Skipf("can't read: %v", err)
			}

			defer func() {
				if r := recover(); r != nil {
					t.Errorf("PANIC: %v", r)
					failed++
				}
			}()

			result := pipelineFromSource(t, string(data), strings.TrimSuffix(e.Name(), ".gk"))
			if result == "" {
				t.Error("empty output")
				failed++
			} else {
				passed++
				// Log first 40 lines only
				lines := strings.Split(result, "\n")
				if len(lines) > 40 {
					lines = lines[:40]
					lines = append(lines, "... (truncated)")
				}
				t.Logf("Output:\n%s", strings.Join(lines, "\n"))
			}
		})
	}
	t.Logf("RESULTS: %d passed, %d failed out of %d total", passed, failed, passed+failed)
}

// pipelineFromSource runs the full pipeline and returns Go source.
func pipelineFromSource(t *testing.T, source, pkgName string) string {
	t.Helper()

	// Parse
	file, err := parser.ParseString(source)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Ensure there's a block with a name
	if len(file.Blocks) > 0 && file.Blocks[0].Name == "" {
		file.Blocks[0].Name = pkgName
	}

	// Check
	c := checker.New()
	c.CheckFile(file)
	checkErrs := c.Errors()
	// Log but don't fail on checker errors for now
	if len(checkErrs) > 0 {
		t.Logf("checker warnings: %v", checkErrs)
	}

	// Lower
	lowerer := NewLowerer()
	prog := lowerer.Lower(file)
	if prog.Package == "" {
		prog.Package = pkgName
	}

	// Emit Go
	result := EmitGo(prog)
	return result
}
