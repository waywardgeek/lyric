package lir

import (
	"testing"

	"github.com/waywardgeek/grok/pkg/ast"
)

func TestLowerEmptyProgram(t *testing.T) {
	l := NewLowerer()
	file := &ast.File{
		Blocks: []ast.GrokBlock{{Name: "main"}},
	}
	prog := l.Lower(file)
	if prog.Package != "main" {
		t.Errorf("expected package 'main', got %q", prog.Package)
	}
	if len(prog.Functions) != 0 {
		t.Errorf("expected 0 functions, got %d", len(prog.Functions))
	}
}

func TestLowerStructDecl(t *testing.T) {
	l := NewLowerer()
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Name: "test",
			Structs: []ast.StructDecl{{
				Name:     "Point",
				IsPublic: true,
				Fields: []ast.Field{
					{Name: "X", Type: ast.TypeExpr{Kind: ast.TypeNamed, Data: &ast.NamedType{Name: "f64"}}},
					{Name: "Y", Type: ast.TypeExpr{Kind: ast.TypeNamed, Data: &ast.NamedType{Name: "f64"}}},
				},
			}},
		}},
	}
	prog := l.Lower(file)
	if len(prog.Structs) != 1 {
		t.Fatalf("expected 1 struct, got %d", len(prog.Structs))
	}
	s := prog.Structs[0]
	if s.Name != "Point" {
		t.Errorf("expected name 'Point', got %q", s.Name)
	}
	if !s.IsExported {
		t.Error("expected exported")
	}
	if len(s.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(s.Fields))
	}
	if s.Fields[0].Type.Kind != LTyF64 {
		t.Errorf("expected f64, got kind %d", s.Fields[0].Type.Kind)
	}
}

func TestLowerEnumDecl(t *testing.T) {
	l := NewLowerer()
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Name: "test",
			Enums: []ast.EnumDecl{{
				Name:     "Color",
				IsPublic: true,
				Variants: []ast.EnumVariant{
					{Name: "Red"},
					{Name: "Green"},
					{Name: "Blue"},
				},
			}},
		}},
	}
	prog := l.Lower(file)
	if len(prog.Enums) != 1 {
		t.Fatalf("expected 1 enum, got %d", len(prog.Enums))
	}
	e := prog.Enums[0]
	if len(e.Variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(e.Variants))
	}
	if e.Variants[0].Tag != 0 || e.Variants[1].Tag != 1 || e.Variants[2].Tag != 2 {
		t.Error("variant tags not sequential")
	}
}

func TestLowerSimpleFunction(t *testing.T) {
	l := NewLowerer()
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Name: "test",
			Functions: []ast.FuncDecl{{
				Name:     "add",
				IsPublic: true,
				Params: []ast.Param{
					{Name: "a", Type: ast.TypeExpr{Kind: ast.TypeNamed, Data: &ast.NamedType{Name: "i32"}}},
					{Name: "b", Type: ast.TypeExpr{Kind: ast.TypeNamed, Data: &ast.NamedType{Name: "i32"}}},
				},
				ReturnType: &ast.TypeExpr{Kind: ast.TypeNamed, Data: &ast.NamedType{Name: "i32"}},
				Body: &ast.Block{
					Stmts: []ast.Stmt{{
						Kind: ast.StmtReturn,
						Data: &ast.ReturnStmt{
							Value: &ast.Expr{
								Kind: ast.ExprBinary,
								Data: &ast.BinaryExpr{
									Left:  ast.Expr{Kind: ast.ExprIdent, Data: &ast.IdentExpr{Name: "a"}},
									Op:    ast.OpAdd,
									Right: ast.Expr{Kind: ast.ExprIdent, Data: &ast.IdentExpr{Name: "b"}},
								},
							},
						},
					}},
				},
			}},
		}},
	}
	prog := l.Lower(file)
	if len(prog.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(prog.Functions))
	}
	fn := prog.Functions[0]
	if fn.Name != "add" {
		t.Errorf("expected name 'add', got %q", fn.Name)
	}
	if len(fn.Params) != 2 {
		t.Errorf("expected 2 params, got %d", len(fn.Params))
	}
	if fn.ReturnType.Kind != LTyI32 {
		t.Errorf("expected i32 return, got kind %d", fn.ReturnType.Kind)
	}
	// Body: should have a TempDef (binary op) + Return
	if len(fn.Body) < 2 {
		t.Fatalf("expected at least 2 statements in body, got %d", len(fn.Body))
	}
	if fn.Body[0].Kind != LStmtTempDef {
		t.Errorf("expected TempDef, got kind %d", fn.Body[0].Kind)
	}
	if fn.Body[1].Kind != LStmtReturn {
		t.Errorf("expected Return, got kind %d", fn.Body[1].Kind)
	}
}

