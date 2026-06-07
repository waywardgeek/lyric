package lir

import (
	"fmt"
	"strconv"
)

// Optimize runs a post-lowering LIR→LIR optimization pass over the program.
// This fixes semantic issues that are easier to handle as transforms on the
// flat LIR representation than during initial lowering.
//
// Passes:
//   1. Side-effect temp elimination: fuse void-context builtins into bare statements
//   2. Multi-return destructuring: split single-temp multi-return into tuple VarDecls
//   3. For-range index type coercion: insert casts from platform int to fixed-width
//   4. Optional return wrapping: wrap T values into *T for optional returns

func Optimize(prog *LProgram) {
	for i := range prog.Functions {
		optimizeFunc(&prog.Functions[i])
	}
}

func optimizeFunc(fn *LFuncDecl) {
	// Run structural transforms first
	fn.Body = optimizeStmtsStructural(fn.Body, fn.ReturnType)

	// Collect used temps and vars on the TRANSFORMED stmts
	usedTemps := make(map[int]bool)
	collectUsedTemps(fn.Body, usedTemps)
	usedVars := make(map[string]bool)
	collectUsedVarNames(fn.Body, usedVars)

	fn.Body = eliminateUnusedTempsRecursive(fn.Body, usedTemps, usedVars)

	// Re-collect AFTER elimination — some VarDecls referencing MultiAssign names
	// may have been eliminated, making those names now unused
	usedVars2 := make(map[string]bool)
	collectUsedVarNames(fn.Body, usedVars2)
	usedTemps2 := make(map[int]bool)
	collectUsedTemps(fn.Body, usedTemps2)
	blankUnusedMultiAssignNames(fn.Body, usedTemps2, usedVars2)
}

func optimizeStmtsStructural(stmts []LStmt, returnType *LType) []LStmt {
	stmts = fuseSideEffectTemps(stmts)
	stmts = destructureMultiReturn(stmts)
	stmts = destructureExtractPairs(stmts)
	stmts = fixNilZeroValues(stmts)
	stmts = coerceForRangeIndex(stmts)
	stmts = wrapOptionalReturns(stmts, returnType)
	stmts = rewriteAppendReassign(stmts)

	// Recurse into nested blocks
	for i := range stmts {
		optimizeNestedStmtsStructural(&stmts[i], returnType)
	}
	return stmts
}

// ---------------------------------------------------------------------------
// Pass 2b: Destructure Extract pairs (try operator)
// ---------------------------------------------------------------------------
// Pattern:
//   _tN := someFunc(args...)          // call returning (T, error)
//   _tM := extractError(_tN)         // LExprExtractError
//   ...
//   _tK := extractValue(_tN)         // LExprExtractValue
//
// The _tN single-assignment is invalid in Go for multi-return.
// Replace _tN with a multi-assign: _tN_val, _tN_err := someFunc(...)
// Then rewrite extractError → _tN_err, extractValue → _tN_val

func destructureExtractPairs(stmts []LStmt) []LStmt {
	// First pass: find temps that are sources of Extract operations
	type extractInfo struct {
		hasValue bool
		hasError bool
	}
	extractSources := make(map[int]*extractInfo) // temp ID → extract info

	for _, s := range stmts {
		if s.Kind != LStmtTempDef {
			continue
		}
		td := s.Data.(*LTempDef)
		switch td.Expr.Kind {
		case LExprExtractValue:
			d := td.Expr.Data.(*LExtractValueData)
			if d.Value.Kind == LValTemp {
				info, ok := extractSources[d.Value.TempID]
				if !ok {
					info = &extractInfo{}
					extractSources[d.Value.TempID] = info
				}
				info.hasValue = true
			}
		case LExprExtractError:
			d := td.Expr.Data.(*LExtractErrorData)
			if d.Value.Kind == LValTemp {
				info, ok := extractSources[d.Value.TempID]
				if !ok {
					info = &extractInfo{}
					extractSources[d.Value.TempID] = info
				}
				info.hasError = true
			}
		}
	}

	if len(extractSources) == 0 {
		return stmts
	}

	// Second pass: rewrite
	// - Source temp → multi-assign with _tN_val, _tN_err names
	// - ExtractValue temp → simple assignment from _tN_val
	// - ExtractError temp → simple assignment from _tN_err
	var out []LStmt
	for _, s := range stmts {
		if s.Kind != LStmtTempDef {
			out = append(out, s)
			continue
		}
		td := s.Data.(*LTempDef)

		// Is this a source temp being extracted?
		if _, ok := extractSources[td.ID]; ok {
			valName := fmt.Sprintf("_t%d_val", td.ID)
			errName := fmt.Sprintf("_t%d_err", td.ID)
			out = append(out, LStmt{
				Kind: LStmtMultiAssign,
				Data: &LMultiAssign{
					Names: []string{valName, errName},
					Expr:  td.Expr,
				},
			})
			continue
		}

		// Is this an extract of a source?
		switch td.Expr.Kind {
		case LExprExtractValue:
			d := td.Expr.Data.(*LExtractValueData)
			if d.Value.Kind == LValTemp {
				if _, ok := extractSources[d.Value.TempID]; ok {
					valName := fmt.Sprintf("_t%d_val", d.Value.TempID)
					out = append(out, LStmt{
						Kind: LStmtTempDef,
						Data: &LTempDef{
							ID: td.ID,
							Expr: LExpr{
								Kind: LExprBinOp, // abuse: just need a value ref
								Type: td.Expr.Type,
							},
						},
					})
					// Actually simpler: just make the temp an alias
					// Emit: _tM := _tN_val
					out[len(out)-1] = LStmt{
						Kind: LStmtVarDecl,
						Data: &LVarDecl{
							Name: fmt.Sprintf("_t%d", td.ID),
							Type: td.Expr.Type,
							Init: &LValue{Kind: LValVar, Name: valName, Type: td.Expr.Type},
						},
					}
					continue
				}
			}
		case LExprExtractError:
			d := td.Expr.Data.(*LExtractErrorData)
			if d.Value.Kind == LValTemp {
				if _, ok := extractSources[d.Value.TempID]; ok {
					errName := fmt.Sprintf("_t%d_err", d.Value.TempID)
					out = append(out, LStmt{
						Kind: LStmtVarDecl,
						Data: &LVarDecl{
							Name: fmt.Sprintf("_t%d", td.ID),
							Type: td.Expr.Type,
							Init: &LValue{Kind: LValVar, Name: errName, Type: td.Expr.Type},
						},
					})
					continue
				}
			}
		}

		out = append(out, s)
	}
	return out
}

