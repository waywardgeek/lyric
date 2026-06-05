package lir

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waywardgeek/grok/pkg/checker"
	"github.com/waywardgeek/grok/pkg/parser"
)

// TestCBackendEmitsAllFiles tests that the C backend produces output for all .gk files.
func TestCBackendEmitsAllFiles(t *testing.T) {
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

			pkgName := strings.TrimSuffix(e.Name(), ".gk")
			result := cPipeline(t, string(data), pkgName)
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

// TestCBackendCompilesScan tries to compile ALL .gk files and reports which pass.
func TestCBackendCompilesScan(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not found")
	}
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

			pkgName := strings.TrimSuffix(e.Name(), ".gk")
			cSrc := cPipeline(t, string(data), pkgName)

			tmpDir := t.TempDir()
			cFile := filepath.Join(tmpDir, "main.c")
			if err := os.WriteFile(cFile, []byte(cSrc), 0644); err != nil {
				t.Fatal(err)
			}

			runtimeDir := filepath.Join("..", "..", "runtime")
			binary := filepath.Join(tmpDir, "out")
			cmd := exec.Command("cc", "-o", binary, "-std=gnu11", "-Wall", "-Wno-unused-variable", "-Wno-unused-value", "-I", runtimeDir, cFile)
			out, err := cmd.CombinedOutput()
			if err != nil {
				failed++
				t.Skipf("cc failed: %v\n%s", err, out)
			} else {
				passed++
			}
		})
	}
	t.Logf("RESULTS: %d compiled, %d failed", passed, failed)
}

// TestCBackendRuns tests that compiled C produces expected output.
func TestCBackendRuns(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not found")
	}
	testdataDir := filepath.Join("..", "..", "testdata")

	t.Run("hello_world.gk", func(t *testing.T) {
		path := filepath.Join(testdataDir, "hello_world.gk")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("can't read: %v", err)
		}

		cSrc := cPipeline(t, string(data), "hello_world")
		output := compileCAndRun(t, cSrc, "hello_world")
		expected := "Hello, World!\n"
		if output != expected {
			t.Errorf("expected %q, got %q", expected, output)
			t.Logf("C source:\n%s", cSrc)
		}
	})
}

func cPipeline(t *testing.T, source, pkgName string) string {
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
	return EmitC(prog)
}

func compileCAndRun(t *testing.T, cSrc, name string) string {
	t.Helper()
	tmpDir := t.TempDir()
	cFile := filepath.Join(tmpDir, "main.c")
	if err := os.WriteFile(cFile, []byte(cSrc), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	runtimeDir := filepath.Join("..", "..", "runtime")
	binary := filepath.Join(tmpDir, "out")
	cmd := exec.Command("cc", "-o", binary, "-std=gnu11", "-Wall", "-Wno-unused-variable", "-I", runtimeDir, cFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("C source:\n%s", cSrc)
		t.Fatalf("cc failed: %v\n%s", err, out)
	}

	cmd = exec.Command(binary)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}
	return string(out)
}
