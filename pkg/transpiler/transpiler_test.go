package transpiler

import (
	"strings"
	"testing"

	"github.com/waywardgeek/grok/pkg/ast"
)

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