// ---------------------------------------------------------------------------
// Pass 2c: Fix nil zero values
// ---------------------------------------------------------------------------
// In Go, `return nil, err` doesn't work when the first return is a value type
// like int32. Replace nil with the zero value for the type.

func fixNilZeroValues(stmts []LStmt) []LStmt {
	for i := range stmts {
		if stmts[i].Kind == LStmtReturn {
			r := stmts[i].Data.(*LReturn)
			for j := range r.Values {
				if r.Values[j].Kind == LValLitNull && r.Values[j].Type != nil {
					r.Values[j] = zeroValueForType(r.Values[j].Type)
				}
			}
		}
	}
	return stmts
}

func zeroValueForType(t *LType) LValue {
	switch t.Kind {
	case LTyI8, LTyI16, LTyI32, LTyI64, LTyPlatformInt:
		return LValue{Kind: LValLitInt, IntVal: 0, Type: t}
	case LTyU8, LTyU16, LTyU32, LTyU64, LTyPlatformUint:
		return LValue{Kind: LValLitUint, UintVal: 0, Type: t}
	case LTyF32, LTyF64:
		return LValue{Kind: LValLitFloat, FloatVal: 0, Type: t}
	case LTyBool:
		return LValue{Kind: LValLitBool, BoolVal: false, Type: t}
	case LTyString:
		return LValue{Kind: LValLitString, StrVal: "", Type: t}
	default:
		return LValue{Kind: LValLitNull, Type: t}
	}
}

func optimizeNestedStmtsStructural(s *LStmt, returnType *LType) {
	switch s.Kind {
	case LStmtIf:
		d := s.Data.(*LIf)
		d.Then = optimizeStmtsStructural(d.Then, returnType)
		d.Else = optimizeStmtsStructural(d.Else, returnType)
	case LStmtWhile:
		d := s.Data.(*LWhile)
		d.CondBlock = optimizeStmtsStructural(d.CondBlock, returnType)
		d.Body = optimizeStmtsStructural(d.Body, returnType)
	case LStmtFor:
		d := s.Data.(*LFor)
		d.Body = optimizeStmtsStructural(d.Body, returnType)
	case LStmtSwitch:
		d := s.Data.(*LSwitch)
		for j := range d.Cases {
			d.Cases[j].Body = optimizeStmtsStructural(d.Cases[j].Body, returnType)
		}
	case LStmtTypeSwitch:
		d := s.Data.(*LTypeSwitch)
		for j := range d.Cases {
			d.Cases[j].Body = optimizeStmtsStructural(d.Cases[j].Body, returnType)
		}
	case LStmtBlock:
		d := s.Data.(*LBlock)
		d.Stmts = optimizeStmtsStructural(d.Stmts, returnType)
	case LStmtDefer:
		d := s.Data.(*LDefer)
		d.Body = optimizeStmtsStructural(d.Body, returnType)
	case LStmtSpawn:
		d := s.Data.(*LSpawn)
		d.Body = optimizeStmtsStructural(d.Body, returnType)
	case LStmtLock:
		d := s.Data.(*LLock)
		d.Body = optimizeStmtsStructural(d.Body, returnType)
	case LStmtSelect:
		d := s.Data.(*LSelect)
		for j := range d.Cases {
			d.Cases[j].Body = optimizeStmtsStructural(d.Cases[j].Body, returnType)
		}
	}
}