func TestLowerFlatExpressions(t *testing.T) {
	// a + b * c should produce: %0 = b * c; %1 = a + %0
	l := NewLowerer()
	expr := &ast.Expr{
		Kind: ast.ExprBinary,
		Data: &ast.BinaryExpr{
			Left: ast.Expr{Kind: ast.ExprIdent, Data: &ast.IdentExpr{Name: "a"}},
			Op:   ast.OpAdd,
			Right: ast.Expr{
				Kind: ast.ExprBinary,
				Data: &ast.BinaryExpr{
					Left:  ast.Expr{Kind: ast.ExprIdent, Data: &ast.IdentExpr{Name: "b"}},
					Op:    ast.OpMul,
					Right: ast.Expr{Kind: ast.ExprIdent, Data: &ast.IdentExpr{Name: "c"}},
				},
			},
		},
	}
	l.stmts = nil
	result := l.lowerExpr(expr)

	// Should have 2 TempDefs: %0 = b*c, %1 = a + %0
	if len(l.stmts) != 2 {
		t.Fatalf("expected 2 temps, got %d", len(l.stmts))
	}
	if l.stmts[0].Kind != LStmtTempDef {
		t.Errorf("stmt 0: expected TempDef, got %d", l.stmts[0].Kind)
	}
	if l.stmts[1].Kind != LStmtTempDef {
		t.Errorf("stmt 1: expected TempDef, got %d", l.stmts[1].Kind)
	}

	// Result should be temp %1
	if result.Kind != LValTemp || result.TempID != 1 {
		t.Errorf("expected temp %%1, got kind=%d id=%d", result.Kind, result.TempID)
	}

	// First temp: b * c
	td0 := l.stmts[0].Data.(*LTempDef)
	binop0 := td0.Expr.Data.(*LBinOpData)
	if binop0.Op != LBinMul {
		t.Errorf("expected Mul, got %d", binop0.Op)
	}

	// Second temp: a + %0
	td1 := l.stmts[1].Data.(*LTempDef)
	binop1 := td1.Expr.Data.(*LBinOpData)
	if binop1.Op != LBinAdd {
		t.Errorf("expected Add, got %d", binop1.Op)
	}
	if binop1.Right.Kind != LValTemp || binop1.Right.TempID != 0 {
		t.Errorf("expected right operand to be temp %%0")
	}
}

func TestLowerWhileCondBlock(t *testing.T) {
	// while x > 0 { ... } should produce CondBlock with temps + CondVar
	l := NewLowerer()
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Name: "test",
			Functions: []ast.FuncDecl{{
				Name: "loop",
				Body: &ast.Block{
					Stmts: []ast.Stmt{{
						Kind: ast.StmtWhile,
						Data: &ast.WhileStmt{
							Condition: ast.Expr{
								Kind: ast.ExprBinary,
								Data: &ast.BinaryExpr{
									Left: ast.Expr{Kind: ast.ExprIdent, Data: &ast.IdentExpr{Name: "x"}},
									Op:   ast.OpGt,
									Right: ast.Expr{
										Kind: ast.ExprIntLit,
										Data: &ast.IntLitExpr{Value: "0"},
									},
								},
							},
							Body: ast.Block{
								Stmts: []ast.Stmt{{
									Kind: ast.StmtBreak,
								}},
							},
						},
					}},
				},
			}},
		}},
	}
	prog := l.Lower(file)
	fn := prog.Functions[0]
	if len(fn.Body) != 1 {
		t.Fatalf("expected 1 stmt (while), got %d", len(fn.Body))
	}
	if fn.Body[0].Kind != LStmtWhile {
		t.Fatalf("expected While, got %d", fn.Body[0].Kind)
	}
	w := fn.Body[0].Data.(*LWhile)
	if len(w.CondBlock) == 0 {
		t.Error("expected non-empty CondBlock")
	}
	if w.CondVar.Kind != LValTemp {
		t.Errorf("expected CondVar to be temp, got %d", w.CondVar.Kind)
	}
	if len(w.Body) != 1 || w.Body[0].Kind != LStmtBreak {
		t.Error("expected body with break")
	}
}

func TestLowerTypeMapping(t *testing.T) {
	l := NewLowerer()
	tests := []struct {
		name string
		want LTypeKind
	}{
		{"i8", LTyI8}, {"i16", LTyI16}, {"i32", LTyI32}, {"i64", LTyI64},
		{"u8", LTyU8}, {"u16", LTyU16}, {"u32", LTyU32}, {"u64", LTyU64},
		{"f32", LTyF32}, {"f64", LTyF64},
		{"bool", LTyBool}, {"string", LTyString},
		{"int", LTyPlatformInt}, {"uint", LTyPlatformUint},
		{"error", LTyError}, {"any", LTyAny},
	}
	for _, tt := range tests {
		te := &ast.TypeExpr{Kind: ast.TypeNamed, Data: &ast.NamedType{Name: tt.name}}
		lt := l.lowerTypeExpr(te)
		if lt.Kind != tt.want {
			t.Errorf("%s: expected kind %d, got %d", tt.name, tt.want, lt.Kind)
		}
	}
}

