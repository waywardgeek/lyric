// Package transpiler converts a type-checked Grok AST into Go source code.
package transpiler

import (
	"fmt"
	"strings"

	"github.com/waywardgeek/grok/pkg/ast"
)

// Transpiler converts Grok AST to Go source.
type Transpiler struct {
	buf    strings.Builder
	indent int
	pkg    string // Go package name
}

// New creates a transpiler targeting the given Go package name.
func New(pkg string) *Transpiler {
	return &Transpiler{pkg: pkg}
}

// Transpile converts an AST file to Go source code.
func (t *Transpiler) Transpile(file *ast.File) string {
	t.buf.Reset()
	t.writef("package %s\n", t.pkg)

	for _, block := range file.Blocks {
		t.transpileBlock(&block)
	}
	return t.buf.String()
}

func (t *Transpiler) transpileBlock(block *ast.GrokBlock) {
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
				t.writef("%s %s\n", exportName(fieldName), t.goType(&f.Type))
			}
			t.indent--
			t.writef("}\n")
		}
		t.writef("\nfunc (%s) is%s() {}\n", vName, name)
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
		t.writef("(%s *%s) ", recvName, receiver)
	}

	t.writef("%s(", exportName(fn.Name))

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
		t.transpileStmts(fn.Body.Stmts)
		t.indent--
		t.writeIndent()
		t.writef("}\n")
	} else {
		t.writef("\n")
	}
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
	t.writef("for _, %s := range ", forStmt.Var)
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
	t.writeIndent()
	t.writef("switch ")
	t.transpileExpr(&matchStmt.Value)
	t.writef(" {\n")
	for _, arm := range matchStmt.Arms {
		t.writeIndent()
		t.writef("case ")
		t.transpilePattern(&arm.Pattern)
		t.writef(":\n")
		t.indent++
		t.transpileStmts(arm.Body.Stmts)
		t.indent--
	}
	t.writeIndent()
	t.writef("}\n")
}

// --- Expressions ---

func (t *Transpiler) transpileExpr(expr *ast.Expr) {
	switch expr.Kind {
	case ast.ExprIntLit:
		lit := expr.Data.(*ast.IntLitExpr)
		t.writef("%s", lit.Value)
	case ast.ExprFloatLit:
		lit := expr.Data.(*ast.FloatLitExpr)
		t.writef("%s", lit.Value)
	case ast.ExprStringLit:
		lit := expr.Data.(*ast.StringLitExpr)
		t.writef("%q", lit.Value)
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
		t.writef("%s", id.Name)
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
		t.transpileExpr(&call.Func)
		t.writef("(")
		for i := range call.Args {
			if i > 0 {
				t.writef(", ")
			}
			t.transpileExpr(&call.Args[i])
		}
		t.writef(")")
	case ast.ExprMethodCall:
		mc := expr.Data.(*ast.MethodCallExpr)
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
	case ast.ExprListLit:
		lit := expr.Data.(*ast.ListLitExpr)
		// Without type context we emit []any{...}
		t.writef("[]any{")
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
		t.writef("map[any]any{")
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
			t.transpileStmts(lam.Body.Stmts)
		}
		t.indent--
		t.writeIndent()
		t.writef("}")
	case ast.ExprMatch:
		// match-as-expression is harder in Go; emit a comment
		t.writef("/* match expr */ nil")
	}
}

func (t *Transpiler) transpileBinary(expr *ast.Expr) {
	b := expr.Data.(*ast.BinaryExpr)
	t.transpileExpr(&b.Left)
	t.writef(" %s ", binaryOpString(b.Op))
	t.transpileExpr(&b.Right)
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
		if p.Constraint != "" {
			parts[i] = fmt.Sprintf("%s %s", p.Name, p.Constraint)
		} else {
			parts[i] = fmt.Sprintf("%s any", p.Name)
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
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