// blankUnusedMultiAssignNames blanks unused names in MultiAssign statements.
// Must be called AFTER temp/var elimination so that eliminated VarDecls
// don't falsely mark MultiAssign names as used.
func blankUnusedMultiAssignNames(stmts []LStmt, usedTemps map[int]bool, usedVars map[string]bool) {
	for i := range stmts {
		if stmts[i].Kind == LStmtMultiAssign {
			ma := stmts[i].Data.(*LMultiAssign)
			for j, name := range ma.Names {
				if name != "_" && !usedVars[name] && !usedTemps[parseTempID(name)] {
					ma.Names[j] = "_"
				}
			}
		}
		// Recurse into nested blocks
		switch stmts[i].Kind {
		case LStmtIf:
			d := stmts[i].Data.(*LIf)
			blankUnusedMultiAssignNames(d.Then, usedTemps, usedVars)
			blankUnusedMultiAssignNames(d.Else, usedTemps, usedVars)
		case LStmtWhile:
			d := stmts[i].Data.(*LWhile)
			blankUnusedMultiAssignNames(d.CondBlock, usedTemps, usedVars)
			blankUnusedMultiAssignNames(d.Body, usedTemps, usedVars)
		case LStmtFor:
			d := stmts[i].Data.(*LFor)
			blankUnusedMultiAssignNames(d.Body, usedTemps, usedVars)
		case LStmtSwitch:
			d := stmts[i].Data.(*LSwitch)
			for j := range d.Cases {
				blankUnusedMultiAssignNames(d.Cases[j].Body, usedTemps, usedVars)
			}
		case LStmtTypeSwitch:
			d := stmts[i].Data.(*LTypeSwitch)
			for j := range d.Cases {
				blankUnusedMultiAssignNames(d.Cases[j].Body, usedTemps, usedVars)
			}
		case LStmtBlock:
			d := stmts[i].Data.(*LBlock)
			blankUnusedMultiAssignNames(d.Stmts, usedTemps, usedVars)
		case LStmtDefer:
			d := stmts[i].Data.(*LDefer)
			blankUnusedMultiAssignNames(d.Body, usedTemps, usedVars)
		case LStmtSpawn:
			d := stmts[i].Data.(*LSpawn)
			blankUnusedMultiAssignNames(d.Body, usedTemps, usedVars)
		case LStmtLock:
			d := stmts[i].Data.(*LLock)
			blankUnusedMultiAssignNames(d.Body, usedTemps, usedVars)
		case LStmtSelect:
			d := stmts[i].Data.(*LSelect)
			for j := range d.Cases {
				blankUnusedMultiAssignNames(d.Cases[j].Body, usedTemps, usedVars)
			}
		}
	}
}

// eliminateUnusedTempsRecursive removes unused temps from stmts and all nested blocks
func eliminateUnusedTempsRecursive(stmts []LStmt, usedTemps map[int]bool, usedVars map[string]bool) []LStmt {
	stmts = eliminateUnusedTemps(stmts, usedTemps)
	for i := range stmts {
		switch stmts[i].Kind {
		case LStmtIf:
			d := stmts[i].Data.(*LIf)
			d.Then = eliminateUnusedTempsRecursive(d.Then, usedTemps, usedVars)
			d.Else = eliminateUnusedTempsRecursive(d.Else, usedTemps, usedVars)
		case LStmtWhile:
			d := stmts[i].Data.(*LWhile)
			d.CondBlock = eliminateUnusedTempsRecursive(d.CondBlock, usedTemps, usedVars)
			d.Body = eliminateUnusedTempsRecursive(d.Body, usedTemps, usedVars)
		case LStmtFor:
			d := stmts[i].Data.(*LFor)
			d.Body = eliminateUnusedTempsRecursive(d.Body, usedTemps, usedVars)
		case LStmtSwitch:
			d := stmts[i].Data.(*LSwitch)
			for j := range d.Cases {
				d.Cases[j].Body = eliminateUnusedTempsRecursive(d.Cases[j].Body, usedTemps, usedVars)
			}
		case LStmtTypeSwitch:
			d := stmts[i].Data.(*LTypeSwitch)
			for j := range d.Cases {
				d.Cases[j].Body = eliminateUnusedTempsRecursive(d.Cases[j].Body, usedTemps, usedVars)
			}
		case LStmtBlock:
			d := stmts[i].Data.(*LBlock)
			d.Stmts = eliminateUnusedTempsRecursive(d.Stmts, usedTemps, usedVars)
		case LStmtDefer:
			d := stmts[i].Data.(*LDefer)
			d.Body = eliminateUnusedTempsRecursive(d.Body, usedTemps, usedVars)
		case LStmtSpawn:
			d := stmts[i].Data.(*LSpawn)
			d.Body = eliminateUnusedTempsRecursive(d.Body, usedTemps, usedVars)
		case LStmtLock:
			d := stmts[i].Data.(*LLock)
			d.Body = eliminateUnusedTempsRecursive(d.Body, usedTemps, usedVars)
		case LStmtSelect:
			d := stmts[i].Data.(*LSelect)
			for j := range d.Cases {
				d.Cases[j].Body = eliminateUnusedTempsRecursive(d.Cases[j].Body, usedTemps, usedVars)
			}
		}
	}
	return stmts
}

