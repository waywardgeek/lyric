package transpiler

import (
	"strings"
	"testing"

	"github.com/waywardgeek/grok/pkg/ast"
	"github.com/waywardgeek/grok/pkg/checker"
	"github.com/waywardgeek/grok/pkg/parser"
)

// transpileWithChecker parses, type-checks, and transpiles Grok source.
func transpileWithChecker(t *testing.T, src string) string {
	t.Helper()
	file, err := parser.ParseString(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	c := checker.New()
	c.CheckFile(file)
	if errs := c.Errors(); len(errs) > 0 {
		t.Fatalf("checker errors: %v", errs)
	}
	tr := New("main")
	return tr.Transpile(file)
}

func TestTranspileStruct(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Structs: []ast.StructDecl{{
				Name: "Point",
				Fields: []ast.Field{
					{Name: "X", Type: namedType("f64")},
					{Name: "Y", Type: namedType("f64")},
				},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "type Point struct {")
	assertContains(t, got, "X float64")
	assertContains(t, got, "Y float64")
}

func TestTranspileEnum(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Enums: []ast.EnumDecl{{
				Name: "Option",
				Variants: []ast.EnumVariant{
					{Name: "Some", Fields: []ast.TupleField{{Type: namedType("string")}}},
					{Name: "None"},
				},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "type Option interface {")
	assertContains(t, got, "isOption()")
	assertContains(t, got, "type OptionSome struct {")
	assertContains(t, got, "type OptionNone struct{}")
	assertContains(t, got, "func (OptionSome) isOption() {}")
	assertContains(t, got, "func (OptionNone) isOption() {}")
}

func TestTranspileClass(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Classes: []ast.ClassDecl{{
				Name: "Stack",
				CtorParams: []ast.Param{
					{Name: "capacity", Type: namedType("i32")},
				},
				Fields: []ast.Field{
					{Name: "Items", Type: seqType("string")},
				},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "type Stack struct {")
	assertContains(t, got, "Capacity int32")
	assertContains(t, got, "Items []string")
	assertContains(t, got, "func NewStack(capacity int32) *Stack {")
}

func TestTranspileFunction(t *testing.T) {
	retType := namedType("i32")
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Functions: []ast.FuncDecl{{
				Name: "Add",
				Params: []ast.Param{
					{Name: "a", Type: namedType("i32")},
					{Name: "b", Type: namedType("i32")},
				},
				ReturnType: &retType,
				Body: &ast.Block{
					Stmts: []ast.Stmt{
						{Kind: ast.StmtReturn, Data: &ast.ReturnStmt{
							Value: &ast.Expr{Kind: ast.ExprBinary, Data: &ast.BinaryExpr{
								Left:  ast.Expr{Kind: ast.ExprIdent, Data: &ast.IdentExpr{Name: "a"}},
								Op:    ast.OpAdd,
								Right: ast.Expr{Kind: ast.ExprIdent, Data: &ast.IdentExpr{Name: "b"}},
							}},
						}},
					},
				},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "func Add(a int32, b int32) int32 {")
	assertContains(t, got, "return a + b")
}

func TestTranspileVarDecl(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Functions: []ast.FuncDecl{{
				Name: "Test",
				Body: &ast.Block{
					Stmts: []ast.Stmt{
						{Kind: ast.StmtVarDecl, Data: &ast.VarDeclStmt{
							Name:  "x",
							Value: &ast.Expr{Kind: ast.ExprIntLit, Data: &ast.IntLitExpr{Value: "42"}},
						}},
						{Kind: ast.StmtVarDecl, Data: &ast.VarDeclStmt{
							Name: "y",
							Type: &ast.TypeExpr{Kind: ast.TypeNamed, Data: ast.NamedType{Name: "string"}},
						}},
					},
				},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "x := 42")
	assertContains(t, got, "var y string")
}

func TestTranspileIfElse(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Functions: []ast.FuncDecl{{
				Name: "Test",
				Body: &ast.Block{
					Stmts: []ast.Stmt{
						{Kind: ast.StmtIf, Data: &ast.IfStmt{
							Condition: ast.Expr{Kind: ast.ExprBoolLit, Data: &ast.BoolLitExpr{Value: true}},
							Then:      ast.Block{Stmts: []ast.Stmt{{Kind: ast.StmtBreak}}},
							Else:      &ast.Block{Stmts: []ast.Stmt{{Kind: ast.StmtContinue}}},
						}},
					},
				},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "if true {")
	assertContains(t, got, "break")
	assertContains(t, got, "} else {")
	assertContains(t, got, "continue")
}

func TestTranspileForLoop(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Functions: []ast.FuncDecl{{
				Name: "Test",
				Body: &ast.Block{
					Stmts: []ast.Stmt{
						{Kind: ast.StmtFor, Data: &ast.ForStmt{
							Var:        "item",
							Collection: ast.Expr{Kind: ast.ExprIdent, Data: &ast.IdentExpr{Name: "items"}},
							Body:       ast.Block{},
						}},
					},
				},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "for _, item := range items {")
}

func TestTranspileWhile(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Functions: []ast.FuncDecl{{
				Name: "Test",
				Body: &ast.Block{
					Stmts: []ast.Stmt{
						{Kind: ast.StmtWhile, Data: &ast.WhileStmt{
							Condition: ast.Expr{Kind: ast.ExprBoolLit, Data: &ast.BoolLitExpr{Value: true}},
							Body:      ast.Block{},
						}},
					},
				},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "for true {")
}

func TestTranspileCascade(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Functions: []ast.FuncDecl{{
				Name: "Test",
				Body: &ast.Block{
					Stmts: []ast.Stmt{
						{Kind: ast.StmtCascade, Data: &ast.CascadeStmt{
							Body: ast.Block{Stmts: []ast.Stmt{{Kind: ast.StmtBreak}}},
						}},
					},
				},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "defer func() {")
}

func TestTranspileOptionalType(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Structs: []ast.StructDecl{{
				Name: "Foo",
				Fields: []ast.Field{
					{Name: "Val", Type: ast.TypeExpr{Kind: ast.TypeOptional, Data: ast.OptionalType{Inner: namedType("string")}}},
				},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "Val *string")
}

func TestTranspileMethodCall(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Functions: []ast.FuncDecl{{
				Name: "Test",
				Body: &ast.Block{
					Stmts: []ast.Stmt{
						{Kind: ast.StmtExpr, Data: &ast.ExprStmt{
							Expr: ast.Expr{Kind: ast.ExprMethodCall, Data: &ast.MethodCallExpr{
								Receiver: ast.Expr{Kind: ast.ExprIdent, Data: &ast.IdentExpr{Name: "s"}},
								Method:   "Push",
								Args: []ast.Expr{
									{Kind: ast.ExprIntLit, Data: &ast.IntLitExpr{Value: "5"}},
								},
							}},
						}},
					},
				},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "s.Push(5)")
}

func TestExportName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"foo", "Foo"},
		{"Foo", "Foo"},
		{"x", "X"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := exportName(tc.in); got != tc.want {
			t.Errorf("exportName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Helpers

func namedType(name string) ast.TypeExpr {
	return ast.TypeExpr{Kind: ast.TypeNamed, Data: ast.NamedType{Name: name}}
}

func seqType(elem string) ast.TypeExpr {
	return ast.TypeExpr{Kind: ast.TypeSequence, Data: ast.SequenceType{Elem: namedType(elem)}}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("output does not contain %q\n\ngot:\n%s", want, got)
	}
}

func TestTranspileMatchExpr(t *testing.T) {
	matchExpr := ast.Expr{
		Kind: ast.ExprMatch,
		Data: &ast.MatchStmt{
			Value: ast.Expr{Kind: ast.ExprIdent, Data: &ast.IdentExpr{Name: "x"}},
			Arms: []ast.MatchArm{
				{
					Pattern: ast.Pattern{Kind: ast.PatLiteral, Data: &ast.LiteralPattern{
						Expr: ast.Expr{Kind: ast.ExprIntLit, Data: &ast.IntLitExpr{Value: "1"}},
					}},
					Body: ast.Block{Stmts: []ast.Stmt{{
						Kind: ast.StmtExpr,
						Data: &ast.ExprStmt{Expr: ast.Expr{Kind: ast.ExprIntLit, Data: &ast.IntLitExpr{Value: "10"}}},
					}}},
				},
				{
					Pattern: ast.Pattern{Kind: ast.PatWildcard},
					Body: ast.Block{Stmts: []ast.Stmt{{
						Kind: ast.StmtExpr,
						Data: &ast.ExprStmt{Expr: ast.Expr{Kind: ast.ExprIntLit, Data: &ast.IntLitExpr{Value: "0"}}},
					}}},
				},
			},
		},
	}
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Functions: []ast.FuncDecl{{
				Name: "f",
				Body: &ast.Block{Stmts: []ast.Stmt{
					{
						Kind: ast.StmtVarDecl,
						Data: &ast.VarDeclStmt{Name: "result", Value: &matchExpr},
					},
				}},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "func() any {")
	assertContains(t, got, "switch _m :=")
	assertContains(t, got, "return 10")
	assertContains(t, got, "return 0")
	assertContains(t, got, "}()")
}

func TestSelfToReceiver(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Classes: []ast.ClassDecl{{
				Name: "Counter",
				CtorParams: []ast.Param{
					{Name: "count", Type: namedType("i32")},
				},
				Methods: []ast.FuncDecl{{
					Name: "Get",
					Params: []ast.Param{
						{Name: "self", IsSelf: true},
					},
					ReturnType: func() *ast.TypeExpr { t := namedType("i32"); return &t }(),
					Body: &ast.Block{Stmts: []ast.Stmt{
						{
							Kind: ast.StmtReturn,
							Data: &ast.ReturnStmt{Value: &ast.Expr{
								Kind: ast.ExprFieldAccess,
								Data: &ast.FieldAccessExpr{
									Receiver: ast.Expr{Kind: ast.ExprIdent, Data: &ast.IdentExpr{Name: "self"}},
									Field:    "count",
								},
							}},
						},
					}},
				}},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "r.Count")
	if strings.Contains(got, "self.") {
		t.Errorf("output still contains 'self.': %s", got)
	}
}

func TestTypedListLiteral(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Functions: []ast.FuncDecl{{
				Name: "Main",
				Body: &ast.Block{Stmts: []ast.Stmt{
					{
						Kind: ast.StmtVarDecl,
						Data: &ast.VarDeclStmt{
							Name: "nums",
							Value: &ast.Expr{
								Kind: ast.ExprListLit,
								Data: &ast.ListLitExpr{
									Elems: []ast.Expr{
										{Kind: ast.ExprIntLit, Data: &ast.IntLitExpr{Value: "1"}},
										{Kind: ast.ExprIntLit, Data: &ast.IntLitExpr{Value: "2"}},
									},
								},
							},
						},
					},
				}},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "[]any{1, 2}")
}

func TestTranspileInterface(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Interfaces: []ast.InterfaceDecl{{
				Name: "Greeter",
				Methods: []ast.FuncDecl{
					{
						Name:       "Greet",
						Params:     []ast.Param{{Name: "self", IsSelf: true}},
						ReturnType: &ast.TypeExpr{Kind: ast.TypeNamed, Data: ast.NamedType{Name: "string"}},
					},
				},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "type Greeter interface {")
	assertContains(t, got, "Greet() string")
}

func TestTranspileInterfaceComposition(t *testing.T) {
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Interfaces: []ast.InterfaceDecl{{
				Name: "ReadWriter",
				Implements: []string{"Reader", "Writer"},
				Methods: []ast.FuncDecl{
					{
						Name:   "Close",
						Params: []ast.Param{{Name: "self", IsSelf: true}},
					},
				},
			}},
		}},
	}
	tr := New("main")
	got := tr.Transpile(file)
	assertContains(t, got, "type ReadWriter interface {")
	assertContains(t, got, "Reader")
	assertContains(t, got, "Writer")
	assertContains(t, got, "Close()")
}

func TestTranspileEnumVariantConstructor(t *testing.T) {
	src := `grok test {
  enum Shape {
    Circle(radius: f64)
    Empty
  }
  func main() {
    let c = Circle(5.0)
    let e = Empty
    let _ = c
    let _ = e
  }
}`
	output := transpileWithChecker(t, src)
	if !strings.Contains(output, "ShapeCircle{Radius: 5.0}") {
		t.Errorf("expected variant constructor struct literal, got:\n%s", output)
	}
	if !strings.Contains(output, "ShapeEmpty{}") {
		t.Errorf("expected unit variant struct literal, got:\n%s", output)
	}
}

func TestTranspileEnumMatchTypeSwitch(t *testing.T) {
	src := `grok test {
  enum Shape {
    Circle(radius: f64)
    Empty
  }
  func Area(s: Shape) -> f64 {
    return match s {
      Circle(r) => { r }
      Empty => { 0.0 }
    }
  }
}`
	output := transpileWithChecker(t, src)
	if !strings.Contains(output, ".(type)") {
		t.Errorf("expected type switch, got:\n%s", output)
	}
	if !strings.Contains(output, "case ShapeCircle:") {
		t.Errorf("expected ShapeCircle case, got:\n%s", output)
	}
	if !strings.Contains(output, "r := _m.Radius") {
		t.Errorf("expected field binding, got:\n%s", output)
	}
	if !strings.Contains(output, "case ShapeEmpty:") {
		t.Errorf("expected ShapeEmpty case, got:\n%s", output)
	}
}

func TestTupleReturn(t *testing.T) {
	src := `grok test {
		func divide(a: i32, b: i32) -> (i32, error) {
			return (a / b, nil)
		}
	}`
	got := transpileWithChecker(t, src)
	if !strings.Contains(got, "func Divide(a int32, b int32) (int32, error)") {
		t.Errorf("expected tuple return type signature, got:\n%s", got)
	}
	if !strings.Contains(got, "return a / b, nil") {
		t.Errorf("expected multi-value return, got:\n%s", got)
	}
}

func TestTupleDestructuring(t *testing.T) {
	src := `grok test {
		func getTwo() -> (i32, string) {
			return (42, "hello")
		}
		func main() {
			let (x, y) = getTwo()
		}
	}`
	got := transpileWithChecker(t, src)
	if !strings.Contains(got, "x, y := GetTwo()") {
		t.Errorf("expected tuple destructuring, got:\n%s", got)
	}
}

func TestTranspileGenericFunc(t *testing.T) {
	src := `grok test {
		func identity<T>(x: T) -> T {
			return x
		}
		func main() {
			let a = identity<i32>(42)
		}
	}`
	out := transpileWithChecker(t, src)
	if !strings.Contains(out, "func Identity[T any](x T) T") {
		t.Errorf("expected generic func signature, got:\n%s", out)
	}
	if !strings.Contains(out, "Identity[int32](") {
		t.Errorf("expected type arg at call site, got:\n%s", out)
	}
}

func TestTranspileGenericFuncWithConstraint(t *testing.T) {
	src := `grok test {
		func max<T: Comparable>(a: T, b: T) -> T {
			if a > b {
				return a
			}
			return b
		}
	}`
	out := transpileWithChecker(t, src)
	if !strings.Contains(out, "func Max[T cmp.Ordered](a T, b T) T") {
		t.Errorf("expected constraint mapping, got:\n%s", out)
	}
}

func TestTranspileUnwrap(t *testing.T) {
	src := `grok test {
		func f(x: i32?) -> i32 {
			return x!
		}
	}`
	out := transpileWithChecker(t, src)
	if !strings.Contains(out, "*x") {
		t.Errorf("expected *x for unwrap, got:\n%s", out)
	}
}

func TestTranspileIsnull(t *testing.T) {
	src := `grok test {
		func f(x: i32?) -> bool {
			if isnull(x) {
				return true
			}
			return false
		}
	}`
	out := transpileWithChecker(t, src)
	if !strings.Contains(out, "(x == nil)") {
		t.Errorf("expected (x == nil) for isnull, got:\n%s", out)
	}
}

func TestTranspileLenAppend(t *testing.T) {
	src := `grok test {
		func f() {
			let xs = [1, 2, 3]
			let n = len(xs)
			let ys = append(xs, 4)
		}
	}`
	out := transpileWithChecker(t, src)
	if !strings.Contains(out, "len(xs)") {
		t.Errorf("expected len(xs), got:\n%s", out)
	}
	if !strings.Contains(out, "append(xs, int32(4))") {
		t.Errorf("expected append(xs, int32(4)), got:\n%s", out)
	}
}


func TestTranspileStringMethods(t *testing.T) {
	out := transpileWithChecker(t, `grok test {
		func main() {
			let s = "hello"
			println(s.to_upper())
			println(s.contains("x"))
			let parts = s.split(",")
			println(s.replace("l", "r"))
		}
	}`)
	if !strings.Contains(out, "strings.ToUpper(s)") {
		t.Error("expected strings.ToUpper(s)")
	}
	if !strings.Contains(out, `strings.Contains(s, "x")`) {
		t.Error("expected strings.Contains")
	}
	if !strings.Contains(out, `strings.Split(s, ",")`) {
		t.Error("expected strings.Split")
	}
	if !strings.Contains(out, `strings.ReplaceAll(s, "l", "r")`) {
		t.Error("expected strings.ReplaceAll")
	}
}

func TestTranspileListMethods(t *testing.T) {
	out := transpileWithChecker(t, `grok test {
		func main() {
			let mut xs: [i32] = []
			xs.push(10)
			println(xs.len())
		}
	}`)
	if !strings.Contains(out, "xs = append(xs") {
		t.Error("expected append pattern for push")
	}
	if !strings.Contains(out, "len(xs)") {
		t.Error("expected len(xs)")
	}
}

func TestTranspileMapMethods(t *testing.T) {
	out := transpileWithChecker(t, `grok test {
		func main() {
			let m = map[string]i32{"a": 1}
			println(m.contains_key("a"))
		}
	}`)
	if !strings.Contains(out, `_ok := m["a"]`) {
		t.Error("expected contains_key IIFE pattern")
	}
}

func TestTranspileUnionMatch(t *testing.T) {
	src := `grok test {
		func f(val: string | i32) {
			match val {
				string => { println("str") }
				i32 => { println("int") }
			}
		}
	}`
	out := transpileWithChecker(t, src)
	if !strings.Contains(out, "val.(type)") {
		t.Error("expected type switch, got:", out)
	}
	if !strings.Contains(out, "case string:") {
		t.Error("expected case string:, got:", out)
	}
	if !strings.Contains(out, "case int32:") {
		t.Error("expected case int32:, got:", out)
	}
}

func TestTranspileUnionVarDecl(t *testing.T) {
	src := `grok test {
		func f() {
			let x: string | i32 = 42
		}
	}`
	out := transpileWithChecker(t, src)
	if !strings.Contains(out, "int32(42)") {
		t.Error("expected int32(42) cast for union assignment, got:", out)
	}
}
