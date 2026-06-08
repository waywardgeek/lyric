package verifier

// Invariant verification tests
//
// These tests programmatically check the cross-cutting invariants documented
// in forge.forge's "Invariants — Cross-Cutting" block. Each test references
// the invariant it verifies and the functions/files it checks.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// projectRoot returns the root of the forge project.
func projectRoot(t *testing.T) string {
	t.Helper()
	// Walk up from pkg/verifier/ to project root
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "../..")
}

// parseGoFileForTest parses a Go file and returns its AST.
func parseGoFileForTest(t *testing.T, path string) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", path, err)
	}
	return f
}

// parseGoDirForTest parses all Go files in a directory.
func parseGoDirForTest(t *testing.T, dir string) map[string]*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.AllErrors)
	if err != nil {
		t.Fatalf("failed to parse dir %s: %v", dir, err)
	}
	files := make(map[string]*ast.File)
	for _, pkg := range pkgs {
		for name, f := range pkg.Files {
			files[name] = f
		}
	}
	return files
}

// Invariant 1: AST Expr is a VALUE TYPE — no range-copy of Expr slices
//
// Checks that no code uses `for _, x := range` over []Expr fields, which
// would copy the value type and lose checker annotations. Index-based
// iteration (&slice[i]) must be used instead.
//
// Anchors: checker.go (checkExpr, checkCall), lower.go (lowerExpr, lowerCall),
//          c_backend.go (emitExpr)
func TestInvariant_NoRangeCopyOfExprSlices(t *testing.T) {
	root := projectRoot(t)
	dirs := []string{
		filepath.Join(root, "pkg/checker"),
		filepath.Join(root, "pkg/ast"),
	}
	// Note: pkg/lir/ uses LValue/LExpr (different types), not ast.Expr.
	// The invariant applies to AST Expr only (checker + desugar layers).

	for _, dir := range dirs {
		files := parseGoDirForTest(t, dir)
		for filename, f := range files {
			ast.Inspect(f, func(n ast.Node) bool {
				rangeStmt, ok := n.(*ast.RangeStmt)
				if !ok {
					return true
				}
				// Check if we're ranging over something that could be an Expr slice
				// and using value binding (not just index)
				if rangeStmt.Value == nil {
					return true // index-only, safe
				}
				// Look at the expression being ranged over
				rangeExpr := exprString(rangeStmt.X)
				// Check for known Expr slice fields
				dangerousFields := []string{
					".Args", ".Parts", ".Values", ".Conditions",
					".Elts", ".Elements",
				}
				for _, field := range dangerousFields {
					if strings.HasSuffix(rangeExpr, field) {
						// Check if the value is used with & (pointer)
						valName := ""
						if ident, ok := rangeStmt.Value.(*ast.Ident); ok {
							valName = ident.Name
						}
						if valName != "_" {
							// This is potentially dangerous — ranging over Expr slice
							// with value binding
							short := filepath.Base(filename)
							t.Errorf("%s: range-copy over %s with value binding '%s' — "+
								"use index-based iteration (&slice[i]) per Invariant 1 (AST Expr is VALUE TYPE)",
								short, rangeExpr, valName)
						}
					}
				}
				return true
			})
		}
	}
}

// Invariant 2: ClassRenames must be checked in field/type lookups
//
// Verifies that resolveFieldType checks ClassRenames.
//
// Anchors: c_backend.go (resolveFieldType)
func TestInvariant_ClassRenamesChecked(t *testing.T) {
	root := projectRoot(t)
	cBackend := filepath.Join(root, "pkg/lir/c_backend.go")

	content, err := os.ReadFile(cBackend)
	if err != nil {
		t.Fatal(err)
	}
	src := string(content)

	// resolveFieldType MUST reference ClassRenames
	idx := strings.Index(src, "func (g *cGen) resolveFieldType")
	if idx == -1 {
		t.Fatal("resolveFieldType not found in c_backend.go")
	}

	end := idx + 3000
	if end > len(src) {
		end = len(src)
	}
	body := src[idx:end]

	if !strings.Contains(body, "ClassRenames") {
		t.Error("resolveFieldType does not reference ClassRenames — " +
			"per Invariant 2, field lookups MUST check ClassRenames after monomorphization")
	}
}

