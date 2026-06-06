package ast

import (
	"os"
	"path/filepath"
)

// MergeStdlib merges interface declarations from a parsed stdlib file
// into every block of the target file. Only interfaces that are referenced
// by a relation declaration are merged, to avoid polluting programs that
// don't use them. Also merges classes and functions referenced by name.
func MergeStdlib(file *File, stdFile *File) {
	// Collect all relation hints (interface names used in relations)
	usedIfaces := make(map[string]bool)
	for _, block := range file.Blocks {
		for _, rel := range block.Relations {
			usedIfaces[rel.Hint] = true
		}
	}

	// Collect all type names referenced in user code
	usedTypes := collectUsedTypeNames(file)

	// Collect all function call names in user code
	usedFuncNames := collectUsedFuncNames(file)

	// Build a lookup of all stdlib interfaces by name
	stdIfaceMap := make(map[string]InterfaceDecl)
	for _, block := range stdFile.Blocks {
		for _, iface := range block.Interfaces {
			stdIfaceMap[iface.Name] = iface
		}
	}

	// Build lookup of stdlib classes and functions
	stdClassMap := make(map[string]ClassDecl)
	stdFuncMap := make(map[string]FuncDecl)
	for _, block := range stdFile.Blocks {
		for _, cls := range block.Classes {
			stdClassMap[cls.Name] = cls
		}
		for _, fn := range block.Functions {
			stdFuncMap[fn.Name] = fn
		}
	}

	// Transitively pull in interfaces referenced by embeds
	queue := make([]string, 0, len(usedIfaces))
	for name := range usedIfaces {
		queue = append(queue, name)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		iface, ok := stdIfaceMap[name]
		if !ok {
			continue
		}
		for _, emb := range iface.Embeds {
			embName := emb.Name
			if !usedIfaces[embName] {
				usedIfaces[embName] = true
				queue = append(queue, embName)
			}
		}
	}

	// Collect only referenced interface declarations from the stdlib
	var stdIfaces []InterfaceDecl
	for name := range usedIfaces {
		if iface, ok := stdIfaceMap[name]; ok {
			stdIfaces = append(stdIfaces, iface)
		}
	}

	// Collect referenced classes from the stdlib
	var stdClasses []ClassDecl
	for name := range usedTypes {
		if cls, ok := stdClassMap[name]; ok {
			stdClasses = append(stdClasses, cls)
		}
	}

	// Collect functions that return or use referenced stdlib classes,
	// or that are directly called by user code
	var stdFuncs []FuncDecl
	for name, fn := range stdFuncMap {
		if usedFuncNames[name] || funcReferencesTypes(fn, usedTypes, stdClassMap) {
			stdFuncs = append(stdFuncs, fn)
			// If a function returns a stdlib class, also merge that class
			if fn.ReturnType != nil {
				retNames := make(map[string]bool)
				collectTypeNames(fn.ReturnType, retNames)
				for rn := range retNames {
					if cls, ok := stdClassMap[rn]; ok {
						if !usedTypes[rn] {
							usedTypes[rn] = true
							stdClasses = append(stdClasses, cls)
						}
					}
				}
			}
		}
	}

	if len(stdIfaces) == 0 && len(stdClasses) == 0 && len(stdFuncs) == 0 {
		return
	}

	// Merge into every block of the target file
	for i := range file.Blocks {
		if len(stdIfaces) > 0 {
			file.Blocks[i].Interfaces = append(stdIfaces, file.Blocks[i].Interfaces...)
		}
		if len(stdClasses) > 0 {
			file.Blocks[i].Classes = append(stdClasses, file.Blocks[i].Classes...)
		}
		if len(stdFuncs) > 0 {
			file.Blocks[i].Functions = append(stdFuncs, file.Blocks[i].Functions...)
		}
	}
}

// collectUsedTypeNames walks the user's AST and collects all NamedType references.
func collectUsedTypeNames(file *File) map[string]bool {
	names := make(map[string]bool)
	for _, block := range file.Blocks {
		for _, cls := range block.Classes {
			for _, f := range cls.Fields {
				collectTypeNames(&f.Type, names)
			}
		}
		for _, fn := range block.Functions {
			for _, p := range fn.Params {
				collectTypeNames(&p.Type, names)
			}
			if fn.ReturnType != nil {
				collectTypeNames(fn.ReturnType, names)
			}
		}
		for _, s := range block.Structs {
			for _, f := range s.Fields {
				collectTypeNames(&f.Type, names)
			}
		}
	}
	return names
}

// collectTypeNames recursively extracts NamedType names from a type expression.
func collectTypeNames(te *TypeExpr, names map[string]bool) {
	if te == nil {
		return
	}
	switch te.Kind {
	case TypeNamed:
		if d, ok := te.Data.(NamedType); ok {
			names[d.Name] = true
		} else if dp, ok := te.Data.(*NamedType); ok {
			names[dp.Name] = true
		}
	case TypeOptional:
		if d, ok := te.Data.(OptionalType); ok {
			collectTypeNames(&d.Inner, names)
		}
	case TypeSequence:
		if d, ok := te.Data.(SequenceType); ok {
			collectTypeNames(&d.Elem, names)
		}
	}
}

