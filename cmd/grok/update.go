package main

import (
	"fmt"
	goast "go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/waywardgeek/grok/pkg/parser"
)

// runUpdate regenerates Zone 2 and Zone 3 of an existing .grok file.
func runUpdate(grokPath string) error {
	raw, err := os.ReadFile(grokPath)
	if err != nil {
		return err
	}

	zone1, _ := splitAtMarker(string(raw), "// --- index ---")

	sources, err := extractSourcePaths(grokPath, zone1)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		sources, err = scanGoFiles(filepath.Dir(grokPath))
		if err != nil {
			return err
		}
	}

	grokDir := filepath.Dir(grokPath)
	modPath := findModulePath(grokDir)

	var allFuncs []funcEntry
	importSet := make(map[string]bool)

	for _, src := range sources {
		fullPath := filepath.Join(grokDir, src)
		funcs, imports, err := extractFuncsFromGoFile(fullPath, filepath.Base(src))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", src, err)
			continue
		}
		allFuncs = append(allFuncs, funcs...)
		for _, imp := range imports {
			importSet[imp] = true
		}
	}

	zone2 := buildFunctionIndex(allFuncs, sources)
	zone3 := buildDependencies(importSet, modPath)

	return os.WriteFile(grokPath, []byte(zone1+zone2+zone3), 0644)
}

func splitAtMarker(text, marker string) (string, string) {
	idx := strings.Index(text, marker)
	if idx < 0 {
		return text, ""
	}
	return text[:idx], text[idx:]
}

func extractSourcePaths(grokPath, zone1 string) ([]string, error) {
	f, err := parser.ParseString(zone1)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", grokPath, err)
	}
	var sources []string
	seen := make(map[string]bool)
	for _, block := range f.Blocks {
		for _, s := range block.Source {
			if !seen[s] {
				sources = append(sources, s)
				seen[s] = true
			}
		}
	}
	return sources, nil
}

func extractFuncsFromGoFile(fullPath, baseName string) ([]funcEntry, []string, error) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, fullPath, nil, goparser.ParseComments)
	if err != nil {
		return nil, nil, err
	}

	var funcs []funcEntry
	for _, decl := range f.Decls {
		fn, ok := decl.(*goast.FuncDecl)
		if !ok {
			continue
		}
		entry := funcEntry{
			File: baseName,
			Line: fset.Position(fn.Pos()).Line,
		}
		if fn.Doc != nil && len(fn.Doc.List) > 0 {
			entry.DocLine = strings.TrimPrefix(fn.Doc.List[0].Text, "// ")
			entry.DocLine = strings.TrimPrefix(entry.DocLine, "/* ")
		}
		entry.Signature = buildSignature(fn, fset)
		funcs = append(funcs, entry)
	}

	sort.Slice(funcs, func(i, j int) bool { return funcs[i].Line < funcs[j].Line })

	var imports []string
	for _, imp := range f.Imports {
		imports = append(imports, strings.Trim(imp.Path.Value, `"`))
	}
	return funcs, imports, nil
}
