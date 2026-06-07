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
		stdPath := filepath.Join(stdlibDir, "std.fg")
		if stdSrc, err := os.ReadFile(stdPath); err == nil {
			if stdFile, err := parser.ParseFile(string(stdSrc), stdPath); err == nil {
				for _, pf := range files {
					ast.MergeStdlib(pf.file, stdFile)
				}
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
