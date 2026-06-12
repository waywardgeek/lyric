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

var commands = []string{"verify", "update", "gen", "fmt", "compile", "test", "help"}

// resolveCommand matches a unique prefix of a command name.
// Returns the full command name or an error if ambiguous/unknown.
func resolveCommand(prefix string) (string, error) {
	if prefix == "-h" || prefix == "--help" {
		return "help", nil
	}
	var matches []string
	for _, c := range commands {
		if c == prefix {
			return c, nil // exact match
		}
		if strings.HasPrefix(c, prefix) {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("unknown command: %s", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous command %q: matches %s", prefix, strings.Join(matches, ", "))
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	cmd, cmdErr := resolveCommand(os.Args[1])
	if cmdErr != nil {
		fmt.Fprintf(os.Stderr, "%v\n\n%s", cmdErr, usage)
		os.Exit(1)
	}
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
	case "help":
		fmt.Print(usage)
		return
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

	checkInvariants := true // on by default
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
		case "--check-invariants":
			checkInvariants = true
		case "--no-check-invariants":
			checkInvariants = false
		default:
			inputs = append(inputs, args[i])
		}
	}

	if len(inputs) == 0 {
		return fmt.Errorf("usage: forge compile <file.fg> [...] [-o output.go] [-pkg name] [-mod modpath]")
	}

	// Parse all input files
	var allFiles []*ast.File
	for _, input := range inputs {
		src, err := os.ReadFile(input)
		if err != nil {
			return fmt.Errorf("reading %s: %w", input, err)
		}
		file, err := parser.ParseFile(string(src), input)
		if err != nil {
			return err
		}
		allFiles = append(allFiles, file)
	}

	// Merge all files into one before any processing (cross-file references)
	merged := ast.MergeFiles(allFiles)

	// Merge stdlib interfaces ONCE into merged file
	stdlibDir := ast.FindStdlibDir()
	if stdlibDir != "" {
		stdFile := loadStdlib(stdlibDir)
		if stdFile != nil {
			ast.MergeStdlib(merged, stdFile)
		}
	}

	// Desugar (all five passes on merged file)
	ast.DesugarInterfaceEmbeds(merged)
	ast.DesugarInterfaceFields(merged)
	ast.DesugarRelations(merged)
	ast.DesugarDestructors(merged)
	ast.DesugarDefaultImpls(merged)

	// Post-desugar invariant checks
	if checkInvariants {
		violations := ast.ValidatePostDesugar(merged)
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "INVARIANT: %s\n", v)
		}
	}

	// Check (two-phase on merged file)
	ch := checker.New()
	ch.CheckFile(merged)
	if errs := ch.Errors(); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
	}


	out := output
	if out == "" {
		out = strings.TrimSuffix(filepath.Base(inputs[0]), filepath.Ext(inputs[0])) + ".c"
	}

	lowerer := lir.NewLowerer()
	prog := lowerer.Lower(merged)
	prog.Package = pkg

	// Post-lower invariant checks (before optimization)
	if checkInvariants {
		violations := lir.ValidatePostLower(prog)
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "INVARIANT: %s\n", v)
		}
		if len(violations) > 0 {
			panic(fmt.Sprintf("%d void* violations (LTyAny in LIR)", len(violations)))
		}
	}

	lir.Optimize(prog)
	lir.Monomorphize(prog)
	lir.RewriteImplRenames(prog)

	if checkInvariants {
		violations := lir.ValidatePostMono(prog)
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "INVARIANT: %s\n", v)
		}
		if len(violations) > 0 {
			panic(fmt.Sprintf("%d post-mono violations", len(violations)))
		}
	}

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
			fmt.Fprintf(os.Stderr, "warning: failed to parse stdlib file %s: %v\n", path, err)
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

	checkInvariants := true // on by default
	var inputs []string
	var outputFile string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--check-invariants":
			checkInvariants = true
		case "--no-check-invariants":
			checkInvariants = false
		case "-o":
			i++
			if i < len(args) {
				outputFile = args[i]
			}
		default:
			inputs = append(inputs, args[i])
		}
	}
	if len(inputs) == 0 {
		return fmt.Errorf("usage: forge test <file.fg> [...]")
	}

	// Parse all input files
	var files []*ast.File
	for _, input := range inputs {
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

	// Post-desugar invariant checks
	if checkInvariants {
		violations := ast.ValidatePostDesugar(merged)
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "INVARIANT: %s\n", v)
		}
	}

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

	// Post-lower invariant checks
	if checkInvariants {
		violations := lir.ValidatePostLower(prog)
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "INVARIANT: %s\n", v)
		}
		if len(violations) > 0 {
			fmt.Fprintf(os.Stderr, "  %d void* violations (LTyAny in LIR)\n", len(violations))
		}
	}

	lir.Optimize(prog)
	lir.Monomorphize(prog)
	lir.RewriteImplRenames(prog)

	if checkInvariants {
		violations := lir.ValidatePostMono(prog)
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "INVARIANT: %s\n", v)
		}
		if len(violations) > 0 {
			fmt.Fprintf(os.Stderr, "  %d post-mono violations\n", len(violations))
		}
	}

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

	// Strip user's main() to avoid duplicate with test runner
	filtered := make([]lir.LFuncDecl, 0, len(prog.Functions))
	for _, f := range prog.Functions {
		if f.Name == "main" && f.Receiver == "" {
			continue
		}
		filtered = append(filtered, f)
	}
	prog.Functions = filtered

	// Generate C source + test runner
	cSrc := lir.EmitC(prog)
	cSrc += lir.EmitTestRunner(testFuncs)

	// If -o was provided, just write the C file and exit
	if outputFile != "" {
		if err := os.WriteFile(outputFile, []byte(cSrc), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", outputFile, err)
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", outputFile)
		return nil
	}

	// Write to temp file
	tmpDir, err := os.MkdirTemp("", "forge-test-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	//defer os.RemoveAll(tmpDir) // DEBUG: keep for inspection
	fmt.Fprintf(os.Stderr, "DEBUG: temp dir: %s\n", tmpDir)

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
