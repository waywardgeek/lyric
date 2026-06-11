package ast

import (
	"fmt"
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

	// Collect user-defined function names so stdlib doesn't shadow them.
	// Users can call shadowed stdlib functions via std.funcName().
	userDefinedFuncs := make(map[string]bool)
	for _, block := range file.Blocks {
		for _, fn := range block.Functions {
			userDefinedFuncs[fn.Name] = true
		}
	}

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
		// Skip stdlib functions that would shadow user-defined functions
		if userDefinedFuncs[name] {
			continue
		}
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

	// Collect stdlib relations whose participant types are being merged.
	// This handles e.g. "relation HashedList Dict<V>:d owns [DictEntry<V>:d]"
	// which gives Dict its fields via the HashedList interface.
	mergedClasses := make(map[string]bool)
	for _, cls := range stdClasses {
		mergedClasses[cls.Name] = true
	}

	// Build set of existing relations to avoid duplicates
	// (when std.fg is both an input file AND stdlib source)
	existingRels := make(map[string]bool)
	for _, block := range file.Blocks {
		for _, rel := range block.Relations {
			key := rel.Hint + ":" + rel.Parent.TypeName + ":" + rel.Child.TypeName
			existingRels[key] = true
		}
	}

	var stdRelations []RelationDecl
	for _, block := range stdFile.Blocks {
		for _, rel := range block.Relations {
			parentName := rel.Parent.TypeName
			childName := rel.Child.TypeName
			// Skip if this relation already exists in the target file
			key := rel.Hint + ":" + parentName + ":" + childName
			if existingRels[key] {
				continue
			}
			if mergedClasses[parentName] || mergedClasses[childName] {
				stdRelations = append(stdRelations, rel)
				// Also ensure the interface hint is merged
				if rel.Hint != "" && !usedIfaces[rel.Hint] {
					usedIfaces[rel.Hint] = true
					if iface, ok := stdIfaceMap[rel.Hint]; ok {
						stdIfaces = append(stdIfaces, iface)
						// Transitively pull embedded interfaces
						for _, emb := range iface.Embeds {
							if !usedIfaces[emb.Name] {
								usedIfaces[emb.Name] = true
								if embIface, ok := stdIfaceMap[emb.Name]; ok {
									stdIfaces = append(stdIfaces, embIface)
								}
							}
						}
					}
				}
				// Ensure both participant types are merged
				if !mergedClasses[parentName] {
					if cls, ok := stdClassMap[parentName]; ok {
						mergedClasses[parentName] = true
						stdClasses = append(stdClasses, cls)
					}
				}
				if !mergedClasses[childName] {
					if cls, ok := stdClassMap[childName]; ok {
						mergedClasses[childName] = true
						stdClasses = append(stdClasses, cls)
					}
				}
			}
		}
	}

	// Transitively pull in functions called by already-merged functions.
	// E.g. dict_set calls sym() which must also be merged.
	mergedFuncs := make(map[string]bool)
	for _, fn := range stdFuncs {
		mergedFuncs[fn.Name] = true
	}
	changed := true
	for changed {
		changed = false
		for name, fn := range stdFuncMap {
			if mergedFuncs[name] {
				continue
			}
			// Check if any merged function calls this one
			for _, mfn := range stdFuncs {
				calls := make(map[string]bool)
				if mfn.Body != nil {
					collectFuncCallNames(mfn.Body.Stmts, calls)
				}
				if calls[name] {
					mergedFuncs[name] = true
					stdFuncs = append(stdFuncs, fn)
					changed = true
					// Also merge return type classes
					if fn.ReturnType != nil {
						retNames := make(map[string]bool)
						collectTypeNames(fn.ReturnType, retNames)
						for rn := range retNames {
							if cls, ok := stdClassMap[rn]; ok {
								if !mergedClasses[rn] {
									mergedClasses[rn] = true
									stdClasses = append(stdClasses, cls)
								}
							}
						}
					}
					break
				}
			}
		}
	}

	if len(stdIfaces) == 0 && len(stdClasses) == 0 && len(stdFuncs) == 0 && len(stdRelations) == 0 {
		return
	}

	// Merge into the first block only (multi-file compilation merges blocks,
	// so merging into every block would cause duplicate definitions)
	if len(file.Blocks) > 0 {
		if len(stdIfaces) > 0 {
			file.Blocks[0].Interfaces = append(stdIfaces, file.Blocks[0].Interfaces...)
		}
		if len(stdClasses) > 0 {
			file.Blocks[0].Classes = append(stdClasses, file.Blocks[0].Classes...)
		}
		if len(stdFuncs) > 0 {
			file.Blocks[0].Functions = append(stdFuncs, file.Blocks[0].Functions...)
		}
		if len(stdRelations) > 0 {
			file.Blocks[0].Relations = append(stdRelations, file.Blocks[0].Relations...)
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
			for _, m := range cls.Methods {
				for _, p := range m.Params {
					collectTypeNames(&p.Type, names)
				}
				if m.ReturnType != nil {
					collectTypeNames(m.ReturnType, names)
				}
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

// primitiveTypes are built-in type names that should not trigger stdlib merging.
var primitiveTypes = map[string]bool{
	"string": true, "bool": true, "any": true, "error": true,
	"i8": true, "i16": true, "i32": true, "i64": true, "i128": true, "i256": true,
	"u8": true, "u16": true, "u32": true, "u64": true, "u128": true, "u256": true,
	"f32": true, "f64": true, "int": true, "uint": true, "byte": true, "rune": true,
}

// collectTypeNames recursively extracts NamedType names from a type expression.
// Excludes primitive types to avoid false-positive stdlib merging.
func collectTypeNames(te *TypeExpr, names map[string]bool) {
	if te == nil {
		return
	}
	switch te.Kind {
	case TypeNamed:
		if d, ok := te.Data.(NamedType); ok {
			if !primitiveTypes[d.Name] {
				names[d.Name] = true
			}
		} else if dp, ok := te.Data.(*NamedType); ok {
			if !primitiveTypes[dp.Name] {
				names[dp.Name] = true
			}
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
		for _, cls := range block.Classes {
			for _, m := range cls.Methods {
				if m.Body != nil {
					collectFuncCallNames(m.Body.Stmts, names)
				}
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
			if d.ElseBlock != nil {
				collectFuncCallNames(d.ElseBlock.Stmts, names)
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
			if d.LetValue != nil {
				collectFuncCallNamesExpr(d.LetValue, names)
			} else {
				collectFuncCallNamesExpr(&d.Condition, names)
			}
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
			collectFuncCallNamesExpr(&d.Target, names)
		}
	case StmtMatch:
		if d, ok := stmt.Data.(*MatchStmt); ok {
			collectFuncCallNamesExpr(&d.Value, names)
			for _, arm := range d.Arms {
				collectFuncCallNames(arm.Body.Stmts, names)
				if arm.Guard != nil {
					collectFuncCallNamesExpr(arm.Guard, names)
				}
			}
		}
	case StmtWhile:
		if d, ok := stmt.Data.(*WhileStmt); ok {
			collectFuncCallNamesExpr(&d.Condition, names)
			collectFuncCallNames(d.Body.Stmts, names)
		}
	case StmtBlock:
		if d, ok := stmt.Data.(*Block); ok {
			collectFuncCallNames(d.Stmts, names)
		}
	case StmtSpawn:
		if d, ok := stmt.Data.(*SpawnStmt); ok {
			collectFuncCallNames(d.Body.Stmts, names)
		}
	case StmtSelect:
		if d, ok := stmt.Data.(*SelectStmt); ok {
			for _, c := range d.Cases {
				if c.Expr != nil {
					collectFuncCallNamesExpr(c.Expr, names)
				}
				collectFuncCallNames(c.Body.Stmts, names)
			}
		}
	case StmtLock:
		if d, ok := stmt.Data.(*LockStmt); ok {
			collectFuncCallNamesExpr(&d.Mutex, names)
			collectFuncCallNames(d.Body.Stmts, names)
		}
	case StmtCascade:
		if d, ok := stmt.Data.(*CascadeStmt); ok {
			collectFuncCallNames(d.Body.Stmts, names)
		}
	case StmtYield:
		if d, ok := stmt.Data.(*YieldStmt); ok {
			if d.Value != nil {
				collectFuncCallNamesExpr(d.Value, names)
			}
		}
	case StmtBreak, StmtContinue:
		// no expressions to collect
	default:
		panic(fmt.Sprintf("collectFuncCallNamesStmt: unhandled StmtKind %d", stmt.Kind))
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
	case ExprStructLit:
		if d, ok := expr.Data.(*StructLitExpr); ok {
			for i := range d.Fields {
				collectFuncCallNamesExpr(&d.Fields[i].Value, names)
			}
		}
	case ExprMatch:
		if d, ok := expr.Data.(*MatchStmt); ok {
			collectFuncCallNamesExpr(&d.Value, names)
			for _, arm := range d.Arms {
				collectFuncCallNames(arm.Body.Stmts, names)
				if arm.Guard != nil {
					collectFuncCallNamesExpr(arm.Guard, names)
				}
			}
		}
	case ExprLambda:
		if d, ok := expr.Data.(*LambdaExpr); ok {
			collectFuncCallNames(d.Body.Stmts, names)
		}
	case ExprIndex:
		if d, ok := expr.Data.(*IndexExpr); ok {
			collectFuncCallNamesExpr(&d.Receiver, names)
			collectFuncCallNamesExpr(&d.Index, names)
		}
	case ExprSlice:
		if d, ok := expr.Data.(*SliceExpr); ok {
			collectFuncCallNamesExpr(&d.Receiver, names)
			if d.Low != nil {
				collectFuncCallNamesExpr(d.Low, names)
			}
			if d.High != nil {
				collectFuncCallNamesExpr(d.High, names)
			}
		}
	case ExprListLit:
		if d, ok := expr.Data.(*ListLitExpr); ok {
			for i := range d.Elems {
				collectFuncCallNamesExpr(&d.Elems[i], names)
			}
		}
	case ExprMapLit:
		if d, ok := expr.Data.(*MapLitExpr); ok {
			for i := range d.Entries {
				collectFuncCallNamesExpr(&d.Entries[i].Key, names)
				collectFuncCallNamesExpr(&d.Entries[i].Value, names)
			}
		}
	case ExprTupleLit:
		if d, ok := expr.Data.(*TupleLitExpr); ok {
			for i := range d.Elems {
				collectFuncCallNamesExpr(&d.Elems[i], names)
			}
		}
	case ExprCast:
		if d, ok := expr.Data.(*CastExpr); ok {
			collectFuncCallNamesExpr(&d.Operand, names)
		}
	case ExprUnwrap:
		if d, ok := expr.Data.(*UnwrapExpr); ok {
			collectFuncCallNamesExpr(&d.Operand, names)
		}
	case ExprTry:
		if d, ok := expr.Data.(*TryExpr); ok {
			collectFuncCallNamesExpr(&d.Operand, names)
		}
	case ExprIs:
		if d, ok := expr.Data.(*IsExpr); ok {
			collectFuncCallNamesExpr(&d.Operand, names)
		}
	case ExprIfElse:
		if d, ok := expr.Data.(*IfElseExpr); ok {
			collectFuncCallNamesExpr(&d.Cond, names)
			for i := range d.Then.Stmts {
				collectFuncCallNamesStmt(&d.Then.Stmts[i], names)
			}
			for i := range d.Else.Stmts {
				collectFuncCallNamesStmt(&d.Else.Stmts[i], names)
			}
			for i := range d.ElseIfs {
				collectFuncCallNamesExpr(&d.ElseIfs[i].Cond, names)
				for j := range d.ElseIfs[i].Body.Stmts {
					collectFuncCallNamesStmt(&d.ElseIfs[i].Body.Stmts[j], names)
				}
			}
		}
	case ExprStringInterp:
		if d, ok := expr.Data.(*StringInterpExpr); ok {
			for i := range d.Parts {
				collectFuncCallNamesExpr(&d.Parts[i], names)
			}
		}
	case ExprIdent, ExprIntLit, ExprFloatLit, ExprStringLit, ExprBoolLit, ExprNil:
		// no sub-expressions to collect
	default:
		panic(fmt.Sprintf("collectFuncCallNamesExpr: unhandled ExprKind %d", expr.Kind))
	}
}

// MergeFiles merges multiple parsed AST files into a single file.
// All blocks from subsequent files are appended to the first file's blocks.
// This enables multi-file compilation where cross-file references resolve correctly.
func MergeFiles(files []*File) *File {
	if len(files) == 0 {
		return &File{}
	}
	if len(files) == 1 {
		return files[0]
	}
	merged := &File{
		Filename: files[0].Filename,
		Blocks:   make([]ForgeBlock, 0),
		Span:     files[0].Span,
	}
	for _, f := range files {
		merged.Blocks = append(merged.Blocks, f.Blocks...)
		merged.Comments = append(merged.Comments, f.Comments...)
	}
	return merged
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