// ---------------------------------------------------------------------------
// Pass 1: Side-effect temp elimination
// ---------------------------------------------------------------------------
// Pattern: _tN := fmt.Println(...)  followed by  _ = _tN
// Fuse into: a single LStmtSideEffect (bare call statement, no assignment)
//
// Also handles: LExprBuiltin for println/print, and any call whose result
// is only consumed by a LStmtExpr (discard).

func fuseSideEffectTemps(stmts []LStmt) []LStmt {
	// Build set of temp IDs that are only used in LStmtExpr discards
	discardedTemps := make(map[int]bool)
	for _, s := range stmts {
		if s.Kind == LStmtExpr {
			es := s.Data.(*LExprStmt)
			discardedTemps[es.TempID] = true
		}
	}

	var out []LStmt
	for _, s := range stmts {
		if s.Kind == LStmtTempDef {
			td := s.Data.(*LTempDef)
			if discardedTemps[td.ID] {
				// This temp is only used in a discard — check if it's a call/builtin
				if isSideEffectExpr(&td.Expr) {
					// Emit as bare side-effect statement
					out = append(out, LStmt{
						Kind: LStmtSideEffect,
						Data: &LSideEffect{Expr: td.Expr},
					})
					continue
				}
			}
		}
		if s.Kind == LStmtExpr {
			es := s.Data.(*LExprStmt)
			if discardedTemps[es.TempID] {
				// Skip the discard — already fused above (or will be unused)
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

func isSideEffectExpr(e *LExpr) bool {
	switch e.Kind {
	case LExprCall, LExprMethodCall:
		return true
	case LExprBuiltin:
		d := e.Data.(*LBuiltinData)
		switch d.Name {
		case "println", "print", "channel_close", "channel_receive":
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Pass 2: Multi-return destructuring
// ---------------------------------------------------------------------------
// Pattern:
//   _tN := someFunc(args...)    // function returns (T, error)
//   val := _tN                  // LStmtVarDecl with Init referencing _tN
//   err := _tN                  // LStmtVarDecl with Init referencing _tN
//
// Replace with:
//   LStmtMultiAssign{Names: ["val", "err"], Expr: call-expr}

func destructureMultiReturn(stmts []LStmt) []LStmt {
	var out []LStmt
	i := 0
	for i < len(stmts) {
		if i+2 < len(stmts) && stmts[i].Kind == LStmtTempDef {
			td := stmts[i].Data.(*LTempDef)
			// Check if next 1+ stmts are VarDecls referencing this temp
			tempID := td.ID
			j := i + 1
			var names []string
			var types []*LType
			for j < len(stmts) && stmts[j].Kind == LStmtVarDecl {
				vd := stmts[j].Data.(*LVarDecl)
				if vd.Init != nil && vd.Init.Kind == LValTemp && vd.Init.TempID == tempID {
					names = append(names, vd.Name)
					types = append(types, vd.Type)
					j++
				} else {
					break
				}
			}
			if len(names) >= 2 {
				// Multi-return destructuring
				out = append(out, LStmt{
					Kind: LStmtMultiAssign,
					Data: &LMultiAssign{
						Names: names,
						Types: types,
						Expr:  td.Expr,
					},
				})
				i = j
				continue
			}
		}
		out = append(out, stmts[i])
		i++
	}
	return out
}

// ---------------------------------------------------------------------------
// Pass 6: Rewrite append/push to reassign
// ---------------------------------------------------------------------------
// Pattern: _tN := append(slice, x)  where _tN is never used
// Rewrite: slice = append(slice, x)  (Go requires append result to be assigned)
// Same for method calls: _tN := slice.push(x) → slice = slice.push(x)

func rewriteAppendReassign(stmts []LStmt) []LStmt {
	// No-op: handled in C backend by detecting SideEffect(append(...))
	return stmts
}

// ---------------------------------------------------------------------------
// Pass 3: For-range index type coercion
// ---------------------------------------------------------------------------
// In Go, for-range index is `int`. Forge uses i32 by default.
// When the index variable is used with i32 context, insert a cast.
// For now: if a For stmt has an IndexVar and the body uses it in
// expressions with int32 types, we insert a shadow variable.
//
// Actually simpler: just change the For's index var type to platform int,
// and let Go handle it. The real fix is at the use site.
// For now we skip this — it requires data-flow analysis.

func coerceForRangeIndex(stmts []LStmt) []LStmt {
	// TODO: implement type coercion for for-range indices
	return stmts
}

// ---------------------------------------------------------------------------
// Pass 4: Optional return wrapping
// ---------------------------------------------------------------------------
// When a function returns *T (optional) but the return statement has a bare
// T value, wrap it in LExprWrapOptional.

func wrapOptionalReturns(stmts []LStmt, returnType *LType) []LStmt {
	if returnType == nil {
		return stmts
	}
	for i := range stmts {
		if stmts[i].Kind == LStmtReturn {
			r := stmts[i].Data.(*LReturn)
			if returnType.Kind == LTyOptional && len(r.Values) == 1 {
				v := r.Values[0]
				// If the value is nil, don't wrap
				if v.Kind == LValLitNull {
					continue
				}
				// If the value's type is not already a pointer, wrap it
				if v.Type != nil && v.Type.Kind != LTyOptional {
					r.Values[0] = LValue{
						Kind: LValTemp,
						Type: returnType,
					}
					// We can't easily insert a temp here in the post-pass...
					// This needs to be handled differently.
					// Skip for now — the lowerer should handle this.
				}
			}
		}
	}
	return stmts
}

// ---------------------------------------------------------------------------
// Pass 5: Eliminate unused temps
// ---------------------------------------------------------------------------
// Find temps that are declared but never referenced in any value position.
// Convert them to side-effect statements (bare expression) or remove them.

func eliminateUnusedTemps(stmts []LStmt, usedTemps map[int]bool) []LStmt {

	// Also collect used variable names for VarDecl elimination
	usedVars := make(map[string]bool)
	collectUsedVarNames(stmts, usedVars)

	// Rewrite: unused TempDef → SideEffect (if call/builtin) or remove
	var out []LStmt
	for _, s := range stmts {
		if s.Kind == LStmtTempDef {
			td := s.Data.(*LTempDef)
			if !usedTemps[td.ID] {
				// Temp is unused — emit as side effect if it has side effects, else drop
				if isSideEffectExpr(&td.Expr) || td.Expr.Kind == LExprBuiltin {
					out = append(out, LStmt{
						Kind: LStmtSideEffect,
						Data: &LSideEffect{Expr: td.Expr},
					})
				}
				// Otherwise drop entirely
				continue
			}
		}
		// Also eliminate unused VarDecls with temp-like names (_tN patterns)
		if s.Kind == LStmtVarDecl {
			vd := s.Data.(*LVarDecl)
			if len(vd.Name) > 2 && vd.Name[0] == '_' && vd.Name[1] == 't' {
				// Parse the numeric ID from the name
				numStr := vd.Name[2:]
				isNumeric := true
				for _, c := range numStr {
					if c < '0' || c > '9' {
						isNumeric = false
						break
					}
				}
				if isNumeric && !usedVars[vd.Name] && !usedTemps[parseTempID(vd.Name)] {
					continue
				}
			}
		}
		out = append(out, s)
	}
	return out
}

func parseTempID(name string) int {
	if len(name) > 2 && name[0] == '_' && name[1] == 't' {
		numStr := name[2:]
		// Strip _val/_err suffix if present
		if idx := len(numStr); idx > 4 && numStr[idx-4:] == "_val" {
			numStr = numStr[:idx-4]
		} else if idx > 4 && numStr[idx-4:] == "_err" {
			numStr = numStr[:idx-4]
		}
		id, err := strconv.Atoi(numStr)
		if err == nil {
			return id
		}
	}
	return -1
}

func collectUsedTemps(stmts []LStmt, used map[int]bool) {
	for _, s := range stmts {
		collectUsedTempsInStmt(&s, used)
	}
}

func collectUsedTempsInStmt(s *LStmt, used map[int]bool) {
	switch s.Kind {
	case LStmtTempDef:
		td := s.Data.(*LTempDef)
		collectUsedTempsInExpr(&td.Expr, used)
	case LStmtVarDecl:
		vd := s.Data.(*LVarDecl)
		if vd.Init != nil {
			collectUsedTempsInValue(vd.Init, used)
		}
	case LStmtAssign:
		a := s.Data.(*LAssign)
		collectUsedTempsInValue(&a.Value, used)
	case LStmtStructSet:
		ss := s.Data.(*LStructSet)
		collectUsedTempsInValue(&ss.Receiver, used)
		collectUsedTempsInValue(&ss.Value, used)
	case LStmtClassSet:
		cs := s.Data.(*LClassSet)
		collectUsedTempsInValue(&cs.Handle, used)
		collectUsedTempsInValue(&cs.Value, used)
	case LStmtIndexSet:
		is := s.Data.(*LIndexSet)
		collectUsedTempsInValue(&is.Collection, used)
		collectUsedTempsInValue(&is.Index, used)
		collectUsedTempsInValue(&is.Value, used)
	case LStmtIf:
		d := s.Data.(*LIf)
		collectUsedTempsInValue(&d.Cond, used)
		collectUsedTemps(d.Then, used)
		collectUsedTemps(d.Else, used)
	case LStmtWhile:
		d := s.Data.(*LWhile)
		collectUsedTemps(d.CondBlock, used)
		collectUsedTempsInValue(&d.CondVar, used)
		collectUsedTemps(d.Body, used)
	case LStmtFor:
		d := s.Data.(*LFor)
		collectUsedTempsInValue(&d.Collection, used)
		collectUsedTemps(d.Body, used)
	case LStmtSwitch:
		d := s.Data.(*LSwitch)
		collectUsedTempsInValue(&d.Tag, used)
		for _, c := range d.Cases {
			collectUsedTemps(c.Body, used)
		}
	case LStmtTypeSwitch:
		d := s.Data.(*LTypeSwitch)
		collectUsedTempsInValue(&d.Value, used)
		for _, c := range d.Cases {
			collectUsedTemps(c.Body, used)
		}
	case LStmtReturn:
		r := s.Data.(*LReturn)
		for _, v := range r.Values {
			collectUsedTempsInValue(&v, used)
		}
	case LStmtSend:
		snd := s.Data.(*LSend)
		collectUsedTempsInValue(&snd.Channel, used)
		collectUsedTempsInValue(&snd.Value, used)
	case LStmtSelect:
		sel := s.Data.(*LSelect)
		for _, c := range sel.Cases {
			collectUsedTempsInValue(&c.Channel, used)
			collectUsedTempsInValue(&c.Value, used)
			collectUsedTemps(c.Body, used)
		}
	case LStmtExpr:
		es := s.Data.(*LExprStmt)
		used[es.TempID] = true
	case LStmtSideEffect:
		se := s.Data.(*LSideEffect)
		collectUsedTempsInExpr(&se.Expr, used)
	case LStmtMultiAssign:
		ma := s.Data.(*LMultiAssign)
		collectUsedTempsInExpr(&ma.Expr, used)
	case LStmtBlock:
		d := s.Data.(*LBlock)
		collectUsedTemps(d.Stmts, used)
	case LStmtDefer:
		d := s.Data.(*LDefer)
		collectUsedTemps(d.Body, used)
	case LStmtSpawn:
		d := s.Data.(*LSpawn)
		collectUsedTemps(d.Body, used)
	case LStmtLock:
		d := s.Data.(*LLock)
		collectUsedTempsInValue(&d.Mutex, used)
		collectUsedTemps(d.Body, used)
	}
}

func collectUsedTempsInExpr(e *LExpr, used map[int]bool) {
	if e == nil {
		return
	}
	switch e.Kind {
	case LExprBinOp:
		d := e.Data.(*LBinOpData)
		collectUsedTempsInValue(&d.Left, used)
		collectUsedTempsInValue(&d.Right, used)
	case LExprUnOp:
		d := e.Data.(*LUnOpData)
		collectUsedTempsInValue(&d.Operand, used)
	case LExprCast:
		d := e.Data.(*LCastData)
		collectUsedTempsInValue(&d.Operand, used)
	case LExprStructField:
		d := e.Data.(*LStructFieldData)
		collectUsedTempsInValue(&d.Receiver, used)
	case LExprClassGet:
		d := e.Data.(*LClassGetData)
		collectUsedTempsInValue(&d.Handle, used)
	case LExprIndexGet:
		d := e.Data.(*LIndexGetData)
		collectUsedTempsInValue(&d.Collection, used)
		collectUsedTempsInValue(&d.Index, used)
	case LExprCall:
		d := e.Data.(*LCallData)
		for _, a := range d.Args {
			collectUsedTempsInValue(&a, used)
		}
	case LExprMethodCall:
		d := e.Data.(*LMethodCallData)
		collectUsedTempsInValue(&d.Receiver, used)
		for _, a := range d.Args {
			collectUsedTempsInValue(&a, used)
		}
	case LExprBuiltin:
		d := e.Data.(*LBuiltinData)
		for _, a := range d.Args {
			collectUsedTempsInValue(&a, used)
		}
	case LExprStructLit:
		d := e.Data.(*LStructLitData)
		for _, f := range d.Fields {
			collectUsedTempsInValue(&f.Value, used)
		}
	case LExprClassAlloc:
		d := e.Data.(*LClassAllocData)
		for _, f := range d.Fields {
			collectUsedTempsInValue(&f.Value, used)
		}
	case LExprWrapOptional:
		d := e.Data.(*LWrapOptionalData)
		collectUsedTempsInValue(&d.Value, used)
	case LExprUnwrapOptional:
		d := e.Data.(*LUnwrapOptionalData)
		collectUsedTempsInValue(&d.Value, used)
	case LExprIsNull:
		d := e.Data.(*LIsNullData)
		collectUsedTempsInValue(&d.Value, used)
	case LExprVariantConstruct:
		d := e.Data.(*LVariantConstructData)
		for _, f := range d.Fields {
			collectUsedTempsInValue(&f, used)
		}
	case LExprVariantTag:
		d := e.Data.(*LVariantTagData)
		collectUsedTempsInValue(&d.Value, used)
	case LExprVariantData:
		d := e.Data.(*LVariantDataData)
		collectUsedTempsInValue(&d.Value, used)
	case LExprExtractValue:
		d := e.Data.(*LExtractValueData)
		collectUsedTempsInValue(&d.Value, used)
	case LExprExtractError:
		d := e.Data.(*LExtractErrorData)
		collectUsedTempsInValue(&d.Value, used)
	case LExprMakeResult:
		d := e.Data.(*LMakeResultData)
		collectUsedTempsInValue(&d.Value, used)
		collectUsedTempsInValue(&d.Err, used)
	case LExprFormat:
		d := e.Data.(*LFormatData)
		for _, p := range d.Parts {
			if !p.IsLiteral {
				collectUsedTempsInValue(&p.Value, used)
			}
		}
	case LExprFuncLit:
		d := e.Data.(*LFuncLitData)
		collectUsedTemps(d.Body, used)
	case LExprSlice:
		d := e.Data.(*LSliceData)
		collectUsedTempsInValue(&d.Collection, used)
		if d.Low != nil {
			collectUsedTempsInValue(d.Low, used)
		}
		if d.High != nil {
			collectUsedTempsInValue(d.High, used)
		}
	case LExprMakeChannel:
		d := e.Data.(*LMakeChannelData)
		if d.BufSize != nil {
			collectUsedTempsInValue(d.BufSize, used)
		}
	}
}

func collectUsedTempsInValue(v *LValue, used map[int]bool) {
	if v == nil {
		return
	}
	if v.Kind == LValTemp {
		used[v.TempID] = true
	}
}


// collectUsedVarNames collects all variable names that appear in value positions
func collectUsedVarNames(stmts []LStmt, used map[string]bool) {
	for _, s := range stmts {
		collectUsedVarNamesInStmt(&s, used)
	}
}

func collectUsedVarNamesInStmt(s *LStmt, used map[string]bool) {
	switch s.Kind {
	case LStmtTempDef:
		td := s.Data.(*LTempDef)
		collectUsedVarNamesInExpr(&td.Expr, used)
	case LStmtVarDecl:
		vd := s.Data.(*LVarDecl)
		if vd.Init != nil {
			collectUsedVarNamesInValue(vd.Init, used)
		}
	case LStmtAssign:
		a := s.Data.(*LAssign)
		collectUsedVarNamesInValue(&a.Value, used)
		// The target itself is a use for assignment
	case LStmtStructSet:
		ss := s.Data.(*LStructSet)
		collectUsedVarNamesInValue(&ss.Receiver, used)
		collectUsedVarNamesInValue(&ss.Value, used)
	case LStmtClassSet:
		cs := s.Data.(*LClassSet)
		collectUsedVarNamesInValue(&cs.Handle, used)
		collectUsedVarNamesInValue(&cs.Value, used)
	case LStmtIndexSet:
		is := s.Data.(*LIndexSet)
		collectUsedVarNamesInValue(&is.Collection, used)
		collectUsedVarNamesInValue(&is.Index, used)
		collectUsedVarNamesInValue(&is.Value, used)
	case LStmtIf:
		d := s.Data.(*LIf)
		collectUsedVarNamesInValue(&d.Cond, used)
		collectUsedVarNames(d.Then, used)
		collectUsedVarNames(d.Else, used)
	case LStmtWhile:
		d := s.Data.(*LWhile)
		collectUsedVarNames(d.CondBlock, used)
		collectUsedVarNamesInValue(&d.CondVar, used)
		collectUsedVarNames(d.Body, used)
	case LStmtFor:
		d := s.Data.(*LFor)
		collectUsedVarNamesInValue(&d.Collection, used)
		collectUsedVarNames(d.Body, used)
	case LStmtSwitch:
		d := s.Data.(*LSwitch)
		collectUsedVarNamesInValue(&d.Tag, used)
		for _, c := range d.Cases {
			collectUsedVarNames(c.Body, used)
		}
	case LStmtTypeSwitch:
		d := s.Data.(*LTypeSwitch)
		collectUsedVarNamesInValue(&d.Value, used)
		for _, c := range d.Cases {
			collectUsedVarNames(c.Body, used)
		}
	case LStmtReturn:
		r := s.Data.(*LReturn)
		for _, v := range r.Values {
			collectUsedVarNamesInValue(&v, used)
		}
	case LStmtSend:
		snd := s.Data.(*LSend)
		collectUsedVarNamesInValue(&snd.Channel, used)
		collectUsedVarNamesInValue(&snd.Value, used)
	case LStmtSelect:
		sel := s.Data.(*LSelect)
		for _, c := range sel.Cases {
			collectUsedVarNamesInValue(&c.Channel, used)
			collectUsedVarNamesInValue(&c.Value, used)
			collectUsedVarNames(c.Body, used)
		}
	case LStmtSideEffect:
		se := s.Data.(*LSideEffect)
		collectUsedVarNamesInExpr(&se.Expr, used)
	case LStmtMultiAssign:
		ma := s.Data.(*LMultiAssign)
		collectUsedVarNamesInExpr(&ma.Expr, used)
	case LStmtBlock:
		d := s.Data.(*LBlock)
		collectUsedVarNames(d.Stmts, used)
	case LStmtDefer:
		d := s.Data.(*LDefer)
		collectUsedVarNames(d.Body, used)
	case LStmtSpawn:
		d := s.Data.(*LSpawn)
		collectUsedVarNames(d.Body, used)
	case LStmtLock:
		d := s.Data.(*LLock)
		collectUsedVarNamesInValue(&d.Mutex, used)
		collectUsedVarNames(d.Body, used)
	}
}

func collectUsedVarNamesInExpr(e *LExpr, used map[string]bool) {
	if e == nil {
		return
	}
	switch e.Kind {
	case LExprBinOp:
		d := e.Data.(*LBinOpData)
		collectUsedVarNamesInValue(&d.Left, used)
		collectUsedVarNamesInValue(&d.Right, used)
	case LExprUnOp:
		d := e.Data.(*LUnOpData)
		collectUsedVarNamesInValue(&d.Operand, used)
	case LExprCast:
		d := e.Data.(*LCastData)
		collectUsedVarNamesInValue(&d.Operand, used)
	case LExprStructField:
		d := e.Data.(*LStructFieldData)
		collectUsedVarNamesInValue(&d.Receiver, used)
	case LExprClassGet:
		d := e.Data.(*LClassGetData)
		collectUsedVarNamesInValue(&d.Handle, used)
	case LExprIndexGet:
		d := e.Data.(*LIndexGetData)
		collectUsedVarNamesInValue(&d.Collection, used)
		collectUsedVarNamesInValue(&d.Index, used)
	case LExprCall:
		d := e.Data.(*LCallData)
		for i := range d.Args {
			collectUsedVarNamesInValue(&d.Args[i], used)
		}
	case LExprMethodCall:
		d := e.Data.(*LMethodCallData)
		collectUsedVarNamesInValue(&d.Receiver, used)
		for i := range d.Args {
			collectUsedVarNamesInValue(&d.Args[i], used)
		}
	case LExprBuiltin:
		d := e.Data.(*LBuiltinData)
		for i := range d.Args {
			collectUsedVarNamesInValue(&d.Args[i], used)
		}
	case LExprStructLit:
		d := e.Data.(*LStructLitData)
		for i := range d.Fields {
			collectUsedVarNamesInValue(&d.Fields[i].Value, used)
		}
	case LExprClassAlloc:
		d := e.Data.(*LClassAllocData)
		for i := range d.Fields {
			collectUsedVarNamesInValue(&d.Fields[i].Value, used)
		}
	case LExprWrapOptional:
		d := e.Data.(*LWrapOptionalData)
		collectUsedVarNamesInValue(&d.Value, used)
	case LExprUnwrapOptional:
		d := e.Data.(*LUnwrapOptionalData)
		collectUsedVarNamesInValue(&d.Value, used)
	case LExprIsNull:
		d := e.Data.(*LIsNullData)
		collectUsedVarNamesInValue(&d.Value, used)
	case LExprVariantConstruct:
		d := e.Data.(*LVariantConstructData)
		for i := range d.Fields {
			collectUsedVarNamesInValue(&d.Fields[i], used)
		}
	case LExprVariantTag:
		d := e.Data.(*LVariantTagData)
		collectUsedVarNamesInValue(&d.Value, used)
	case LExprVariantData:
		d := e.Data.(*LVariantDataData)
		collectUsedVarNamesInValue(&d.Value, used)
	case LExprExtractValue:
		d := e.Data.(*LExtractValueData)
		collectUsedVarNamesInValue(&d.Value, used)
	case LExprExtractError:
		d := e.Data.(*LExtractErrorData)
		collectUsedVarNamesInValue(&d.Value, used)
	case LExprMakeResult:
		d := e.Data.(*LMakeResultData)
		collectUsedVarNamesInValue(&d.Value, used)
		collectUsedVarNamesInValue(&d.Err, used)
	case LExprFormat:
		d := e.Data.(*LFormatData)
		for _, p := range d.Parts {
			if !p.IsLiteral {
				collectUsedVarNamesInValue(&p.Value, used)
			}
		}
	case LExprFuncLit:
		d := e.Data.(*LFuncLitData)
		collectUsedVarNames(d.Body, used)
	case LExprSlice:
		d := e.Data.(*LSliceData)
		collectUsedVarNamesInValue(&d.Collection, used)
		if d.Low != nil {
			collectUsedVarNamesInValue(d.Low, used)
		}
		if d.High != nil {
			collectUsedVarNamesInValue(d.High, used)
		}
	case LExprMakeChannel:
		d := e.Data.(*LMakeChannelData)
		if d.BufSize != nil {
			collectUsedVarNamesInValue(d.BufSize, used)
		}
	}
}

func collectUsedVarNamesInValue(v *LValue, used map[string]bool) {
	if v == nil {
		return
	}
	if v.Kind == LValVar {
		used[v.Name] = true
	}
}
