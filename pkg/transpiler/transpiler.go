// Package transpiler converts a type-checked Grok AST into Go source code.
package transpiler

import (
	"fmt"
	"strings"

	"github.com/waywardgeek/grok/pkg/ast"
	"github.com/waywardgeek/grok/pkg/checker"
)

// variantCtorInfo holds info needed to transpile an enum variant constructor call.
type variantCtorInfo struct {
	enumName   string   // e.g., "Shape"
	fieldNames []string // e.g., ["radius"] or ["width", "height"]; positional if empty name
}

// Transpiler converts Grok AST to Go source.
type Transpiler struct {
	buf               strings.Builder
	indent            int
	pkg               string            // Go package name
	currentReturnType string            // Go return type of current function
	selfName          string            // receiver name to replace "self" with
	classes           map[string]bool   // tracks which types are classes (pointer receiver methods)
	topFuncs          map[string]bool   // tracks top-level function names
	autoImports       map[string]bool   // auto-detected import requirements (e.g., "fmt" for Sprintf)
	classTypeParams       map[string][]ast.TypeParam // class name → type params for receiver emission
	variantCtors      map[string]*variantCtorInfo // variant name → constructor info
	unitVariants      map[string]string           // variant name → enum name (for unit variants)
	classCtorFields   map[string][]string         // class name → constructor field names
}

// New creates a transpiler targeting the given Go package name.
func New(pkg string) *Transpiler {
	return &Transpiler{pkg: pkg, classes: make(map[string]bool), topFuncs: make(map[string]bool), autoImports: make(map[string]bool), variantCtors: make(map[string]*variantCtorInfo), unitVariants: make(map[string]string), classTypeParams: make(map[string][]ast.TypeParam), classCtorFields: make(map[string][]string)}
}

func (t *Transpiler) needsImport(pkg string) {
	t.autoImports[pkg] = true
}

// Transpile converts an AST file to Go source code.
func (t *Transpiler) Transpile(file *ast.File) string {
	t.buf.Reset()
	for k := range t.autoImports {
		delete(t.autoImports, k)
	}

	// Transpile body first (may discover auto-imports like fmt for f-strings)
	var bodyBuf strings.Builder
	for _, block := range file.Blocks {
		t.transpileBlock(&block)
	}
	bodyBuf.WriteString(t.buf.String())

	// Now build final output: package + imports + body
	t.buf.Reset()
	t.writef("package %s\n", t.pkg)

	// Collect all imports: user-declared + auto-detected
	importSet := make(map[string]string) // path → alias
	for _, block := range file.Blocks {
		for _, imp := range block.Imports {
			importSet[imp.Path] = imp.Alias
		}
	}
	for pkg := range t.autoImports {
		if _, exists := importSet[pkg]; !exists {
			importSet[pkg] = ""
		}
	}

	if len(importSet) > 0 {
		t.writef("\nimport (\n")
		t.indent++
		for path, alias := range importSet {
			t.writeIndent()
			if alias != "" && alias != path {
				t.writef("%s %q\n", alias, path)
			} else {
				t.writef("%q\n", path)
			}
		}
		t.indent--
		t.writef(")\n")
	}

	t.buf.WriteString(bodyBuf.String())
	return t.buf.String()
}

func (t *Transpiler) transpileBlock(block *ast.GrokBlock) {
	// Register class names and top-level function names
	for i := range block.Classes {
		t.classes[block.Classes[i].Name] = true
		// Register constructor parameter names for struct literal emission
		var ctorFields []string
		for _, p := range block.Classes[i].CtorParams {
			ctorFields = append(ctorFields, p.Name)
		}
		t.classCtorFields[block.Classes[i].Name] = ctorFields
	}
	for i := range block.Functions {
		t.topFuncs[block.Functions[i].Name] = true
	}
	for i := range block.Interfaces {
		t.writef("\n")
		t.transpileInterface(&block.Interfaces[i])
	}
	for i := range block.Structs {
		t.writef("\n")
		t.transpileStruct(&block.Structs[i])
	}
	for i := range block.Enums {
		t.writef("\n")
		t.transpileEnum(&block.Enums[i])
	}
	for i := range block.Classes {
		t.writef("\n")
		t.transpileClass(&block.Classes[i])
	}
	for i := range block.Functions {
		t.writef("\n")
		t.transpileFunc(&block.Functions[i], "")
	}
}

// --- Type transpilation ---

