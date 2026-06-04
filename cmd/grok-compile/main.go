// Command grok-compile parses .gk files and transpiles them to Go.
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
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: grok-compile <file.gk>... [-o output.go] [-pkg name]\n")
		os.Exit(1)
	}

	var inputs []string
	output := ""
	pkg := "main"

	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "-o":
			i++
			if i < len(os.Args) {
				output = os.Args[i]
			}
		case "-pkg":
			i++
			if i < len(os.Args) {
				pkg = os.Args[i]
			}
		default:
			inputs = append(inputs, os.Args[i])
		}
	}

	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "error: no input files")
		os.Exit(1)
	}

	// Parse all files
	type parsedFile struct {
		file   *ast.File
		input  string
		output string
	}
	var files []parsedFile
	for _, input := range inputs {
		src, err := os.ReadFile(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %v\n", input, err)
			os.Exit(1)
		}
		file, err := parser.ParseFile(string(src), input)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		out := output
		if out == "" {
			out = strings.TrimSuffix(filepath.Base(input), filepath.Ext(input)) + ".go"
		}
		files = append(files, parsedFile{file: file, input: input, output: out})
	}

	// Shared type checker — register types from all files first
	ch := checker.New()
	for _, pf := range files {
		ch.CheckFile(pf.file)
	}
	if errs := ch.Errors(); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
	}

	// Transpile each file
	for _, pf := range files {
		tr := transpiler.New(pkg)
		goSrc := tr.Transpile(pf.file)
		if err := os.WriteFile(pf.output, []byte(goSrc), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", pf.output, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", pf.output)
	}
}