// funcReferencesTypes returns true if the function's return type references any of the given types.
func funcReferencesTypes(fn FuncDecl, usedTypes map[string]bool, stdClasses map[string]ClassDecl) bool {
	if fn.ReturnType != nil {
		retNames := make(map[string]bool)
		collectTypeNames(fn.ReturnType, retNames)
		for name := range retNames {
			if usedTypes[name] {
				return true
			}
		}
	}
	return false
}

// collectUsedFuncNames walks the user's AST expressions to find function call names.
func collectUsedFuncNames(file *File) map[string]bool {
	names := make(map[string]bool)
	for _, block := range file.Blocks {
		for _, fn := range block.Functions {
			if fn.Body != nil {
				collectFuncCallNames(fn.Body.Stmts, names)
			}
		}
	}
	return names
}

func collectFuncCallNames(stmts []Stmt, names map[string]bool) {
	for _, stmt := range stmts {
		collectFuncCallNamesStmt(&stmt, names)
	}
}

func collectFuncCallNamesStmt(stmt *Stmt, names map[string]bool) {
	switch stmt.Kind {
	case StmtExpr:
		if d, ok := stmt.Data.(*ExprStmt); ok {
			collectFuncCallNamesExpr(&d.Expr, names)
		}
	case StmtVarDecl:
		if d, ok := stmt.Data.(*VarDeclStmt); ok {
			if d.Value != nil {
				collectFuncCallNamesExpr(d.Value, names)
			}
		}
	case StmtReturn:
		if d, ok := stmt.Data.(*ReturnStmt); ok {
			if d.Value != nil {
				collectFuncCallNamesExpr(d.Value, names)
			}
		}
	case StmtIf:
		if d, ok := stmt.Data.(*IfStmt); ok {
			collectFuncCallNamesExpr(&d.Condition, names)
			collectFuncCallNames(d.Then.Stmts, names)
			if d.Else != nil {
				collectFuncCallNames(d.Else.Stmts, names)
			}
			for _, elif := range d.ElseIfs {
				collectFuncCallNamesExpr(&elif.Condition, names)
				collectFuncCallNames(elif.Body.Stmts, names)
			}
		}
	case StmtFor:
		if d, ok := stmt.Data.(*ForStmt); ok {
			collectFuncCallNamesExpr(&d.Collection, names)
			collectFuncCallNames(d.Body.Stmts, names)
		}
	case StmtAssign:
		if d, ok := stmt.Data.(*AssignStmt); ok {
			collectFuncCallNamesExpr(&d.Value, names)
		}
	}
}

func collectFuncCallNamesExpr(expr *Expr, names map[string]bool) {
	if expr == nil {
		return
	}
	switch expr.Kind {
	case ExprCall:
		if d, ok := expr.Data.(*CallExpr); ok {
			if d.Func.Kind == ExprIdent {
				if id, ok := d.Func.Data.(*IdentExpr); ok {
					names[id.Name] = true
				}
			}
			collectFuncCallNamesExpr(&d.Func, names)
			for i := range d.Args {
				collectFuncCallNamesExpr(&d.Args[i], names)
			}
		}
	case ExprBinary:
		if d, ok := expr.Data.(*BinaryExpr); ok {
			collectFuncCallNamesExpr(&d.Left, names)
			collectFuncCallNamesExpr(&d.Right, names)
		}
	case ExprUnary:
		if d, ok := expr.Data.(*UnaryExpr); ok {
			collectFuncCallNamesExpr(&d.Operand, names)
		}
	case ExprFieldAccess:
		if d, ok := expr.Data.(*FieldAccessExpr); ok {
			collectFuncCallNamesExpr(&d.Receiver, names)
		}
	case ExprMethodCall:
		if d, ok := expr.Data.(*MethodCallExpr); ok {
			collectFuncCallNamesExpr(&d.Receiver, names)
			for i := range d.Args {
				collectFuncCallNamesExpr(&d.Args[i], names)
			}
		}
	}
}

// FindStdlibDir locates the stdlib directory.
// Search order:
//  1. FORGE_STDLIB env var
//  2. ../stdlib/ relative to the executable
//  3. ./stdlib/ relative to current directory
func FindStdlibDir() string {
	if dir := os.Getenv("FORGE_STDLIB"); dir != "" {
		return dir
	}

	// Relative to executable
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "..", "stdlib")
		if info, err := os.Stat(filepath.Join(dir, "std.fg")); err == nil && !info.IsDir() {
			return dir
		}
		dir = filepath.Join(filepath.Dir(exe), "stdlib")
		if info, err := os.Stat(filepath.Join(dir, "std.fg")); err == nil && !info.IsDir() {
			return dir
		}
	}

	// Relative to working directory — walk up to find project root
	dir, _ := os.Getwd()
	for dir != "/" && dir != "." {
		candidate := filepath.Join(dir, "stdlib")
		if info, err := os.Stat(filepath.Join(candidate, "std.fg")); err == nil && !info.IsDir() {
			return candidate
		}
		dir = filepath.Dir(dir)
	}

	return ""
}
