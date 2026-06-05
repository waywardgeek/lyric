package lir

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/waywardgeek/forge/pkg/checker"
	"github.com/waywardgeek/forge/pkg/parser"
)

// genericFiles are the .fg files that use generics and exercise monomorphization.
var genericFiles = []string{
	"generics.fg",
	"classes.fg",
	"collections.fg",
	"inference.fg",
	"lambdas.fg",
	"where_clause.fg",
	"user_constraint.fg",
}



// TestMonomorphizeCompiles tests that monomorphized output compiles with go build.
func TestMonomorphizeCompiles(t *testing.T) {
	testdataDir := filepath.Join("..", "..", "testdata")

	for _, name := range genericFiles {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(testdataDir, name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Skipf("can't read: %v", err)
			}

			pkgName := strings.TrimSuffix(name, ".fg")
			goSrc := monoPipeline(t, string(data), pkgName)
			if goSrc == "" {
				t.Fatal("empty output")
			}

			// Write to temp dir and compile
			tmpDir := t.TempDir()
			goFile := filepath.Join(tmpDir, "main.go")
			// Replace package line with main for compilation
			if idx := strings.Index(goSrc, "\n"); idx > 0 {
				goSrc = "package main" + goSrc[idx:]
			}
			if err := os.WriteFile(goFile, []byte(goSrc), 0644); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command("go", "build", "-o", filepath.Join(tmpDir, "out"), goFile)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("Go source:\n%s", goSrc)
				t.Fatalf("go build failed: %v\n%s", err, out)
			}
		})
	}
}

// TestMonomorphizeOutputMatches verifies monomorphized output produces the same
// runtime output as the non-monomorphized (pass-through) path.
func TestMonomorphizeOutputMatches(t *testing.T) {
	testdataDir := filepath.Join("..", "..", "testdata")

	for _, name := range genericFiles {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(testdataDir, name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Skipf("can't read: %v", err)
			}

			pkgName := strings.TrimSuffix(name, ".fg")
			source := string(data)

			// Run without monomorphization
			normalSrc := normalPipeline(t, source, pkgName)
			// Run with monomorphization
			monoSrc := monoPipeline(t, source, pkgName)

			// If no func main(), only verify both compile (don't run)
			if !strings.Contains(normalSrc, "func main()") {
				compileOnly(t, normalSrc, pkgName, "normal")
				compileOnly(t, monoSrc, pkgName, "mono")
				return
			}

			normalOut := compileAndRun(t, normalSrc, pkgName, "normal")
			monoOut := compileAndRun(t, monoSrc, pkgName, "mono")

			// Strip pointer addresses (0x...) which vary between runs
			stripPtrs := func(s string) string {
				return regexp.MustCompile(`0x[0-9a-f]+`).ReplaceAllString(s, "PTR")
			}
			if stripPtrs(normalOut) != stripPtrs(monoOut) {
				t.Errorf("output mismatch:\n--- normal ---\n%s\n--- mono ---\n%s", normalOut, monoOut)
			}
		})
	}
}

// TestMonomorphizeAllFiles verifies monomorphization doesn't break non-generic files.
func TestMonomorphizeAllFiles(t *testing.T) {
	testdataDir := filepath.Join("..", "..", "testdata")
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Skipf("no testdata dir: %v", err)
	}

	passed, failed := 0, 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".fg") {
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

			pkgName := strings.TrimSuffix(e.Name(), ".fg")
			result := monoPipeline(t, string(data), pkgName)
			if result == "" {
				t.Error("empty output")
				failed++
			} else {
				passed++
			}
		})
	}
	t.Logf("RESULTS: %d passed, %d failed", passed, failed)
}

func monoPipeline(t *testing.T, source, pkgName string) string {
	t.Helper()
	file, err := parser.ParseString(source)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(file.Blocks) > 0 && file.Blocks[0].Name == "" {
		file.Blocks[0].Name = pkgName
	}

	c := checker.New()
	c.CheckFile(file)

	lowerer := NewLowerer()
	prog := lowerer.Lower(file)
	if prog.Package == "" {
		prog.Package = pkgName
	}

	Optimize(prog)
	Monomorphize(prog)
	return EmitGo(prog)
}

func normalPipeline(t *testing.T, source, pkgName string) string {
	t.Helper()
	file, err := parser.ParseString(source)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(file.Blocks) > 0 && file.Blocks[0].Name == "" {
		file.Blocks[0].Name = pkgName
	}

	c := checker.New()
	c.CheckFile(file)

	lowerer := NewLowerer()
	prog := lowerer.Lower(file)
	if prog.Package == "" {
		prog.Package = pkgName
	}

	Optimize(prog)
	return EmitGo(prog)
}

func compileOnly(t *testing.T, goSrc, pkgName, label string) {
	t.Helper()
	tmpDir := t.TempDir()
	// Don't replace package — compile as a non-main package
	goFile := filepath.Join(tmpDir, pkgName+".go")
	if err := os.WriteFile(goFile, []byte(goSrc), 0644); err != nil {
		t.Fatalf("[%s] write failed: %v", label, err)
	}
	cmd := exec.Command("go", "build", goFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("[%s] Go source:\n%s", label, goSrc)
		t.Fatalf("[%s] go build failed: %v\n%s", label, err, out)
	}
}

func compileAndRun(t *testing.T, goSrc, pkgName, label string) string {
	t.Helper()
	tmpDir := t.TempDir()
	// Replace whatever package line with package main
	if idx := strings.Index(goSrc, "\n"); idx > 0 {
		goSrc = "package main" + goSrc[idx:]
	}
	goFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(goFile, []byte(goSrc), 0644); err != nil {
		t.Fatalf("[%s] write failed: %v", label, err)
	}

	binary := filepath.Join(tmpDir, "out")
	cmd := exec.Command("go", "build", "-o", binary, goFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("[%s] Go source:\n%s", label, goSrc)
		t.Fatalf("[%s] go build failed: %v\n%s", label, err, out)
	}

	cmd = exec.Command(binary)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[%s] run failed: %v\n%s", label, err, out)
	}
	return string(out)
}
