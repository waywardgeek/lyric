package lir

import (
	"fmt"

	"github.com/waywardgeek/forge/pkg/ast"
)

// ValidatePostMono checks LIR invariants after monomorphization. Returns violations.
// Checks:
// 1. No LTyTypeVar nodes remain (all generics resolved)
// 2. No generic functions remain (TypeParams should be empty)
// 3. No generic classes remain
func ValidatePostMono(prog *LProgram) []ast.InvariantViolation {
	var violations []ast.InvariantViolation

	// Check no generic functions remain
	for _, f := range prog.Functions {
		if len(f.TypeParams) > 0 {
			violations = append(violations, ast.InvariantViolation{
				Stage:   "post-mono",
				Check:   "no-generics",
				Message: fmt.Sprintf("function %s still has TypeParams %v", f.Name, f.TypeParams),
			})
		}
		// Check params for type vars
		for _, p := range f.Params {
			collectTypeVarViolations("post-mono", f.Name, "param "+p.Name, p.Type, &violations)
		}
		collectTypeVarViolations("post-mono", f.Name, "return", f.ReturnType, &violations)
		collectTypeVarStmtViolations(f.Name, f.Body, &violations)
	}

	// Check no generic classes remain
	for _, c := range prog.Classes {
		if len(c.TypeParams) > 0 {
			violations = append(violations, ast.InvariantViolation{
				Stage:   "post-mono",
				Check:   "no-generics",
				Message: fmt.Sprintf("class %s still has TypeParams", c.Name),
			})
		}
	}

	return violations
}

func collectTypeVarViolations(stage, funcName, context string, t *LType, violations *[]ast.InvariantViolation) {
	if t == nil {
		return
	}
	if t.Kind == LTyTypeVar {
		*violations = append(*violations, ast.InvariantViolation{
			Stage:   stage,
			Check:   "no-type-vars",
			Message: fmt.Sprintf("%s %s has unresolved type var %q", funcName, context, t.Name),
		})
		return
	}
	collectTypeVarViolations(stage, funcName, context, t.Elem, violations)
	collectTypeVarViolations(stage, funcName, context, t.Key, violations)
	collectTypeVarViolations(stage, funcName, context, t.Return, violations)
	for _, ta := range t.TypeArgs {
		collectTypeVarViolations(stage, funcName, context, ta, violations)
	}
	for _, f := range t.Fields {
		collectTypeVarViolations(stage, funcName, context+"."+f.Name, f.Type, violations)
	}
	for i, p := range t.Params {
		collectTypeVarViolations(stage, funcName, fmt.Sprintf("%s.param%d", context, i), p, violations)
	}
}

func collectTypeVarStmtViolations(funcName string, stmts []LStmt, violations *[]ast.InvariantViolation) {
	for _, stmt := range stmts {
		switch stmt.Kind {
		case LStmtTempDef:
			td := stmt.Data.(*LTempDef)
			collectTypeVarViolations("post-mono", funcName, fmt.Sprintf("_t%d", td.ID), td.Expr.Type, violations)
		case LStmtVarDecl:
			vd := stmt.Data.(*LVarDecl)
			collectTypeVarViolations("post-mono", funcName, "var "+vd.Name, vd.Type, violations)
		case LStmtIf:
			d := stmt.Data.(*LIf)
			collectTypeVarStmtViolations(funcName, d.Then, violations)
			collectTypeVarStmtViolations(funcName, d.Else, violations)
		case LStmtSwitch:
			d := stmt.Data.(*LSwitch)
			for _, c := range d.Cases {
				collectTypeVarStmtViolations(funcName, c.Body, violations)
			}
		case LStmtFor:
			d := stmt.Data.(*LFor)
			collectTypeVarViolations("post-mono", funcName, "for-var", d.VarType, violations)
			collectTypeVarStmtViolations(funcName, d.Body, violations)
		case LStmtWhile:
			d := stmt.Data.(*LWhile)
			collectTypeVarStmtViolations(funcName, d.CondBlock, violations)
			collectTypeVarStmtViolations(funcName, d.Body, violations)
		case LStmtTypeSwitch:
			d := stmt.Data.(*LTypeSwitch)
			for _, c := range d.Cases {
				collectTypeVarStmtViolations(funcName, c.Body, violations)
			}
		case LStmtBlock:
			d := stmt.Data.(*LBlock)
			collectTypeVarStmtViolations(funcName, d.Stmts, violations)
		}
	}
}


