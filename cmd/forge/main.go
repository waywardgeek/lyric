// Command forge is the unified CLI for the Forge toolchain.
//
// Usage:
//
//	forge verify <file.forge> [file.forge...]    Check .forge files against Go source
//	forge update <file.forge> [file.forge...]    Regenerate function index and dependencies
//	forge gen <package-dir>                    Scaffold a new .forge file from Go source
//	forge compile <file.fg> [-o out] [-pkg p]  Compile .fg files to C
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/waywardgeek/forge/pkg/ast"
	"github.com/waywardgeek/forge/pkg/checker"
	"github.com/waywardgeek/forge/pkg/lir"
	"github.com/waywardgeek/forge/pkg/parser"
	"github.com/waywardgeek/forge/pkg/verifier"
)

const usage = `Usage: forge <command> [arguments]

Commands:
  verify   <file.forge> [...]          Check .forge files against Go source
  update   <file.forge> [...]          Regenerate function index and dependencies
  gen      <package-dir>              Scaffold a new .forge file from Go source
  fmt      <file.forge> [...]          Format .forge files
  compile  <file.fg> [...] [-o out]            Compile .fg files to C
  test     <file.fg> [...]            Compile, discover test_* functions, run tests
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "verify":
		err = cmdVerify(args)
	case "update":
		err = cmdUpdate(args)
	case "gen":
		err = cmdGen(args)
	case "fmt":
		err = cmdFmt(args)
	case "compile":
		err = cmdCompile(args)
	case "test":
		err = cmdTest(args)
	case "help", "-h", "--help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", cmd, usage)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// --- verify ---

func cmdVerify(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: forge verify <file.forge> [...]")
	}

	totalErrors, totalWarnings := 0, 0
	for _, forgePath := range args {
		result, err := verifier.Verify(forgePath)
		if err != nil {
			return fmt.Errorf("%s: %w", forgePath, err)
		}
		for _, f := range result.Findings {
			fmt.Println(f)
			switch f.Severity {
			case verifier.Error:
				totalErrors++
			case verifier.Warning:
				totalWarnings++
			}
		}
	}

	fmt.Printf("\n%d errors, %d warnings\n", totalErrors, totalWarnings)
	if totalErrors > 0 {
		os.Exit(1)
	}
	return nil
}

// --- update ---

func cmdUpdate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: forge update [--prune] <file.forge> [...]")
	}
	prune := false
	var files []string
	for _, a := range args {
		if a == "--prune" {
			prune = true
		} else {
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		return fmt.Errorf("usage: forge update [--prune] <file.forge> [...]")
	}
	for _, forgePath := range files {
		if err := runUpdate(forgePath, prune); err != nil {
			return fmt.Errorf("%s: %w", forgePath, err)
		}
		fmt.Printf("updated %s\n", forgePath)
	}
	return nil
}

// --- gen ---

func cmdGen(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: forge gen <package-dir>")
	}
	return runGen(args[0])
}

// --- compile ---

func cmdCompile(args []string) error {
	var inputs []string
	output := ""
	pkg := "main"
	modPath := ""
	_ = modPath // reserved for future multi-file module support

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o":
			i++
			if i < len(args) {
				output = args[i]
			}
		case "-pkg":
			i++
			if i < len(args) {
				pkg = args[i]
			}
		case "-mod":
			i++
			if i < len(args) {
				modPath = args[i]
			}
		case "--c":
			// accepted for backwards compatibility, already the default
		default:
			inputs = append(inputs, args[i])
		}
	}

	if len(inputs) == 0 {
		return fmt.Errorf("usage: forge compile <file.fg> [...] [-o output.go] [-pkg name] [-mod modpath]")
	}

	type parsedFile struct {
		file   *ast.File
		input  string
		output string
	}
	var files []parsedFile
	for _, input := range inputs {
		src, err := os.ReadFile(input)
		if err != nil {
			return fmt.Errorf("reading %s: %w", input, err)
		}
		file, err := parser.ParseFile(string(src), input)
		if err != nil {
			return err
		}
		out := output
		if out == "" {
			out = strings.TrimSuffix(filepath.Base(input), filepath.Ext(input)) + ".c"
		}
		files = append(files, parsedFile{file: file, input: input, output: out})
	}

	// Merge stdlib interfaces into all files before desugaring
	stdlibDir := ast.FindStdlibDir()
	if stdlibDir != "" {
		stdFile := loadStdlib(stdlibDir)
		if stdFile != nil {
			for _, pf := range files {
				ast.MergeStdlib(pf.file, stdFile)
			}
		}
	}

	// Desugar: embeds → flatten, interface fields → getters/setters, relations → field injection + impl blocks,
	// destructors → destroy methods on owned classes, default impls → generic functions
	for _, pf := range files {
		ast.DesugarInterfaceEmbeds(pf.file)
		ast.DesugarInterfaceFields(pf.file)
		ast.DesugarRelations(pf.file)
		ast.DesugarDestructors(pf.file)
		ast.DesugarDefaultImpls(pf.file)
	}

	ch := checker.New()
	for _, pf := range files {
		ch.CheckFile(pf.file)
	}
	if errs := ch.Errors(); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
	}

	// Merge all files into one for unified lowering (cross-file references)
	var allFiles []*ast.File
	for _, pf := range files {
		allFiles = append(allFiles, pf.file)
	}
	merged := ast.MergeFiles(allFiles)

	out := output
	if out == "" {
		out = strings.TrimSuffix(filepath.Base(inputs[0]), filepath.Ext(inputs[0])) + ".c"
	}

	lowerer := lir.NewLowerer()
	prog := lowerer.Lower(merged)
	prog.Package = pkg
	lir.Optimize(prog)
	lir.Monomorphize(prog)
	lir.RewriteImplRenames(prog)
	src := lir.EmitC(prog)

	if err := os.WriteFile(out, []byte(src), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", out, err)
	}
	fmt.Printf("wrote %s\n", out)
	return nil
}


// loadStdlib loads and parses all .fg files from the stdlib directory,
// merging them into a single ast.File for use with MergeStdlib.
func loadStdlib(dir string) *ast.File {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var combined *ast.File
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".fg") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		file, err := parser.ParseFile(string(src), path)
		if err != nil {
			continue
		}
		if combined == nil {
			combined = file
		} else {
			// Merge blocks from this file into the combined file
			combined.Blocks = append(combined.Blocks, file.Blocks...)
		}
	}
	return combined
}

// --- test ---

func cmdTest(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: forge test <file.fg> [...]")
	}

	// Parse all input files
	var files []*ast.File
	for _, input := range args {
		src, err := os.ReadFile(input)
		if err != nil {
			return fmt.Errorf("reading %s: %w", input, err)
		}
		file, err := parser.ParseFile(string(src), input)
		if err != nil {
			return err
		}
		files = append(files, file)
	}

	// Merge all files into one compilation unit before processing
	merged := ast.MergeFiles(files)

	// Merge stdlib
	stdlibDir := ast.FindStdlibDir()
	if stdlibDir != "" {
		stdFile := loadStdlib(stdlibDir)
		if stdFile != nil {
			ast.MergeStdlib(merged, stdFile)
		}
	}

	// Desugar
	ast.DesugarInterfaceEmbeds(merged)
	ast.DesugarInterfaceFields(merged)
	ast.DesugarRelations(merged)
	ast.DesugarDestructors(merged)
	ast.DesugarDefaultImpls(merged)

	// Check
	ch := checker.New()
	ch.CheckFile(merged)
	if errs := ch.Errors(); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
	}
	lowerer := lir.NewLowerer()
	prog := lowerer.Lower(merged)
	prog.Package = "test"
	lir.Optimize(prog)
	lir.Monomorphize(prog)
	lir.RewriteImplRenames(prog)

	// Discover test_* functions
	var testFuncs []string
	for _, f := range prog.Functions {
		name := f.Name
		if f.Receiver != "" {
			continue
		}
		if strings.HasPrefix(name, "test_") {
			testFuncs = append(testFuncs, name)
		}
	}

	if len(testFuncs) == 0 {
		fmt.Fprintln(os.Stderr, "no test_* functions found")
		return nil
	}

	// Generate C source + test runner
	cSrc := lir.EmitC(prog)
	cSrc += lir.EmitTestRunner(testFuncs)

	// Write to temp file
	tmpDir, err := os.MkdirTemp("", "forge-test-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cFile := filepath.Join(tmpDir, "test.c")
	binFile := filepath.Join(tmpDir, "test")
	if err := os.WriteFile(cFile, []byte(cSrc), 0644); err != nil {
		return fmt.Errorf("writing C: %w", err)
	}

	// Copy runtime header
	runtimeDir := findRuntimeDir()
	if runtimeDir != "" {
		rtSrc, err := os.ReadFile(filepath.Join(runtimeDir, "forge_runtime.h"))
		if err == nil {
			os.WriteFile(filepath.Join(tmpDir, "forge_runtime.h"), rtSrc, 0644)
		}
	}

	// Compile
	gcc := exec.Command("gcc", "-std=gnu11", "-O0", "-g", "-o", binFile, cFile, "-I", tmpDir)
	gcc.Stderr = os.Stderr
	gcc.Stdout = os.Stdout
	if err := gcc.Run(); err != nil {
		return fmt.Errorf("gcc compilation failed: %w", err)
	}

	// Run
	test := exec.Command(binFile)
	test.Stderr = os.Stderr
	test.Stdout = os.Stdout
	if err := test.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

func findRuntimeDir() string {
	// Check relative to executable
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "..", "runtime")
		if _, err := os.Stat(filepath.Join(dir, "forge_runtime.h")); err == nil {
			return dir
		}
	}
	// Check relative to working directory
	candidates := []string{"runtime", "../runtime", "../../runtime"}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "forge_runtime.h")); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}
