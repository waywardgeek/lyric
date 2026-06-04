// Command grok is the unified CLI for the Grok toolchain.
//
// Usage:
//
//	grok verify <file.grok> [file.grok...]    Check .grok files against Go source
//	grok update <file.grok> [file.grok...]    Regenerate function index and dependencies
//	grok gen <package-dir>                    Scaffold a new .grok file from Go source
//	grok compile <file.gk> [-o out] [-pkg p]  Compile .gk files to Go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/waywardgeek/grok/pkg/ast"
	"github.com/waywardgeek/grok/pkg/checker"
	"github.com/waywardgeek/grok/pkg/parser"
	"github.com/waywardgeek/grok/pkg/transpiler"
	"github.com/waywardgeek/grok/pkg/verifier"
)

const usage = `Usage: grok <command> [arguments]

Commands:
  verify   <file.grok> [...]          Check .grok files against Go source
  update   <file.grok> [...]          Regenerate function index and dependencies
  gen      <package-dir>              Scaffold a new .grok file from Go source
  compile  <file.gk> [...] [-o out]   Compile .gk files to Go
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
		return fmt.Errorf("usage: grok verify <file.grok> [...]")
	}

	totalErrors, totalWarnings := 0, 0
	for _, grokPath := range args {
		result, err := verifier.Verify(grokPath)
		if err != nil {
			return fmt.Errorf("%s: %w", grokPath, err)
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
		return fmt.Errorf("usage: grok update <file.grok> [...]")
	}
	for _, grokPath := range args {
		if err := runUpdate(grokPath); err != nil {
			return fmt.Errorf("%s: %w", grokPath, err)
		}
		fmt.Printf("updated %s\n", grokPath)
	}
	return nil
}

// --- gen ---

func cmdGen(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: grok gen <package-dir>")
	}
	return runGen(args[0])
}

// --- compile ---

func cmdCompile(args []string) error {
	var inputs []string
	output := ""
	pkg := "main"

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
		default:
			inputs = append(inputs, args[i])
		}
	}

	if len(inputs) == 0 {
		return fmt.Errorf("usage: grok compile <file.gk> [...] [-o output.go] [-pkg name]")
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
			out = strings.TrimSuffix(filepath.Base(input), filepath.Ext(input)) + ".go"
		}
		files = append(files, parsedFile{file: file, input: input, output: out})
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

	for _, pf := range files {
		tr := transpiler.New(pkg)
		goSrc := tr.Transpile(pf.file)
		if err := os.WriteFile(pf.output, []byte(goSrc), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", pf.output, err)
		}
		fmt.Printf("wrote %s\n", pf.output)
	}
	return nil
}