func (t *Transpiler) goType(te *ast.TypeExpr) string {
	if te == nil {
		return ""
	}
	switch te.Kind {
	case ast.TypeNamed:
		nt := te.Data.(ast.NamedType)
		return t.goNamedType(nt.Name, nt.Args)
	case ast.TypeOptional:
		ot := te.Data.(ast.OptionalType)
		inner := t.goType(&ot.Inner)
		return "*" + inner
	case ast.TypeSequence:
		st := te.Data.(ast.SequenceType)
		return "[]" + t.goType(&st.Elem)
	case ast.TypeMap:
		mt := te.Data.(ast.MapType)
		return fmt.Sprintf("map[%s]%s", t.goType(&mt.Key), t.goType(&mt.Value))
	case ast.TypeTuple:
		// Go doesn't have tuples — generate a struct or use multiple returns
		tt := te.Data.(ast.TupleType)
		if len(tt.Fields) == 0 {
			return "struct{}"
		}
		// For return types, caller handles multiple returns
		parts := make([]string, len(tt.Fields))
		for i, f := range tt.Fields {
			parts[i] = t.goType(&f.Type)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case ast.TypeFunc:
		ft := te.Data.(ast.FuncType)
		params := make([]string, len(ft.Params))
		for i := range ft.Params {
			params[i] = t.goType(&ft.Params[i])
		}
		ret := t.goType(&ft.Return)
		if ret == "" || ret == "struct{}" {
			return fmt.Sprintf("func(%s)", strings.Join(params, ", "))
		}
		return fmt.Sprintf("func(%s) %s", strings.Join(params, ", "), ret)
	case ast.TypeChannel:
		ct := te.Data.(ast.ChannelType)
		return fmt.Sprintf("chan %s", t.goType(&ct.Elem))
	case ast.TypeLock:
		return "sync.Mutex"
	case ast.TypeUnit:
		return ""
	default:
		return "any"
	}
}

func (t *Transpiler) goNamedType(name string, args []ast.TypeExpr) string {
	goName := grokPrimitiveToGo(name)
	if len(args) == 0 {
		return goName
	}
	// Generic: Type[A, B]
	typeArgs := make([]string, len(args))
	for i := range args {
		typeArgs[i] = t.goType(&args[i])
	}
	return fmt.Sprintf("%s[%s]", goName, strings.Join(typeArgs, ", "))
}

func grokPrimitiveToGo(name string) string {
	switch name {
	case "i8":
		return "int8"
	case "i16":
		return "int16"
	case "i32":
		return "int32"
	case "i64":
		return "int64"
	case "i128", "i256":
		return "int64" // Go doesn't have i128/i256; needs big.Int in practice
	case "u8":
		return "uint8"
	case "u16":
		return "uint16"
	case "u32":
		return "uint32"
	case "u64":
		return "uint64"
	case "u128", "u256":
		return "uint64"
	case "f32":
		return "float32"
	case "f64":
		return "float64"
	case "f128":
		return "float64" // Go doesn't have f128
	case "int":
		return "int"
	case "uint":
		return "uint"
	case "float":
		return "float64"
	case "bool":
		return "bool"
	case "string":
		return "string"
	case "any":
		return "any"
	default:
		return name // user-defined type
	}
}

// --- Declarations ---

func (t *Transpiler) transpileInterface(iface *ast.InterfaceDecl) {
	name := exportName(iface.Name)
	typeParams := t.typeParamList(iface.TypeParams)

	t.writef("type %s%s interface {\n", name, typeParams)
	t.indent++

	// Embed composed interfaces
	for _, parent := range iface.Implements {
		t.writeIndent()
		t.writef("%s\n", exportName(parent))
	}

	// Method signatures
	for _, m := range iface.Methods {
		t.writeIndent()
		t.writef("%s(", exportName(m.Name))
		first := true
		for _, p := range m.Params {
			if p.IsSelf {
				continue
			}
			if !first {
				t.writef(", ")
			}
			t.writef("%s %s", p.Name, t.goType(&p.Type))
			first = false
		}
		t.writef(")")
		if m.ReturnType != nil {
			t.writef(" %s", t.goType(m.ReturnType))
		}
		t.writef("\n")
	}

	t.indent--
	t.writef("}\n")
}

func (t *Transpiler) transpileStruct(s *ast.StructDecl) {
	name := exportName(s.Name)
	typeParams := t.typeParamList(s.TypeParams)
	t.writef("type %s%s struct {\n", name, typeParams)
	t.indent++
	for _, f := range s.Fields {
		t.writeIndent()
		t.writef("%s %s\n", exportName(f.Name), t.goType(&f.Type))
	}
	t.indent--
	t.writef("}\n")
}

func (t *Transpiler) transpileClass(cls *ast.ClassDecl) {
	name := exportName(cls.Name)
	typeParams := t.typeParamList(cls.TypeParams)
	if len(cls.TypeParams) > 0 {
		t.classTypeParams[name] = cls.TypeParams
	}

	// Struct with fields (including ctor params as fields)
	t.writef("type %s%s struct {\n", name, typeParams)
	t.indent++
	for _, p := range cls.CtorParams {
		t.writeIndent()
		t.writef("%s %s\n", exportName(p.Name), t.goType(&p.Type))
	}
	for _, f := range cls.Fields {
		t.writeIndent()
		t.writef("%s %s\n", exportName(f.Name), t.goType(&f.Type))
	}
	t.indent--
	t.writef("}\n")

	// Constructor
	if len(cls.CtorParams) > 0 {
		t.writef("\nfunc New%s(", name)
		for i, p := range cls.CtorParams {
			if i > 0 {
				t.writef(", ")
			}
			t.writef("%s %s", p.Name, t.goType(&p.Type))
		}
		t.writef(") *%s {\n", name)
		t.indent++
		t.writeIndent()
		t.writef("return &%s{\n", name)
		t.indent++
		for _, p := range cls.CtorParams {
			t.writeIndent()
			t.writef("%s: %s,\n", exportName(p.Name), p.Name)
		}
		t.indent--
		t.writeIndent()
		t.writef("}\n")
		t.indent--
		t.writef("}\n")
	}

	// Methods
	for i := range cls.Methods {
		t.writef("\n")
		t.transpileFunc(&cls.Methods[i], name)
	}
}

func (t *Transpiler) transpileEnum(e *ast.EnumDecl) {
	name := exportName(e.Name)

	// Interface for the enum
	t.writef("type %s interface {\n", name)
	t.indent++
	t.writeIndent()
	t.writef("is%s()\n", name)
	t.indent--
	t.writef("}\n")

	// Variant structs
	for _, v := range e.Variants {
		vName := fmt.Sprintf("%s%s", name, exportName(v.Name))
		var fieldNames []string
		if len(v.Fields) == 0 {
			t.writef("\ntype %s struct{}\n", vName)
		} else {
			t.writef("\ntype %s struct {\n", vName)
			t.indent++
			for i, f := range v.Fields {
				t.writeIndent()
				fieldName := f.Name
				if fieldName == "" {
					fieldName = fmt.Sprintf("V%d", i)
				}
				fieldNames = append(fieldNames, fieldName)
				t.writef("%s %s\n", exportName(fieldName), t.goType(&f.Type))
			}
			t.indent--
			t.writef("}\n")
		}
		t.writef("\nfunc (%s) is%s() {}\n", vName, name)

		// Register variant constructor info for call transpilation
		if len(v.Fields) > 0 {
			t.variantCtors[v.Name] = &variantCtorInfo{
				enumName:   e.Name,
				fieldNames: fieldNames,
			}
		} else {
			t.unitVariants[v.Name] = e.Name
		}
	}
}

func (t *Transpiler) transpileFunc(fn *ast.FuncDecl, receiver string) {
	t.writeIndent()
	t.writef("func ")

	// Receiver
	if receiver != "" {
		// Find if any param is self/mut self
		recvName := "r"
		for _, p := range fn.Params {
			if p.IsSelf {
				if p.Name != "" && p.Name != "self" {
					recvName = p.Name
				}
				break
			}
		}
		t.writef("(%s *%s", recvName, receiver)
		// Add type params for generic classes
		if tps, ok := t.classTypeParams[receiver]; ok && len(tps) > 0 {
			t.writef("[")
			for i, tp := range tps {
				if i > 0 {
					t.writef(", ")
				}
				t.writef("%s", tp.Name)
			}
			t.writef("]")
		}
		t.writef(") ")
		t.selfName = recvName
	} else {
		t.selfName = ""
	}

	// Special-case: Main -> main for Go entry point
	funcName := exportName(fn.Name)
	if (fn.Name == "Main" || fn.Name == "main") && t.pkg == "main" {
		funcName = "main"
	}
	t.writef("%s", funcName)

	// Type parameters
	typeParams := t.typeParamList(fn.TypeParams)
	t.writef("%s(", typeParams)

	// Parameters (skip self)
	first := true
	for _, p := range fn.Params {
		if p.IsSelf {
			continue
		}
		if !first {
			t.writef(", ")
		}
		t.writef("%s %s", p.Name, t.goType(&p.Type))
		first = false
	}
	t.writef(")")

	// Return type
	if fn.ReturnType != nil {
		ret := t.goType(fn.ReturnType)
		if ret != "" {
			t.writef(" %s", ret)
		}
	}

	// Body
	if fn.Body != nil {
		t.writef(" {\n")
		t.indent++
		prevReturn := t.currentReturnType
		if fn.ReturnType != nil {
			t.currentReturnType = t.goType(fn.ReturnType)
		} else {
			t.currentReturnType = ""
		}
		t.transpileStmts(fn.Body.Stmts)
		t.currentReturnType = prevReturn
		t.indent--
		t.writeIndent()
		t.writef("}\n")
	} else {
		t.writef("\n")
	}
}

// --- Built-in Methods ---

// transpileBuiltinMethod handles method calls on built-in types (string, list, map).
// Returns true if handled, false if the caller should use default method call emission.
func (t *Transpiler) transpileBuiltinMethod(mc *ast.MethodCallExpr) bool {
	ct, ok := mc.Receiver.ResolvedType.(*checker.Type)
	if !ok || ct == nil {
		return false
	}
	switch ct.Kind {
	case checker.TyString:
		return t.transpileStringMethod(mc)
	case checker.TyList:
		return t.transpileListMethod(mc)
	case checker.TyMap:
		return t.transpileMapMethod(mc)
	}
	return false
}

func (t *Transpiler) transpileStringMethod(mc *ast.MethodCallExpr) bool {
	switch mc.Method {
	case "len":
		t.writef("len(")
		t.transpileExpr(&mc.Receiver)
		t.writef(")")
	case "contains":
		t.autoImports["strings"] = true
		t.writef("strings.Contains(")
		t.transpileExpr(&mc.Receiver)
		t.writef(", ")
		t.transpileExpr(&mc.Args[0])
		t.writef(")")
	case "has_prefix":
		t.autoImports["strings"] = true
		t.writef("strings.HasPrefix(")
		t.transpileExpr(&mc.Receiver)
		t.writef(", ")
		t.transpileExpr(&mc.Args[0])
		t.writef(")")
	case "has_suffix":
		t.autoImports["strings"] = true
		t.writef("strings.HasSuffix(")
		t.transpileExpr(&mc.Receiver)
		t.writef(", ")
		t.transpileExpr(&mc.Args[0])
		t.writef(")")
	case "to_upper":
		t.autoImports["strings"] = true
		t.writef("strings.ToUpper(")
		t.transpileExpr(&mc.Receiver)
		t.writef(")")
	case "to_lower":
		t.autoImports["strings"] = true
		t.writef("strings.ToLower(")
		t.transpileExpr(&mc.Receiver)
		t.writef(")")
	case "trim":
		t.autoImports["strings"] = true
		t.writef("strings.TrimSpace(")
		t.transpileExpr(&mc.Receiver)
		t.writef(")")
	case "trim_left":
		t.autoImports["strings"] = true
		t.writef("strings.TrimLeft(")
		t.transpileExpr(&mc.Receiver)
		t.writef(`, " \t\n\r")`)
	case "trim_right":
		t.autoImports["strings"] = true
		t.writef("strings.TrimRight(")
		t.transpileExpr(&mc.Receiver)
		t.writef(`, " \t\n\r")`)
	case "replace":
		t.autoImports["strings"] = true
		t.writef("strings.ReplaceAll(")
		t.transpileExpr(&mc.Receiver)
		t.writef(", ")
		t.transpileExpr(&mc.Args[0])
		t.writef(", ")
		t.transpileExpr(&mc.Args[1])
		t.writef(")")
	case "split":
		t.autoImports["strings"] = true
		t.writef("strings.Split(")
		t.transpileExpr(&mc.Receiver)
		t.writef(", ")
		t.transpileExpr(&mc.Args[0])
		t.writef(")")
	case "index_of":
		t.autoImports["strings"] = true
		t.writef("strings.Index(")
		t.transpileExpr(&mc.Receiver)
		t.writef(", ")
		t.transpileExpr(&mc.Args[0])
		t.writef(")")
	case "repeat":
		t.autoImports["strings"] = true
		t.writef("strings.Repeat(")
		t.transpileExpr(&mc.Receiver)
		t.writef(", int(")
		t.transpileExpr(&mc.Args[0])
		t.writef("))")
	default:
		return false
	}
	return true
}

func (t *Transpiler) transpileListMethod(mc *ast.MethodCallExpr) bool {
	switch mc.Method {
	case "len":
		t.writef("len(")
		t.transpileExpr(&mc.Receiver)
		t.writef(")")
	case "push":
		// list.push(x) → list = append(list, x)
		t.transpileExpr(&mc.Receiver)
		t.writef(" = append(")
		t.transpileExpr(&mc.Receiver)
		t.writef(", ")
		t.transpileExpr(&mc.Args[0])
		t.writef(")")
	case "pop":
		// list.pop() → IIFE that returns last element and shrinks the list
		// func() T { _v := xs[len(xs)-1]; xs = xs[:len(xs)-1]; return _v }()
		retType := "any"
		if ct, ok := mc.Receiver.ResolvedType.(*checker.Type); ok && ct != nil && ct.Elem != nil {
			retType = checkerTypeToGo(ct.Elem)
		}
		t.writef("func() %s { _v := ", retType)
		t.transpileExpr(&mc.Receiver)
		t.writef("[len(")
		t.transpileExpr(&mc.Receiver)
		t.writef(")-1]; ")
		t.transpileExpr(&mc.Receiver)
		t.writef(" = ")
		t.transpileExpr(&mc.Receiver)
		t.writef("[:len(")
		t.transpileExpr(&mc.Receiver)
		t.writef(")-1]; return _v }()")
	case "contains":
		t.autoImports["slices"] = true
		t.writef("slices.Contains(")
		t.transpileExpr(&mc.Receiver)
		t.writef(", ")
		t.transpileExpr(&mc.Args[0])
		t.writef(")")
	case "reverse":
		t.autoImports["slices"] = true
		t.writef("slices.Reverse(")
		t.transpileExpr(&mc.Receiver)
		t.writef(")")
	case "join":
		t.autoImports["strings"] = true
		t.writef("strings.Join(")
		t.transpileExpr(&mc.Receiver)
		t.writef(", ")
		t.transpileExpr(&mc.Args[0])
		t.writef(")")
	default:
		return false
	}
	return true
}

func (t *Transpiler) transpileMapMethod(mc *ast.MethodCallExpr) bool {
	ct, _ := mc.Receiver.ResolvedType.(*checker.Type)
	keyGoType := "any"
	valGoType := "any"
	if ct != nil {
		if ct.Key != nil {
			keyGoType = checkerTypeToGo(ct.Key)
		}
		if ct.Val != nil {
			valGoType = checkerTypeToGo(ct.Val)
		}
	}
	switch mc.Method {
	case "len":
		t.writef("len(")
		t.transpileExpr(&mc.Receiver)
		t.writef(")")
	case "contains_key":
		t.writef("func() bool { _, _ok := ")
		t.transpileExpr(&mc.Receiver)
		t.writef("[")
		t.transpileExpr(&mc.Args[0])
		t.writef("]; return _ok }()")
	case "keys":
		t.writef("func() []%s { var _keys []%s; for _k := range ", keyGoType, keyGoType)
		t.transpileExpr(&mc.Receiver)
		t.writef(" { _keys = append(_keys, _k) }; return _keys }()")
	case "values":
		t.writef("func() []%s { var _vals []%s; for _, _v := range ", valGoType, valGoType)
		t.transpileExpr(&mc.Receiver)
		t.writef(" { _vals = append(_vals, _v) }; return _vals }()")
	default:
		return false
	}
	return true
}


// --- Statements ---

func (t *Transpiler) transpileStmts(stmts []ast.Stmt) {
	for i := range stmts {
		t.transpileStmt(&stmts[i])
	}
}

func (t *Transpiler) transpileStmt(stmt *ast.Stmt) {
	switch stmt.Kind {
	case ast.StmtVarDecl:
		t.transpileVarDecl(stmt)
	case ast.StmtAssign:
		t.transpileAssign(stmt)
	case ast.StmtReturn:
		t.transpileReturn(stmt)
	case ast.StmtExpr:
		es := stmt.Data.(*ast.ExprStmt)
		t.writeIndent()
		t.transpileExpr(&es.Expr)
		t.writef("\n")
	case ast.StmtIf:
		t.transpileIf(stmt)
	case ast.StmtFor:
		t.transpileFor(stmt)
	case ast.StmtWhile:
		t.transpileWhile(stmt)
	case ast.StmtMatch:
		t.transpileMatch(stmt)
	case ast.StmtBlock:
		blk := stmt.Data.(*ast.Block)
		t.writeIndent()
		t.writef("{\n")
		t.indent++
		t.transpileStmts(blk.Stmts)
		t.indent--
		t.writeIndent()
		t.writef("}\n")
	case ast.StmtCascade:
		cs := stmt.Data.(*ast.CascadeStmt)
		// cascade → defer func() { ... }()
		t.writeIndent()
		t.writef("defer func() {\n")
		t.indent++
		t.transpileStmts(cs.Body.Stmts)
		t.indent--
		t.writeIndent()
		t.writef("}()\n")
	case ast.StmtBreak:
		t.writeIndent()
		t.writef("break\n")
	case ast.StmtContinue:
		t.writeIndent()
		t.writef("continue\n")
	}
}

func (t *Transpiler) transpileVarDecl(stmt *ast.Stmt) {
	decl := stmt.Data.(*ast.VarDeclStmt)
	t.writeIndent()

	// Tuple destructuring: let (a, b) = expr → a, b := expr
	if len(decl.Names) > 0 {
		t.writef("%s := ", strings.Join(decl.Names, ", "))
		t.transpileExpr(decl.Value)
		t.writef("\n")
		return
	}

	if decl.Value != nil {
		if decl.Type != nil {
			t.writef("var %s %s = ", decl.Name, t.goType(decl.Type))
			t.transpileExpr(decl.Value)
		} else if decl.Name == "_" {
			t.writef("_ = ")
			t.transpileExpr(decl.Value)
		} else {
			t.writef("%s := ", decl.Name)
			t.transpileExpr(decl.Value)
		}
	} else if decl.Type != nil {
		t.writef("var %s %s", decl.Name, t.goType(decl.Type))
	}
	t.writef("\n")
}

func (t *Transpiler) transpileAssign(stmt *ast.Stmt) {
	assign := stmt.Data.(*ast.AssignStmt)
	t.writeIndent()
	t.transpileExpr(&assign.Target)
	t.writef(" = ")
	t.transpileExpr(&assign.Value)
	t.writef("\n")
}

func (t *Transpiler) transpileReturn(stmt *ast.Stmt) {
	ret := stmt.Data.(*ast.ReturnStmt)
	t.writeIndent()
	if ret.Value != nil {
		t.writef("return ")
		// Handle optional wrapping: if return type is *T and value is T, wrap with pointer
		if t.currentReturnType != "" && ret.Value.Kind != ast.ExprTupleLit && ret.Value.Kind != ast.ExprNil {
			if rt, ok := ret.Value.ResolvedType.(*checker.Type); ok {
				exprGoType := checkerTypeToGo(rt)
				if exprGoType != "" && exprGoType != t.currentReturnType {
					// Check if it's an optional wrapping case: *T vs T
					if strings.HasPrefix(t.currentReturnType, "*") && t.currentReturnType[1:] == exprGoType {
						// Emit: func() *T { v := expr; return &v }()
						t.writef("func() %s { _v := ", t.currentReturnType)
						t.transpileExpr(ret.Value)
						t.writef("; return &_v }()")
						t.writef("\n")
						return
					}
					t.writef("%s(", t.currentReturnType)
					t.transpileExpr(ret.Value)
					t.writef(")")
					t.writef("\n")
					return
				}
			}
		}
		t.transpileExpr(ret.Value)
		t.writef("\n")
	} else {
		t.writef("return\n")
	}
}

func (t *Transpiler) transpileIf(stmt *ast.Stmt) {
	ifStmt := stmt.Data.(*ast.IfStmt)
	t.writeIndent()
	t.writef("if ")
	t.transpileExpr(&ifStmt.Condition)
	t.writef(" {\n")
	t.indent++
	t.transpileStmts(ifStmt.Then.Stmts)
	t.indent--
	for _, elif := range ifStmt.ElseIfs {
		t.writeIndent()
		t.writef("} else if ")
		t.transpileExpr(&elif.Condition)
		t.writef(" {\n")
		t.indent++
		t.transpileStmts(elif.Body.Stmts)
		t.indent--
	}
	if ifStmt.Else != nil {
		t.writeIndent()
		t.writef("} else {\n")
		t.indent++
		t.transpileStmts(ifStmt.Else.Stmts)
		t.indent--
	}
	t.writeIndent()
	t.writef("}\n")
}

func (t *Transpiler) transpileFor(stmt *ast.Stmt) {
	forStmt := stmt.Data.(*ast.ForStmt)
	t.writeIndent()
	if forStmt.IndexVar != "" {
		t.writef("for %s, %s := range ", forStmt.IndexVar, forStmt.Var)
	} else {
		t.writef("for _, %s := range ", forStmt.Var)
	}
	t.transpileExpr(&forStmt.Collection)
	t.writef(" {\n")
	t.indent++
	t.transpileStmts(forStmt.Body.Stmts)
	t.indent--
	t.writeIndent()
	t.writef("}\n")
}

func (t *Transpiler) transpileWhile(stmt *ast.Stmt) {
	whileStmt := stmt.Data.(*ast.WhileStmt)
	t.writeIndent()
	t.writef("for ")
	t.transpileExpr(&whileStmt.Condition)
	t.writef(" {\n")
	t.indent++
	t.transpileStmts(whileStmt.Body.Stmts)
	t.indent--
	t.writeIndent()
	t.writef("}\n")
}

func (t *Transpiler) transpileMatch(stmt *ast.Stmt) {
	matchStmt := stmt.Data.(*ast.MatchStmt)
	isEnumMatch := t.isEnumMatch(&matchStmt.Value)
	t.writeIndent()
	if isEnumMatch {
		t.writef("switch _m := ")
		t.transpileExpr(&matchStmt.Value)
		t.writef(".(type) {\n")
	} else {
		t.writef("switch ")
		t.transpileExpr(&matchStmt.Value)
		t.writef(" {\n")
	}
	for _, arm := range matchStmt.Arms {
		t.writeIndent()
		if arm.Pattern.Kind == ast.PatWildcard {
			t.writef("default:\n")
		} else if arm.Pattern.Kind == ast.PatVariant && isEnumMatch {
			vp := arm.Pattern.Data.(*ast.VariantPattern)
			t.writef("case %s:\n", t.variantGoType(vp.Name))
			t.indent++
			t.emitVariantBindings(vp)
			t.transpileStmts(arm.Body.Stmts)
			t.indent--
			continue
		} else if arm.Pattern.Kind == ast.PatIdent && isEnumMatch {
			id := arm.Pattern.Data.(*ast.IdentPattern)
			if goType := t.variantGoType(id.Name); goType != exportName(id.Name) {
				t.writef("case %s:\n", goType)
			} else {
				t.writef("case %s:\n", id.Name)
			}
		} else {
			t.writef("case ")
			t.transpilePattern(&arm.Pattern)
			t.writef(":\n")
		}
		t.indent++
		t.transpileStmts(arm.Body.Stmts)
		t.indent--
	}
	t.writeIndent()
	t.writef("}\n")
}

// isEnumMatch checks if the match value expression has an enum resolved type.
func (t *Transpiler) isEnumMatch(expr *ast.Expr) bool {
	if rt, ok := expr.ResolvedType.(*checker.Type); ok {
		return rt.Kind == checker.TyEnum
	}
	return false
}

// variantGoType returns the Go type name for a variant (e.g., "ShapeCircle").
func (t *Transpiler) variantGoType(variantName string) string {
	if vci, ok := t.variantCtors[variantName]; ok {
		return exportName(vci.enumName) + exportName(variantName)
	}
	if enumName, ok := t.unitVariants[variantName]; ok {
		return exportName(enumName) + exportName(variantName)
	}
	return exportName(variantName)
}

// emitVariantBindings emits field extraction from a type-switch matched variant.
func (t *Transpiler) emitVariantBindings(vp *ast.VariantPattern) {
	vci, ok := t.variantCtors[vp.Name]
	if !ok || len(vp.Bindings) == 0 {
		return
	}
	for i, binding := range vp.Bindings {
		if binding.Kind != ast.PatIdent {
			continue
		}
		id := binding.Data.(*ast.IdentPattern)
		if id.Name == "_" {
			continue
		}
		fieldName := fmt.Sprintf("V%d", i)
		if i < len(vci.fieldNames) {
			fieldName = vci.fieldNames[i]
		}
		t.writeIndent()
		t.writef("%s := _m.%s\n", id.Name, exportName(fieldName))
	}
}

// --- Expressions ---

func (t *Transpiler) transpileExpr(expr *ast.Expr) {
	switch expr.Kind {
	case ast.ExprIntLit:
		lit := expr.Data.(*ast.IntLitExpr)
		// If the checker resolved this to a specific int type, emit a cast
		if rt, ok := expr.ResolvedType.(*checker.Type); ok && rt.Kind == checker.TyInt {
			goType := checkerTypeToGo(rt)
			if goType != "" && goType != "int" {
				t.writef("%s(%s)", goType, lit.Value)
			} else {
				t.writef("%s", lit.Value)
			}
		} else {
			t.writef("%s", lit.Value)
		}
	case ast.ExprFloatLit:
		lit := expr.Data.(*ast.FloatLitExpr)
		// If the checker resolved this to a specific float type, emit a cast
		if rt, ok := expr.ResolvedType.(*checker.Type); ok && rt.Kind == checker.TyFloat {
			goType := checkerTypeToGo(rt)
			if goType != "" && goType != "float64" {
				t.writef("%s(%s)", goType, lit.Value)
			} else {
				t.writef("%s", lit.Value)
			}
		} else {
			t.writef("%s", lit.Value)
		}
	case ast.ExprStringLit:
		lit := expr.Data.(*ast.StringLitExpr)
		t.writef("%q", lit.Value)
	case ast.ExprStringInterp:
		interp := expr.Data.(*ast.StringInterpExpr)
		// Build fmt.Sprintf call: format string + args
		var fmtStr strings.Builder
		var args []ast.Expr
		for i, part := range interp.Parts {
			if i%2 == 0 {
				// String literal part
				lit := part.Data.(*ast.StringLitExpr)
				// Escape % in format string
				fmtStr.WriteString(strings.ReplaceAll(lit.Value, "%", "%%"))
			} else {
				// Expression part — add %v placeholder
				fmtStr.WriteString("%v")
				args = append(args, part)
			}
		}
		if len(args) == 0 {
			// No interpolation — just a regular string
			t.writef("%q", fmtStr.String())
		} else {
			t.needsImport("fmt")
			t.writef("fmt.Sprintf(%q", fmtStr.String())
			for i := range args {
				t.writef(", ")
				t.transpileExpr(&args[i])
			}
			t.writef(")")
		}
	case ast.ExprBoolLit:
		lit := expr.Data.(*ast.BoolLitExpr)
		if lit.Value {
			t.writef("true")
		} else {
			t.writef("false")
		}
	case ast.ExprNil:
		t.writef("nil")
	case ast.ExprIdent:
		id := expr.Data.(*ast.IdentExpr)
		name := id.Name
		if name == "self" && t.selfName != "" {
			name = t.selfName
		} else if enumName, ok := t.unitVariants[name]; ok {
			// Unit enum variant: emit as struct literal
			name = fmt.Sprintf("%s%s{}", exportName(enumName), exportName(name))
		} else if t.topFuncs[name] {
			// Top-level function reference (not a call) — export the name
			name = exportName(name)
		}
		t.writef("%s", name)
	case ast.ExprUnary:
		u := expr.Data.(*ast.UnaryExpr)
		switch u.Op {
		case ast.OpNeg:
			t.writef("-")
		case ast.OpNot:
			t.writef("!")
		}
		t.transpileExpr(&u.Operand)
	case ast.ExprBinary:
		t.transpileBinary(expr)
	case ast.ExprCall:
		call := expr.Data.(*ast.CallExpr)
		// Check if this is an enum variant constructor
		if call.Func.Kind == ast.ExprIdent {
			id := call.Func.Data.(*ast.IdentExpr)
			if vci, ok := t.variantCtors[id.Name]; ok {
				// Emit as struct literal: EnumVariant{Field: val, ...}
				goName := fmt.Sprintf("%s%s", exportName(vci.enumName), exportName(id.Name))
				t.writef("%s{", goName)
				for i := range call.Args {
					if i > 0 {
						t.writef(", ")
					}
					fieldName := fmt.Sprintf("V%d", i)
					if i < len(vci.fieldNames) {
						fieldName = vci.fieldNames[i]
					}
					t.writef("%s: ", exportName(fieldName))
					t.transpileExpr(&call.Args[i])
				}
				t.writef("}")
				break
			}
			// Check if this is a class constructor: ClassName<T>() → &ClassName[T]{}
			if t.classes[id.Name] {
				t.writef("&%s", exportName(id.Name))
				if len(call.TypeArgs) > 0 {
					t.writef("[")
					for i := range call.TypeArgs {
						if i > 0 {
							t.writef(", ")
						}
						t.writef("%s", t.goType(&call.TypeArgs[i]))
					}
					t.writef("]")
				}
				t.writef("{")
				// Map positional args to constructor fields
				if ci, ok := t.classCtorFields[id.Name]; ok {
					for i := range call.Args {
						if i > 0 {
							t.writef(", ")
						}
						fieldName := fmt.Sprintf("V%d", i)
						if i < len(ci) {
							fieldName = ci[i]
						}
						t.writef("%s: ", exportName(fieldName))
						t.transpileExpr(&call.Args[i])
					}
				} else {
					for i := range call.Args {
						if i > 0 {
							t.writef(", ")
						}
						t.transpileExpr(&call.Args[i])
					}
				}
				t.writef("}")
				break
			}
		}
		// Handle builtins and top-level function names
		callDone := false
		if call.Func.Kind == ast.ExprIdent {
			id := call.Func.Data.(*ast.IdentExpr)
			switch id.Name {
			case "println":
				t.needsImport("fmt")
				t.writef("fmt.Println")
			case "print":
				t.needsImport("fmt")
				t.writef("fmt.Print")
			case "len":
				t.writef("len")
			case "append":
				t.writef("append")
			case "isnull":
				// isnull(expr) → (expr == nil)
				t.writef("(")
				if len(call.Args) > 0 {
					t.transpileExpr(&call.Args[0])
				}
				t.writef(" == nil)")
				callDone = true
			default:
				if t.topFuncs[id.Name] {
					if (id.Name == "Main" || id.Name == "main") && t.pkg == "main" {
						t.writef("main")
					} else {
						t.writef("%s", exportName(id.Name))
					}
				} else {
					t.writef("%s", id.Name)
				}
			}
		} else {
			t.transpileExpr(&call.Func)
		}
		if !callDone {
			// Emit type arguments: func[T, U](...)
			if len(call.TypeArgs) > 0 {
			t.writef("[")
			for i := range call.TypeArgs {
				if i > 0 {
					t.writef(", ")
				}
				t.writef("%s", t.goType(&call.TypeArgs[i]))
			}
			t.writef("]")
		}
		t.writef("(")
		for i := range call.Args {
			if i > 0 {
				t.writef(", ")
			}
			t.transpileExpr(&call.Args[i])
		}
		t.writef(")")
		}
	case ast.ExprMethodCall:
		mc := expr.Data.(*ast.MethodCallExpr)
		if t.transpileBuiltinMethod(mc) {
			break
		}
		t.transpileExpr(&mc.Receiver)
		t.writef(".%s(", exportName(mc.Method))
		for i := range mc.Args {
			if i > 0 {
				t.writef(", ")
			}
			t.transpileExpr(&mc.Args[i])
		}
		t.writef(")")
	case ast.ExprFieldAccess:
		fa := expr.Data.(*ast.FieldAccessExpr)
		t.transpileExpr(&fa.Receiver)
		t.writef(".%s", exportName(fa.Field))
	case ast.ExprIndex:
		idx := expr.Data.(*ast.IndexExpr)
		t.transpileExpr(&idx.Receiver)
		t.writef("[")
		t.transpileExpr(&idx.Index)
		t.writef("]")
	case ast.ExprSlice:
		sl := expr.Data.(*ast.SliceExpr)
		t.transpileExpr(&sl.Receiver)
		t.writef("[")
		if sl.Low != nil {
			t.transpileExpr(sl.Low)
		}
		t.writef(":")
		if sl.High != nil {
			t.transpileExpr(sl.High)
		}
		t.writef("]")
	case ast.ExprListLit:
		lit := expr.Data.(*ast.ListLitExpr)
		// Use resolved type from checker if available
		typeStr := "[]any"
		if rt, ok := expr.ResolvedType.(*checker.Type); ok && rt.Kind == checker.TyList {
			typeStr = checkerTypeToGo(rt)
		}
		t.writef("%s{", typeStr)
		for i := range lit.Elems {
			if i > 0 {
				t.writef(", ")
			}
			t.transpileExpr(&lit.Elems[i])
		}
		t.writef("}")
	case ast.ExprTupleLit:
		// Go doesn't have tuples — emit as struct literal or just group
		lit := expr.Data.(*ast.TupleLitExpr)
		// For now, emit as a function-style grouping (works for returns)
		for i := range lit.Elems {
			if i > 0 {
				t.writef(", ")
			}
			t.transpileExpr(&lit.Elems[i])
		}
	case ast.ExprMapLit:
		lit := expr.Data.(*ast.MapLitExpr)
		typeStr := "map[any]any"
		if rt, ok := expr.ResolvedType.(*checker.Type); ok && rt.Kind == checker.TyMap {
			typeStr = checkerTypeToGo(rt)
		}
		t.writef("%s{", typeStr)
		for i, e := range lit.Entries {
			if i > 0 {
				t.writef(", ")
			}
			t.transpileExpr(&e.Key)
			t.writef(": ")
			t.transpileExpr(&e.Value)
		}
		t.writef("}")
	case ast.ExprStructLit:
		sl := expr.Data.(*ast.StructLitExpr)
		// Classes use pointer receivers, so emit &ClassName{...}
		if t.classes[sl.TypeName] {
			t.writef("&")
		}
		t.writef("%s{", exportName(sl.TypeName))
		for i, f := range sl.Fields {
			if i > 0 {
				t.writef(", ")
			}
			t.writef("%s: ", exportName(f.Name))
			t.transpileExpr(&f.Value)
		}
		t.writef("}")
	case ast.ExprLambda:
		lam := expr.Data.(*ast.LambdaExpr)
		t.writef("func(")
		for i, p := range lam.Params {
			if i > 0 {
				t.writef(", ")
			}
			t.writef("%s %s", p.Name, t.goType(&p.Type))
		}
		t.writef(")")
		if lam.ReturnType != nil {
			ret := t.goType(lam.ReturnType)
			if ret != "" {
				t.writef(" %s", ret)
			}
		}
		t.writef(" {\n")
		t.indent++
		if lam.Body != nil {
			stmts := lam.Body.Stmts
			// If lambda has a return type and last stmt is an expr stmt,
			// emit it as a return statement
			if lam.ReturnType != nil && len(stmts) > 0 {
				last := stmts[len(stmts)-1]
				if last.Kind == ast.StmtExpr {
					t.transpileStmts(stmts[:len(stmts)-1])
					t.writeIndent()
					t.writef("return ")
					t.transpileExpr(&last.Data.(*ast.ExprStmt).Expr)
					t.writef("\n")
				} else {
					t.transpileStmts(stmts)
				}
			} else {
				t.transpileStmts(stmts)
			}
		}
		t.indent--
		t.writeIndent()
		t.writef("}")
	case ast.ExprMatch:
		// Match-as-expression in Go via IIFE: func() T { switch v { case ... } }()
		m := expr.Data.(*ast.MatchStmt)
		retType := "any"
		if rt, ok := expr.ResolvedType.(*checker.Type); ok {
			if gt := checkerTypeToGo(rt); gt != "" {
				retType = gt
			}
		}
		isEnumMatch := t.isEnumMatch(&m.Value)
		t.writef("func() %s {\n", retType)
		t.indent++
		t.writeIndent()
		if isEnumMatch {
			t.writef("switch _m := ")
			t.transpileExpr(&m.Value)
			t.writef(".(type) {\n")
		} else {
			t.writef("switch _m := ")
			t.transpileExpr(&m.Value)
			t.writef("; _m {\n")
		}
		for _, arm := range m.Arms {
			t.writeIndent()
			if arm.Pattern.Kind == ast.PatWildcard {
				t.writef("default:\n")
			} else if arm.Pattern.Kind == ast.PatVariant && isEnumMatch {
				vp := arm.Pattern.Data.(*ast.VariantPattern)
				t.writef("case %s:\n", t.variantGoType(vp.Name))
				t.indent++
				t.emitVariantBindings(vp)
			} else if arm.Pattern.Kind == ast.PatIdent && isEnumMatch {
				id := arm.Pattern.Data.(*ast.IdentPattern)
				if goType := t.variantGoType(id.Name); goType != exportName(id.Name) {
					t.writef("case %s:\n", goType)
				} else {
					t.writef("case %s:\n", id.Name)
				}
				t.indent++
			} else {
				t.writef("case ")
				t.transpilePattern(&arm.Pattern)
				t.writef(":\n")
				t.indent++
			}
			// Last statement in arm body is the result — emit as return
			if len(arm.Body.Stmts) > 0 {
				for i := 0; i < len(arm.Body.Stmts)-1; i++ {
					t.transpileStmt(&arm.Body.Stmts[i])
				}
				last := &arm.Body.Stmts[len(arm.Body.Stmts)-1]
				if last.Kind == ast.StmtExpr {
					t.writeIndent()
					t.writef("return ")
					exprStmt := last.Data.(*ast.ExprStmt)
					t.transpileExpr(&exprStmt.Expr)
					t.writef("\n")
				} else {
					t.transpileStmt(last)
				}
			}
			t.indent--
		}
		// Add fallback default only if no wildcard arm exists
		hasDefault := false
		for _, arm := range m.Arms {
			if arm.Pattern.Kind == ast.PatWildcard {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			t.writeIndent()
			t.writef("default:\n")
			t.indent++
			t.writeIndent()
			// Use typed zero value for non-any return types
			if retType != "any" {
				t.writef("var _zero %s\n", retType)
				t.writeIndent()
				t.writef("return _zero\n")
			} else {
				t.writef("return nil\n")
			}
			t.indent--
		}
		t.writeIndent()
		t.writef("}\n")
		t.indent--
		t.writeIndent()
		t.writef("}()")
	case ast.ExprCast:
		cast := expr.Data.(*ast.CastExpr)
		goType := t.goType(&cast.TargetType)
		t.writef("%s(", goType)
		t.transpileExpr(&cast.Operand)
		t.writef(")")
	case ast.ExprUnwrap:
		// expr! → dereference pointer (panic if nil via Go's nil pointer deref)
		// Emit: func() T { if _v := expr; _v != nil { return *_v }; panic("unwrap of nil optional") }()
		unwrap := expr.Data.(*ast.UnwrapExpr)
		// Simple form: just dereference with *
		t.writef("*")
		t.transpileExpr(&unwrap.Operand)
	}
}

func (t *Transpiler) transpileBinary(expr *ast.Expr) {
	b := expr.Data.(*ast.BinaryExpr)
	// If the binary expression has a resolved type and operands have different
	// resolved types, insert explicit Go casts to the wider type.
	var targetGoType string
	if expr.ResolvedType != nil {
		if rt, ok := expr.ResolvedType.(*checker.Type); ok && rt.IsNumeric() {
			targetGoType = checkerTypeToGo(rt)
		}
	}
	t.transpileExprWithCast(&b.Left, targetGoType)
	t.writef(" %s ", binaryOpString(b.Op))
	t.transpileExprWithCast(&b.Right, targetGoType)
}

// transpileExprWithCast emits an expression, wrapping it in a Go type cast if
// its resolved type differs from the target type.
func (t *Transpiler) transpileExprWithCast(expr *ast.Expr, targetGoType string) {
	if targetGoType == "" || expr.ResolvedType == nil {
		t.transpileExpr(expr)
		return
	}
	if rt, ok := expr.ResolvedType.(*checker.Type); ok {
		exprGoType := checkerTypeToGo(rt)
		if exprGoType != targetGoType && exprGoType != "" {
			t.writef("%s(", targetGoType)
			t.transpileExpr(expr)
			t.writef(")")
			return
		}
	}
	t.transpileExpr(expr)
}

// checkerTypeToGo converts a checker.Type to a Go type string.
func checkerTypeToGo(ct *checker.Type) string {
	switch ct.Kind {
	case checker.TyInt:
		if ct.Bits == -1 {
			return "int"
		}
		return fmt.Sprintf("int%d", ct.Bits)
	case checker.TyUint:
		if ct.Bits == -1 {
			return "uint"
		}
		return fmt.Sprintf("uint%d", ct.Bits)
	case checker.TyFloat:
		return fmt.Sprintf("float%d", ct.Bits)
	case checker.TyBool:
		return "bool"
	case checker.TyString:
		return "string"
	case checker.TyList:
		if ct.Elem != nil {
			return "[]" + checkerTypeToGo(ct.Elem)
		}
		return "[]any"
	case checker.TyMap:
		kt, vt := "any", "any"
		if ct.Key != nil {
			kt = checkerTypeToGo(ct.Key)
		}
		if ct.Val != nil {
			vt = checkerTypeToGo(ct.Val)
		}
		return "map[" + kt + "]" + vt
	case checker.TyOptional:
		if ct.Elem != nil {
			return "*" + checkerTypeToGo(ct.Elem)
		}
		return "*any"
	case checker.TyTuple:
		parts := make([]string, len(ct.Fields))
		for i, f := range ct.Fields {
			parts[i] = checkerTypeToGo(f.Type)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case checker.TyInterface:
		if ct.Name != "" {
			return ct.Name
		}
		return "interface{}"
	case checker.TyClass, checker.TyStruct:
		if ct.Name != "" {
			return ct.Name
		}
		return "any"
	case checker.TyVar:
		if ct.Name != "" {
			return ct.Name
		}
		return "any"
	default:
		return "any"
	}
}

func binaryOpString(op ast.BinaryOp) string {
	switch op {
	case ast.OpAdd:
		return "+"
	case ast.OpSub:
		return "-"
	case ast.OpMul:
		return "*"
	case ast.OpDiv:
		return "/"
	case ast.OpMod:
		return "%"
	case ast.OpEq:
		return "=="
	case ast.OpNeq:
		return "!="
	case ast.OpLt:
		return "<"
	case ast.OpLe:
		return "<="
	case ast.OpGt:
		return ">"
	case ast.OpGe:
		return ">="
	case ast.OpAnd:
		return "&&"
	case ast.OpOr:
		return "||"
	case ast.OpBitAnd:
		return "&"
	case ast.OpBitOr:
		return "|"
	case ast.OpBitXor:
		return "^"
	case ast.OpShl:
		return "<<"
	case ast.OpShr:
		return ">>"
	default:
		return "?"
	}
}

// --- Patterns ---

func (t *Transpiler) transpilePattern(pat *ast.Pattern) {
	switch pat.Kind {
	case ast.PatIdent:
		id := pat.Data.(*ast.IdentPattern)
		t.writef("%s", id.Name)
	case ast.PatLiteral:
		lit := pat.Data.(*ast.LiteralPattern)
		t.transpileExpr(&lit.Expr)
	case ast.PatWildcard:
		t.writef("default")
	case ast.PatVariant:
		vp := pat.Data.(*ast.VariantPattern)
		t.writef("%s", vp.Name)
	case ast.PatTuple:
		tp := pat.Data.(*ast.TuplePattern)
		for i := range tp.Elems {
			if i > 0 {
				t.writef(", ")
			}
			t.transpilePattern(&tp.Elems[i])
		}
	}
}

// --- Helpers ---

func (t *Transpiler) writef(format string, args ...any) {
	fmt.Fprintf(&t.buf, format, args...)
}

func (t *Transpiler) writeIndent() {
	for i := 0; i < t.indent; i++ {
		t.buf.WriteString("\t")
	}
}

func (t *Transpiler) typeParamList(params []ast.TypeParam) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, len(params))
	for i, p := range params {
		constraint := t.mapConstraint(p.Constraint)
		if constraint != "" {
			parts[i] = fmt.Sprintf("%s %s", p.Name, constraint)
		} else {
			parts[i] = fmt.Sprintf("%s any", p.Name)
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// mapConstraint maps Grok constraint names to Go equivalents.
func (t *Transpiler) mapConstraint(name string) string {
	switch name {
	case "":
		return ""
	case "Comparable":
		t.needsImport("cmp")
		return "cmp.Ordered"
	case "Equatable", "Hashable":
		return "comparable"
	default:
		return name // pass through user-defined constraints
	}
}

// exportName capitalizes the first letter for Go export.
func exportName(name string) string {
	if name == "" {
		return ""
	}
	// Already capitalized
	if name[0] >= 'A' && name[0] <= 'Z' {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