func TestLowerOptionalType(t *testing.T) {
	l := NewLowerer()
	te := &ast.TypeExpr{
		Kind: ast.TypeOptional,
		Data: &ast.OptionalType{
			Inner: ast.TypeExpr{Kind: ast.TypeNamed, Data: &ast.NamedType{Name: "i32"}},
		},
	}
	lt := l.lowerTypeExpr(te)
	if lt.Kind != LTyOptional {
		t.Errorf("expected Optional, got %d", lt.Kind)
	}
	if lt.Elem.Kind != LTyI32 {
		t.Errorf("expected i32 elem, got %d", lt.Elem.Kind)
	}
}

func TestLowerErrorResultTuple(t *testing.T) {
	l := NewLowerer()
	te := &ast.TypeExpr{
		Kind: ast.TypeTuple,
		Data: &ast.TupleType{
			Fields: []ast.TupleField{
				{Type: ast.TypeExpr{Kind: ast.TypeNamed, Data: &ast.NamedType{Name: "i32"}}},
				{Type: ast.TypeExpr{Kind: ast.TypeNamed, Data: &ast.NamedType{Name: "error"}}},
			},
		},
	}
	lt := l.lowerTypeExpr(te)
	if lt.Kind != LTyErrorResult {
		t.Errorf("expected ErrorResult, got %d", lt.Kind)
	}
	if lt.Elem.Kind != LTyI32 {
		t.Errorf("expected i32 value type, got %d", lt.Elem.Kind)
	}
}

func TestLowerIfElse(t *testing.T) {
	l := NewLowerer()
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Name: "test",
			Functions: []ast.FuncDecl{{
				Name: "decide",
				Body: &ast.Block{
					Stmts: []ast.Stmt{{
						Kind: ast.StmtIf,
						Data: &ast.IfStmt{
							Condition: ast.Expr{Kind: ast.ExprBoolLit, Data: &ast.BoolLitExpr{Value: true}},
							Then: ast.Block{
								Stmts: []ast.Stmt{{Kind: ast.StmtBreak}},
							},
							Else: &ast.Block{
								Stmts: []ast.Stmt{{Kind: ast.StmtContinue}},
							},
						},
					}},
				},
			}},
		}},
	}
	prog := l.Lower(file)
	fn := prog.Functions[0]
	if len(fn.Body) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(fn.Body))
	}
	if fn.Body[0].Kind != LStmtIf {
		t.Fatalf("expected If, got %d", fn.Body[0].Kind)
	}
	ifData := fn.Body[0].Data.(*LIf)
	if ifData.Cond.Kind != LValLitBool || !ifData.Cond.BoolVal {
		t.Error("expected true condition")
	}
	if len(ifData.Then) != 1 || ifData.Then[0].Kind != LStmtBreak {
		t.Error("expected break in then")
	}
	if len(ifData.Else) != 1 || ifData.Else[0].Kind != LStmtContinue {
		t.Error("expected continue in else")
	}
}

func TestLowerUnitVariant(t *testing.T) {
	l := NewLowerer()
	file := &ast.File{
		Blocks: []ast.GrokBlock{{
			Name: "test",
			Enums: []ast.EnumDecl{{
				Name: "Color",
				Variants: []ast.EnumVariant{
					{Name: "Red"},
					{Name: "Green"},
				},
			}},
			Functions: []ast.FuncDecl{{
				Name: "getColor",
				Body: &ast.Block{
					Stmts: []ast.Stmt{{
						Kind: ast.StmtReturn,
						Data: &ast.ReturnStmt{
							Value: &ast.Expr{
								Kind: ast.ExprIdent,
								Data: &ast.IdentExpr{Name: "Red"},
							},
						},
					}},
				},
			}},
		}},
	}
	prog := l.Lower(file)
	fn := prog.Functions[0]
	// Should have TempDef (variant construct) + Return
	if len(fn.Body) < 2 {
		t.Fatalf("expected >= 2 stmts, got %d", len(fn.Body))
	}
	td := fn.Body[0].Data.(*LTempDef)
	if td.Expr.Kind != LExprVariantConstruct {
		t.Errorf("expected VariantConstruct, got %d", td.Expr.Kind)
	}
	vc := td.Expr.Data.(*LVariantConstructData)
	if vc.Enum != "Color" || vc.Variant != "Red" || vc.Tag != 0 {
		t.Errorf("expected Color/Red/0, got %s/%s/%d", vc.Enum, vc.Variant, vc.Tag)
	}
}