func ValidatePostLower(prog *LProgram) []ast.InvariantViolation {
	var violations []ast.InvariantViolation

	for _, f := range prog.Functions {
		// Check params for void*
		for _, p := range f.Params {
			if p.Type != nil && p.Type.Kind == LTyAny {
				violations = append(violations, ast.InvariantViolation{
					Stage:   "post-lower",
					Check:   "no-void-star",
					Message: fmt.Sprintf("%s param %q has type void* (LTyAny)", f.Name, p.Name),
				})
			}
		}
		// Check return type
		if f.ReturnType != nil && f.ReturnType.Kind == LTyAny {
			violations = append(violations, ast.InvariantViolation{
				Stage:   "post-lower",
				Check:   "no-void-star",
				Message: fmt.Sprintf("%s return type is void* (LTyAny)", f.Name),
			})
		}
		// Check temps/vars
		collectVoidStarViolations(f.Name, f.Body, &violations)
	}

	return violations
}

func collectVoidStarViolations(funcName string, stmts []LStmt, violations *[]ast.InvariantViolation) {
	for _, stmt := range stmts {
		switch stmt.Kind {
		case LStmtTempDef:
			td := stmt.Data.(*LTempDef)
			if td.Expr.Type != nil && td.Expr.Type.Kind == LTyAny {
				detail := lExprKindName(td.Expr.Kind)
				switch td.Expr.Kind {
				case LExprCall:
					if d := dataAs[LCallData](td.Expr.Data); d != nil {
						detail = fmt.Sprintf("Call(%s)", d.Func)
					}
				case LExprMethodCall:
					if d := dataAs[LMethodCallData](td.Expr.Data); d != nil {
						detail = fmt.Sprintf("MethodCall(.%s)", d.Method)
					}
				case LExprStructField:
					if d := dataAs[LStructFieldData](td.Expr.Data); d != nil {
						detail = fmt.Sprintf("StructField(.%s)", d.Field)
					}
				case LExprClassGet:
					if d := dataAs[LClassGetData](td.Expr.Data); d != nil {
						detail = fmt.Sprintf("ClassGet(%s.%s)", d.Class, d.Field)
					}
				}
				*violations = append(*violations, ast.InvariantViolation{
					Stage:   "post-lower",
					Check:   "no-void-star",
					Message: fmt.Sprintf("%s _t%d → void* via %s", funcName, td.ID, detail),
				})
			}
		case LStmtVarDecl:
			vd := stmt.Data.(*LVarDecl)
			if vd.Type != nil && vd.Type.Kind == LTyAny {
				*violations = append(*violations, ast.InvariantViolation{
					Stage:   "post-lower",
					Check:   "no-void-star",
					Message: fmt.Sprintf("%s var %q has type void* (LTyAny)", funcName, vd.Name),
				})
			}
		case LStmtBlock:
			b := stmt.Data.(*LBlock)
			collectVoidStarViolations(funcName, b.Stmts, violations)
		case LStmtIf:
			ifData := stmt.Data.(*LIf)
			collectVoidStarViolations(funcName, ifData.Then, violations)
			collectVoidStarViolations(funcName, ifData.Else, violations)
		case LStmtSwitch:
			sw := stmt.Data.(*LSwitch)
			for _, c := range sw.Cases {
				collectVoidStarViolations(funcName, c.Body, violations)
			}
		case LStmtFor:
			forData := stmt.Data.(*LFor)
			collectVoidStarViolations(funcName, forData.Body, violations)
		case LStmtWhile:
			whileData := stmt.Data.(*LWhile)
			collectVoidStarViolations(funcName, whileData.CondBlock, violations)
			collectVoidStarViolations(funcName, whileData.Body, violations)
		case LStmtTypeSwitch:
			ts := stmt.Data.(*LTypeSwitch)
			for _, c := range ts.Cases {
				collectVoidStarViolations(funcName, c.Body, violations)
			}
		}
	}
}