// Invariant 3: MergeStdlib merges into block 0 only
//
// Anchors: pkg/ast/stdlib.go (MergeStdlib)
func TestInvariant_MergeStdlibBlock0(t *testing.T) {
	root := projectRoot(t)

	// MergeStdlib is in pkg/ast/stdlib.go
	stdlibFile := filepath.Join(root, "pkg/ast/stdlib.go")
	content, err := os.ReadFile(stdlibFile)
	if err != nil {
		t.Fatalf("failed to read stdlib.go: %v", err)
	}
	src := string(content)

	idx := strings.Index(src, "func MergeStdlib")
	if idx == -1 {
		t.Fatal("MergeStdlib function not found in pkg/ast/stdlib.go")
	}

	// Find end of function (next "func " or EOF)
	end := strings.Index(src[idx+1:], "\nfunc ")
	var body string
	if end == -1 {
		body = src[idx:]
	} else {
		body = src[idx : idx+1+end]
	}

	if !strings.Contains(body, "Blocks[0]") {
		t.Error("MergeStdlib does not reference Blocks[0] — per Invariant 3, " +
			"stdlib must merge into block 0 ONLY")
	}
}

// Invariant 4: Desugar order is Embeds → InterfaceFields → Relations → Destructors → DefaultImpls
//
// Checks the CALL ORDER in cmd/forge/main.go (not definition order in desugar.go).
// Anchors: cmd/forge/main.go (cmdCompile, cmdTest)
func TestInvariant_DesugarOrder(t *testing.T) {
	root := projectRoot(t)

	mainFile := filepath.Join(root, "cmd/forge/main.go")
	content, err := os.ReadFile(mainFile)
	if err != nil {
		t.Fatal(err)
	}
	src := string(content)

	// The required call order
	expectedOrder := []string{
		"DesugarInterfaceEmbeds",
		"DesugarInterfaceFields",
		"DesugarRelations",
		"DesugarDestructors",
		"DesugarDefaultImpls",
	}

	// Find positions of each call (first occurrence)
	positions := make([]int, len(expectedOrder))
	for i, name := range expectedOrder {
		pos := strings.Index(src, name+"(")
		if pos == -1 {
			t.Errorf("desugar call %s not found in cmd/forge/main.go", name)
			return
		}
		positions[i] = pos
	}

	// Verify order
	for i := 1; i < len(positions); i++ {
		if positions[i] <= positions[i-1] {
			t.Errorf("desugar order violation: %s appears before %s — "+
				"per Invariant 4, order must be: %s",
				expectedOrder[i], expectedOrder[i-1],
				strings.Join(expectedOrder, " → "))
		}
	}
}


// Invariant: Checker multi-phase — Phase 0 (type name stubs), Phase 1 (register), Phase 2 (bodies)
//
// Anchors: checker.go (CheckFile, CheckFiles)
func TestInvariant_CheckerMultiPhase(t *testing.T) {
	root := projectRoot(t)
	checkerFile := filepath.Join(root, "pkg/checker/checker.go")

	content, err := os.ReadFile(checkerFile)
	if err != nil {
		t.Fatal(err)
	}
	src := string(content)

	// CheckFiles must exist for cross-file compilation
	if !strings.Contains(src, "func (c *Checker) CheckFiles") {
		t.Error("Checker.CheckFiles not found — required for cross-file type resolution")
	}

	// Phase 0 should pre-register type names
	if !strings.Contains(src, "Phase 0") && !strings.Contains(src, "phase 0") &&
		!strings.Contains(src, "preRegister") && !strings.Contains(src, "PreRegister") {
		// Check for the actual pattern: registering type names before full registration
		if !strings.Contains(src, "registerTypeName") && !strings.Contains(src, "TypeStub") {
			t.Log("Warning: No explicit Phase 0 type name pre-registration found in checker.go")
		}
	}
}

// Invariant: StringInterpExpr.Parts iteration must be index-based
//
// Specifically checks that the string interpolation range-copy bug
// (fixed 2026-06-08) hasn't regressed.
//
// Anchors: checker.go (checkStringInterp), lower.go (lowerStringInterp)
func TestInvariant_StringInterpIndexIteration(t *testing.T) {
	root := projectRoot(t)
	files := map[string]string{
		"checker": filepath.Join(root, "pkg/checker/checker.go"),
		"lowerer": filepath.Join(root, "pkg/lir/lower.go"),
	}

	for name, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read %s: %v", name, err)
		}
		src := string(content)

		// Find StringInterp handling
		idx := strings.Index(src, "StringInterp")
		if idx == -1 {
			continue // Not all files handle it
		}

		// Look for dangerous pattern: "for _, part := range" near StringInterp
		// within 500 chars
		searchArea := src[idx:]
		if len(searchArea) > 2000 {
			searchArea = searchArea[:2000]
		}

		if strings.Contains(searchArea, "for _, part := range") ||
			strings.Contains(searchArea, "for _, p := range") {
			t.Errorf("%s: StringInterp handling uses range-copy pattern — "+
				"must use index-based iteration per 2026-06-08 fix", name)
		}
	}
}

// exprString returns a simple string representation of an AST expression.
func exprString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprString(e.X) + "." + e.Sel.Name
	case *ast.IndexExpr:
		return exprString(e.X) + "[...]"
	default:
		return ""
	}
}
