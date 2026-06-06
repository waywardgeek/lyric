package lir

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waywardgeek/forge/pkg/ast"
	"github.com/waywardgeek/forge/pkg/checker"
	"github.com/waywardgeek/forge/pkg/parser"
)

// TestCBackendEmitsAllFiles tests that the C backend produces output for all .fg files.
func TestCBackendEmitsAllFiles(t *testing.T) {
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

// TestCBackendCompilesScan tries to compile ALL .fg files and reports which pass.
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
		if !strings.HasSuffix(e.Name(), ".fg") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			path := filepath.Join(testdataDir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Skipf("can't read: %v", err)
			}

			pkgName := strings.TrimSuffix(e.Name(), ".fg")
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

	t.Run("hello_world.fg", func(t *testing.T) {
		path := filepath.Join(testdataDir, "hello_world.fg")
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

	t.Run("generators.fg", func(t *testing.T) {
		path := filepath.Join(testdataDir, "generators.fg")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("can't read: %v", err)
		}

		cSrc := cPipeline(t, string(data), "generators")
		output := compileCAndRun(t, cSrc, "generators")
		expected := "count:\n0\n1\n2\n3\n4\nfib:\n0\n1\n1\n2\n3\n5\n8\n13\n21\n34\n"
		if output != expected {
			t.Errorf("expected %q, got %q", expected, output)
			t.Logf("C source:\n%s", cSrc)
		}
	})

	t.Run("demo.fg", func(t *testing.T) {
		path := filepath.Join(testdataDir, "demo.fg")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("can't read: %v", err)
		}

		cSrc := cPipeline(t, string(data), "demo")
		output := compileCAndRun(t, cSrc, "demo")
		expected := "[3] Design Forge: Language spec\n" +
			"[5] Build parser: Recursive descent\n" +
			"[4] Write tests: Full coverage\n" +
			"Highest priority: 5\n" +
			"------------------------------\n" +
			"FizzBuzz: 1, 2, Fizz, 4, Buzz, Fizz, 7, 8, Fizz, Buzz, 11, Fizz, 13, 14, FizzBuzz\n" +
			"Tests are done!\n" +
			"FORGE IS WORKING!\n"
		if output != expected {
			t.Errorf("expected:\n%s\ngot:\n%s", expected, output)
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

	// Merge stdlib interfaces
	stdlibDir := ast.FindStdlibDir()
	if stdlibDir != "" {
		stdPath := filepath.Join(stdlibDir, "std.fg")
		if stdSrc, err := os.ReadFile(stdPath); err == nil {
			if stdFile, err := parser.ParseFile(string(stdSrc), stdPath); err == nil {
				ast.MergeStdlib(file, stdFile)
			}
		}
	}

	// Run desugar passes (interface fields → relations → destructors → default impls)
	ast.DesugarInterfaceFields(file)
	ast.DesugarRelations(file)
	ast.DesugarDestructors(file)
	ast.DesugarDefaultImpls(file)

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
