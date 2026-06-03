// grok-verify is a CLI tool that checks .grok understanding files against Go source.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/waywardgeek/grok/pkg/verifier"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: grok-verify <file.grok> [base-dir]\n")
		os.Exit(1)
	}

	grokPath := os.Args[1]
	baseDir := "."
	if len(os.Args) >= 3 {
		baseDir = os.Args[2]
	} else {
		// Default: project root is parent of the grok file's directory
		abs, err := filepath.Abs(grokPath)
		if err == nil {
			// Walk up looking for go.mod
			dir := filepath.Dir(abs)
			for {
				if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
					baseDir = dir
					break
				}
				parent := filepath.Dir(dir)
				if parent == dir {
					break
				}
				dir = parent
			}
		}
	}

	result, err := verifier.Verify(grokPath, baseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	errors := 0
	warnings := 0
	for _, f := range result.Findings {
		fmt.Println(f)
		switch f.Severity {
		case verifier.Error:
			errors++
		case verifier.Warning:
			warnings++
		}
	}

	fmt.Printf("\n%d errors, %d warnings\n", errors, warnings)
	if errors > 0 {
		os.Exit(1)
	}
}
