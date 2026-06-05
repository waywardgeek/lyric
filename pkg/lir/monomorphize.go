package lir

import (
	"fmt"
	"strings"
)

// Monomorphize performs a LIR→LIR pass that replaces all generic declarations
// with specialized copies for each unique set of concrete type arguments found
// at call sites. After this pass, no LTyTypeVar remains in the program.
//
// This is required for backends that don't support generics natively (e.g. C).
// The Go backend can skip this pass since Go has native generics.
func Monomorphize(prog *LProgram) {
	m := &monoPass{
		prog:            prog,
		funcInstances:   map[string]map[string][]*LType{},
		classInstances:  map[string]map[string][]*LType{},
		structInstances: map[string]map[string][]*LType{},
		classRenames:    map[string]string{},
		funcByName:      map[string]*LFuncDecl{},
		classByName:     map[string]*LClassDecl{},
		structByName:    map[string]*LStructDecl{},
		methodsByClass:  map[string][]*LFuncDecl{},
	}

	// Index generic declarations
	for i := range prog.Functions {
		f := &prog.Functions[i]
		m.funcByName[m.funcKey(f)] = f
		if f.Receiver != "" {
			m.methodsByClass[f.Receiver] = append(m.methodsByClass[f.Receiver], f)
		}
	}
	for i := range prog.Classes {
		m.classByName[prog.Classes[i].Name] = &prog.Classes[i]
	}
	for i := range prog.Structs {
		m.structByName[prog.Structs[i].Name] = &prog.Structs[i]
	}

	// Phase 1: Collect all instantiations by walking all function bodies
	for i := range prog.Functions {
		m.collectFromStmts(prog.Functions[i].Body)
	}

	// Phase 2: Generate specialized copies
	var newFuncs []LFuncDecl
	var newClasses []LClassDecl
	var newStructs []LStructDecl

	// Specialize generic functions (non-method)
	for funcName, instances := range m.funcInstances {
		orig, ok := m.funcByName[funcName]
		if !ok || len(orig.TypeParams) == 0 {
			continue
		}
		for _, types := range instances {
			if len(types) != len(orig.TypeParams) {
				continue
			}
			subst := map[string]*LType{}
			for i, tp := range orig.TypeParams {
				subst[tp.Name] = types[i]
			}
			mangledName := mangleName(orig.Name, types)
			spec := m.specializeFunc(orig, subst, mangledName, "")
			newFuncs = append(newFuncs, spec)
		}
	}

	// Specialize generic classes + their methods
	for className, instances := range m.classInstances {
		orig, ok := m.classByName[className]
		if !ok || len(orig.TypeParams) == 0 {
			continue
		}
		for _, types := range instances {
			if len(types) != len(orig.TypeParams) {
				continue
			}
			subst := map[string]*LType{}
			for i, tp := range orig.TypeParams {
				subst[tp.Name] = types[i]
			}
			mangledName := mangleName(orig.Name, types)
			m.classRenames[orig.Name] = mangledName
			spec := m.specializeClass(orig, subst, mangledName)
			newClasses = append(newClasses, spec)

			// Specialize all methods of this class
			for _, method := range m.methodsByClass[className] {
				methSubst := map[string]*LType{}
				for k, v := range subst {
					methSubst[k] = v
				}
				specMethod := m.specializeFunc(method, methSubst, method.Name, mangledName)
				newFuncs = append(newFuncs, specMethod)
			}
		}
	}

	// Specialize generic structs
	for structName, instances := range m.structInstances {
		orig, ok := m.structByName[structName]
		if !ok || len(orig.TypeParams) == 0 {
			continue
		}
		for _, types := range instances {
			if len(types) != len(orig.TypeParams) {
				continue
			}
			subst := map[string]*LType{}
			for i, tp := range orig.TypeParams {
				subst[tp.Name] = types[i]
			}
			mangledName := mangleName(orig.Name, types)
			spec := m.specializeStruct(orig, subst, mangledName)
			newStructs = append(newStructs, spec)
		}
	}

	// Phase 3: Remove generic declarations, add specialized ones
	prog.Functions = filterFuncs(prog.Functions, m.funcInstances, m.classInstances, m.methodsByClass)
	prog.Functions = append(prog.Functions, newFuncs...)
	prog.Classes = filterClasses(prog.Classes, m.classInstances)
	prog.Classes = append(prog.Classes, newClasses...)
	prog.Structs = filterStructs(prog.Structs, m.structInstances)
	prog.Structs = append(prog.Structs, newStructs...)

	// Phase 4: Rewrite all call sites in all function bodies
	for i := range prog.Functions {
		m.rewriteStmts(prog.Functions[i].Body)
	}

	// Export rename map for C backend
	prog.ClassRenames = m.classRenames
}

// monoPass holds state for the monomorphization pass.
type monoPass struct {
	prog *LProgram // reference to the program being monomorphized

	// Collected instantiations: name → map of type-key → actual []*LType
	funcInstances   map[string]map[string][]*LType
	classInstances  map[string]map[string][]*LType
	structInstances map[string]map[string][]*LType

	// Map from generic class/struct name → mangled name (when only one instantiation)
	classRenames map[string]string

	// Indexes into the original program
	funcByName     map[string]*LFuncDecl
	classByName    map[string]*LClassDecl
	structByName   map[string]*LStructDecl
	methodsByClass map[string][]*LFuncDecl
}

// funcKey returns the lookup key for a function. Methods use "ClassName.MethodName".
func (m *monoPass) funcKey(f *LFuncDecl) string {
	if f.Receiver != "" {
		return f.Receiver + "." + f.Name
	}
	return f.Name
}

// recordFuncInstance records a function instantiation with its actual type pointers.
func (m *monoPass) recordFuncInstance(name string, types []*LType) {
	key := typeKey(types)
	if m.funcInstances[name] == nil {
		m.funcInstances[name] = map[string][]*LType{}
	}
	if _, exists := m.funcInstances[name][key]; !exists {
		m.funcInstances[name][key] = types
	}
}

// recordClassInstance records a class instantiation with its actual type pointers.
func (m *monoPass) recordClassInstance(name string, types []*LType) {
	key := typeKey(types)
	if m.classInstances[name] == nil {
		m.classInstances[name] = map[string][]*LType{}
	}
	if _, exists := m.classInstances[name][key]; !exists {
		m.classInstances[name][key] = types
	}
}

// ---------------------------------------------------------------------------
// Phase 1: Collect instantiations
// ---------------------------------------------------------------------------

func (m *monoPass) collectFromStmts(stmts []LStmt) {
	for i := range stmts {
		m.collectFromStmt(&stmts[i])
	}
}

func (m *monoPass) collectFromStmt(s *LStmt) {
	switch s.Kind {
	case LStmtTempDef:
		d := s.Data.(*LTempDef)
		m.collectFromExpr(&d.Expr)
	case LStmtVarDecl:
		// no nested exprs to collect from
	case LStmtAssign:
		// LAssign.Value is an LValue (flat), no nested exprs
	case LStmtStructSet:
		// flat values only
	case LStmtClassSet:
		// flat values only
	case LStmtIndexSet:
		// flat values only
	case LStmtReturn:
		// LReturn.Values are flat LValues
	case LStmtIf:
		d := s.Data.(*LIf)
		m.collectFromStmts(d.Then)
		m.collectFromStmts(d.Else)
	case LStmtWhile:
		d := s.Data.(*LWhile)
		m.collectFromStmts(d.CondBlock)
		m.collectFromStmts(d.Body)
	case LStmtFor:
		d := s.Data.(*LFor)
		m.collectFromStmts(d.Body)
	case LStmtSwitch:
		d := s.Data.(*LSwitch)
		for _, c := range d.Cases {
			m.collectFromStmts(c.Body)
		}
	case LStmtTypeSwitch:
		d := s.Data.(*LTypeSwitch)
		for _, c := range d.Cases {
			m.collectFromStmts(c.Body)
		}
	case LStmtBlock:
		d := s.Data.(*LBlock)
		m.collectFromStmts(d.Stmts)
	case LStmtSideEffect:
		d := s.Data.(*LSideEffect)
		m.collectFromExpr(&d.Expr)
	case LStmtMultiAssign:
		d := s.Data.(*LMultiAssign)
		m.collectFromExpr(&d.Expr)
	case LStmtSpawn:
		d := s.Data.(*LSpawn)
		m.collectFromStmts(d.Body)
	case LStmtSelect:
		d := s.Data.(*LSelect)
		for _, c := range d.Cases {
			m.collectFromStmts(c.Body)
		}
	case LStmtDefer:
		d := s.Data.(*LDefer)
		m.collectFromStmts(d.Body)
	case LStmtLock:
		d := s.Data.(*LLock)
		m.collectFromStmts(d.Body)
	}
}

func (m *monoPass) collectFromExpr(e *LExpr) {
	if e == nil {
		return
	}
	switch e.Kind {
	case LExprCall:
		d := e.Data.(*LCallData)
		if len(d.TypeArgs) > 0 && !hasTypeVars(d.TypeArgs) {
			m.recordFuncInstance(d.Func, d.TypeArgs)
		}
	case LExprMethodCall:
		d := e.Data.(*LMethodCallData)
		// Method calls on generic classes: the receiver type tells us the class instantiation
		if d.Receiver.Type != nil && d.Receiver.Type.Kind == LTyClassHandle {
			if cls, ok := m.classByName[d.Receiver.Type.Name]; ok && len(cls.TypeParams) > 0 {
				if len(d.TypeArgs) > 0 && !hasTypeVars(d.TypeArgs) {
					m.recordClassInstance(d.Receiver.Type.Name, d.TypeArgs)
				}
			}
		}
	case LExprClassAlloc:
		d := e.Data.(*LClassAllocData)
		if len(d.TypeArgs) > 0 && !hasTypeVars(d.TypeArgs) {
			m.recordClassInstance(d.Class, d.TypeArgs)
		}
	case LExprStructLit:
		d := e.Data.(*LStructLitData)
		// TODO: struct instantiation tracking (need type args on struct lits)
		_ = d
	case LExprFuncLit:
		d := e.Data.(*LFuncLitData)
		m.collectFromStmts(d.Body)
	case LExprBinOp:
		// flat LValues only
	case LExprUnOp:
		// flat LValue only
	case LExprCast:
		// flat LValue only
	case LExprBuiltin:
		// flat LValues only
	case LExprWrapOptional:
		// flat LValue only
	case LExprVariantData:
		// flat LValue only
	case LExprVariantTag:
		// flat LValue only
	case LExprSlice:
		// flat LValues only
	}
}

// ---------------------------------------------------------------------------
// Phase 2: Specialization (deep clone + type substitution)
// ---------------------------------------------------------------------------

func (m *monoPass) specializeFunc(orig *LFuncDecl, subst map[string]*LType, newName, newReceiver string) LFuncDecl {
	spec := LFuncDecl{
		Name:       newName,
		TypeParams: nil, // No longer generic
		Params:     make([]LParam, len(orig.Params)),
		ReturnType: substType(orig.ReturnType, subst),
		Body:       cloneStmts(orig.Body, subst),
		IsExported: orig.IsExported,
	}
	if newReceiver != "" {
		spec.Receiver = newReceiver
	}
	for i, p := range orig.Params {
		spec.Params[i] = LParam{
			Name: p.Name,
			Type: substType(p.Type, subst),
		}
	}
	return spec
}

func (m *monoPass) specializeClass(orig *LClassDecl, subst map[string]*LType, newName string) LClassDecl {
	spec := LClassDecl{
		Name:       newName,
		TypeParams: nil,
		Fields:     make([]LField, len(orig.Fields)),
		GuardedBy:  orig.GuardedBy,
		HasFinal:   orig.HasFinal,
		IsExported: orig.IsExported,
	}
	for i, f := range orig.Fields {
		spec.Fields[i] = LField{
			Name: f.Name,
			Type: substType(f.Type, subst),
		}
	}
	return spec
}

func (m *monoPass) specializeStruct(orig *LStructDecl, subst map[string]*LType, newName string) LStructDecl {
	spec := LStructDecl{
		Name:       newName,
		TypeParams: nil,
		Fields:     make([]LField, len(orig.Fields)),
		IsExported: orig.IsExported,
	}
	for i, f := range orig.Fields {
		spec.Fields[i] = LField{
			Name: f.Name,
			Type: substType(f.Type, subst),
		}
	}
	return spec
}

// ---------------------------------------------------------------------------
// Phase 4: Rewrite call sites
// ---------------------------------------------------------------------------

func (m *monoPass) rewriteStmts(stmts []LStmt) {
	for i := range stmts {
		m.rewriteStmt(&stmts[i])
	}
}

func (m *monoPass) rewriteStmt(s *LStmt) {
	switch s.Kind {
	case LStmtTempDef:
		d := s.Data.(*LTempDef)
		m.rewriteExpr(&d.Expr)
		d.Expr.Type = m.substTypeRemoveVars(d.Expr.Type)
	case LStmtVarDecl:
		d := s.Data.(*LVarDecl)
		d.Type = m.substTypeRemoveVars(d.Type)
		if d.Init != nil {
			d.Init.Type = m.substTypeRemoveVars(d.Init.Type)
		}
	case LStmtAssign:
		d := s.Data.(*LAssign)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
	case LStmtStructSet:
		d := s.Data.(*LStructSet)
		d.Receiver.Type = m.substTypeRemoveVars(d.Receiver.Type)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
	case LStmtClassSet:
		d := s.Data.(*LClassSet)
		d.Handle.Type = m.substTypeRemoveVars(d.Handle.Type)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
	case LStmtIndexSet:
		d := s.Data.(*LIndexSet)
		d.Collection.Type = m.substTypeRemoveVars(d.Collection.Type)
		d.Index.Type = m.substTypeRemoveVars(d.Index.Type)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
	case LStmtReturn:
		d := s.Data.(*LReturn)
		for i := range d.Values {
			d.Values[i].Type = m.substTypeRemoveVars(d.Values[i].Type)
		}
	case LStmtIf:
		d := s.Data.(*LIf)
		d.Cond.Type = m.substTypeRemoveVars(d.Cond.Type)
		m.rewriteStmts(d.Then)
		m.rewriteStmts(d.Else)
	case LStmtWhile:
		d := s.Data.(*LWhile)
		m.rewriteStmts(d.CondBlock)
		d.CondVar.Type = m.substTypeRemoveVars(d.CondVar.Type)
		m.rewriteStmts(d.Body)
	case LStmtFor:
		d := s.Data.(*LFor)
		d.VarType = m.substTypeRemoveVars(d.VarType)
		d.Collection.Type = m.substTypeRemoveVars(d.Collection.Type)
		m.rewriteStmts(d.Body)
	case LStmtSwitch:
		d := s.Data.(*LSwitch)
		d.Tag.Type = m.substTypeRemoveVars(d.Tag.Type)
		for i := range d.Cases {
			m.rewriteStmts(d.Cases[i].Body)
		}
	case LStmtTypeSwitch:
		d := s.Data.(*LTypeSwitch)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
		for i := range d.Cases {
			d.Cases[i].Type = m.substTypeRemoveVars(d.Cases[i].Type)
			m.rewriteStmts(d.Cases[i].Body)
		}
	case LStmtBlock:
		d := s.Data.(*LBlock)
		m.rewriteStmts(d.Stmts)
	case LStmtSideEffect:
		d := s.Data.(*LSideEffect)
		m.rewriteExpr(&d.Expr)
	case LStmtMultiAssign:
		d := s.Data.(*LMultiAssign)
		m.rewriteExpr(&d.Expr)
		for i := range d.Types {
			d.Types[i] = m.substTypeRemoveVars(d.Types[i])
		}
	case LStmtSpawn:
		d := s.Data.(*LSpawn)
		m.rewriteStmts(d.Body)
	case LStmtSelect:
		d := s.Data.(*LSelect)
		for i := range d.Cases {
			d.Cases[i].Channel.Type = m.substTypeRemoveVars(d.Cases[i].Channel.Type)
			d.Cases[i].Value.Type = m.substTypeRemoveVars(d.Cases[i].Value.Type)
			m.rewriteStmts(d.Cases[i].Body)
		}
	case LStmtDefer:
		d := s.Data.(*LDefer)
		m.rewriteStmts(d.Body)
	case LStmtLock:
		d := s.Data.(*LLock)
		m.rewriteStmts(d.Body)
	case LStmtSend:
		d := s.Data.(*LSend)
		d.Channel.Type = m.substTypeRemoveVars(d.Channel.Type)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
	}
}

func (m *monoPass) rewriteExpr(e *LExpr) {
	if e == nil {
		return
	}
	e.Type = m.substTypeRemoveVars(e.Type)
	switch e.Kind {
	case LExprCall:
		d := e.Data.(*LCallData)
		if len(d.TypeArgs) > 0 && !hasTypeVars(d.TypeArgs) {
			d.Func = mangleName(d.Func, d.TypeArgs)
			d.TypeArgs = nil
		}
		for i := range d.Args {
			d.Args[i].Type = m.substTypeRemoveVars(d.Args[i].Type)
		}
	case LExprMethodCall:
		d := e.Data.(*LMethodCallData)
		d.Receiver.Type = m.substTypeRemoveVars(d.Receiver.Type)
		// If receiver is a generic class, rewrite receiver type name
		if d.Receiver.Type != nil && d.Receiver.Type.Kind == LTyClassHandle {
			if _, ok := m.classInstances[d.Receiver.Type.Name]; ok {
				if len(d.TypeArgs) > 0 && !hasTypeVars(d.TypeArgs) {
					mangledClass := mangleName(d.Receiver.Type.Name, d.TypeArgs)
					d.Receiver.Type = &LType{
						Kind:       LTyClassHandle,
						Name:       mangledClass,
						IsExported: d.Receiver.Type.IsExported,
					}
					d.TypeArgs = nil
				}
			}
			// Update expression type from specialized function's return type
			recvName := d.Receiver.Type.Name
			if renamed, ok := m.classRenames[recvName]; ok {
				recvName = renamed
			}
			for _, fn := range m.prog.Functions {
				if fn.Receiver == recvName && fn.Name == d.Method {
					if fn.ReturnType != nil {
						e.Type = fn.ReturnType
					}
					break
				}
			}
		}
		for i := range d.Args {
			d.Args[i].Type = m.substTypeRemoveVars(d.Args[i].Type)
		}
	case LExprClassAlloc:
		d := e.Data.(*LClassAllocData)
		if len(d.TypeArgs) > 0 && !hasTypeVars(d.TypeArgs) {
			mangledName := mangleName(d.Class, d.TypeArgs)
			d.Class = mangledName
			e.Type = &LType{
				Kind:       LTyClassHandle,
				Name:       mangledName,
				IsExported: e.Type.IsExported,
			}
			d.TypeArgs = nil
		}
		for i := range d.Fields {
			d.Fields[i].Value.Type = m.substTypeRemoveVars(d.Fields[i].Value.Type)
		}
	case LExprBinOp:
		d := e.Data.(*LBinOpData)
		d.Left.Type = m.substTypeRemoveVars(d.Left.Type)
		d.Right.Type = m.substTypeRemoveVars(d.Right.Type)
	case LExprUnOp:
		d := e.Data.(*LUnOpData)
		d.Operand.Type = m.substTypeRemoveVars(d.Operand.Type)
	case LExprCast:
		d := e.Data.(*LCastData)
		d.Target = m.substTypeRemoveVars(d.Target)
		d.Operand.Type = m.substTypeRemoveVars(d.Operand.Type)
	case LExprBuiltin:
		d := e.Data.(*LBuiltinData)
		for i := range d.Args {
			d.Args[i].Type = m.substTypeRemoveVars(d.Args[i].Type)
		}
	case LExprFuncLit:
		d := e.Data.(*LFuncLitData)
		d.ReturnType = m.substTypeRemoveVars(d.ReturnType)
		for i := range d.Params {
			d.Params[i].Type = m.substTypeRemoveVars(d.Params[i].Type)
		}
		m.rewriteStmts(d.Body)
	case LExprWrapOptional:
		d := e.Data.(*LWrapOptionalData)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
	case LExprVariantConstruct:
		d := e.Data.(*LVariantConstructData)
		for i := range d.Fields {
			d.Fields[i].Type = m.substTypeRemoveVars(d.Fields[i].Type)
		}
	case LExprVariantData:
		d := e.Data.(*LVariantDataData)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
	case LExprVariantTag:
		d := e.Data.(*LVariantTagData)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
	case LExprSlice:
		d := e.Data.(*LSliceData)
		d.Collection.Type = m.substTypeRemoveVars(d.Collection.Type)
	case LExprStructLit:
		d := e.Data.(*LStructLitData)
		for i := range d.Fields {
			d.Fields[i].Value.Type = m.substTypeRemoveVars(d.Fields[i].Value.Type)
		}
	case LExprStructField:
		d := e.Data.(*LStructFieldData)
		d.Receiver.Type = m.substTypeRemoveVars(d.Receiver.Type)
	case LExprClassGet:
		d := e.Data.(*LClassGetData)
		d.Handle.Type = m.substTypeRemoveVars(d.Handle.Type)
	case LExprIndexGet:
		d := e.Data.(*LIndexGetData)
		d.Collection.Type = m.substTypeRemoveVars(d.Collection.Type)
		d.Index.Type = m.substTypeRemoveVars(d.Index.Type)
	case LExprExtractValue:
		d := e.Data.(*LExtractValueData)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
	case LExprExtractError:
		d := e.Data.(*LExtractErrorData)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
	case LExprMakeResult:
		d := e.Data.(*LMakeResultData)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
		d.Err.Type = m.substTypeRemoveVars(d.Err.Type)
	case LExprIsNull:
		d := e.Data.(*LIsNullData)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
	case LExprUnwrapOptional:
		d := e.Data.(*LUnwrapOptionalData)
		d.Value.Type = m.substTypeRemoveVars(d.Value.Type)
	case LExprMakeSlice:
		// no LValues to rewrite
	case LExprMakeMap:
		// no LValues to rewrite
	case LExprMakeChannel:
		d := e.Data.(*LMakeChannelData)
		d.ElemType = m.substTypeRemoveVars(d.ElemType)
	case LExprFormat:
		d := e.Data.(*LFormatData)
		for i := range d.Parts {
			if !d.Parts[i].IsLiteral {
				d.Parts[i].Value.Type = m.substTypeRemoveVars(d.Parts[i].Value.Type)
			}
		}
	case LExprEnvGet:
		d := e.Data.(*LEnvGetData)
		d.Env.Type = m.substTypeRemoveVars(d.Env.Type)
	case LExprFuncRef:
		d := e.Data.(*LFuncRefData)
		if d.Env != nil {
			d.Env.Type = m.substTypeRemoveVars(d.Env.Type)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers: type substitution
// ---------------------------------------------------------------------------

// substType replaces all LTyTypeVar references with their concrete types from subst.
func substType(t *LType, subst map[string]*LType) *LType {
	if t == nil {
		return nil
	}
	if t.Kind == LTyTypeVar {
		if concrete, ok := subst[t.Name]; ok {
			return concrete
		}
		return t
	}
	// Deep copy with substitution
	out := &LType{
		Kind:       t.Kind,
		Name:       t.Name,
		Bits:       t.Bits,
		IsExported: t.IsExported,
	}
	out.Elem = substType(t.Elem, subst)
	out.Key = substType(t.Key, subst)
	out.Return = substType(t.Return, subst)
	if len(t.Fields) > 0 {
		out.Fields = make([]LField, len(t.Fields))
		for i, f := range t.Fields {
			out.Fields[i] = LField{Name: f.Name, Type: substType(f.Type, subst)}
		}
	}
	if len(t.Params) > 0 {
		out.Params = make([]*LType, len(t.Params))
		for i, p := range t.Params {
			out.Params[i] = substType(p, subst)
		}
	}
	if len(t.Variants) > 0 {
		out.Variants = make([]LVariant, len(t.Variants))
		for i, v := range t.Variants {
			out.Variants[i] = LVariant{Name: v.Name, Tag: v.Tag}
			if len(v.Fields) > 0 {
				out.Variants[i].Fields = make([]LField, len(v.Fields))
				for j, f := range v.Fields {
					out.Variants[i].Fields[j] = LField{Name: f.Name, Type: substType(f.Type, subst)}
				}
			}
		}
	}
	return out
}

// substTypeRemoveVars is used in the rewrite phase. After monomorphization,
// any remaining type vars should already be gone. This is a safety net.
func (m *monoPass) substTypeRemoveVars(t *LType) *LType {
	return t
}




// ---------------------------------------------------------------------------
// Helpers: cloning statements with type substitution
// ---------------------------------------------------------------------------

func cloneStmts(stmts []LStmt, subst map[string]*LType) []LStmt {
	if stmts == nil {
		return nil
	}
	out := make([]LStmt, len(stmts))
	for i := range stmts {
		out[i] = cloneStmt(&stmts[i], subst)
	}
	return out
}

func cloneStmt(s *LStmt, subst map[string]*LType) LStmt {
	out := LStmt{Kind: s.Kind}
	switch s.Kind {
	case LStmtTempDef:
		d := s.Data.(*LTempDef)
		out.Data = &LTempDef{
			ID:   d.ID,
			Expr: cloneExpr(&d.Expr, subst),
		}
	case LStmtVarDecl:
		d := s.Data.(*LVarDecl)
		vd := &LVarDecl{
			Name:    d.Name,
			Type:    substType(d.Type, subst),
			Mutable: d.Mutable,
		}
		if d.Init != nil {
			init := cloneValue(d.Init, subst)
			vd.Init = &init
		}
		out.Data = vd
	case LStmtAssign:
		d := s.Data.(*LAssign)
		val := cloneValue(&d.Value, subst)
		out.Data = &LAssign{Target: d.Target, Value: val}
	case LStmtStructSet:
		d := s.Data.(*LStructSet)
		out.Data = &LStructSet{
			Receiver: cloneValue(&d.Receiver, subst),
			Field:    d.Field,
			Value:    cloneValue(&d.Value, subst),
		}
	case LStmtClassSet:
		d := s.Data.(*LClassSet)
		out.Data = &LClassSet{
			Handle: cloneValue(&d.Handle, subst),
			Class:  d.Class,
			Field:  d.Field,
			Value:  cloneValue(&d.Value, subst),
		}
	case LStmtIndexSet:
		d := s.Data.(*LIndexSet)
		out.Data = &LIndexSet{
			Collection: cloneValue(&d.Collection, subst),
			Index:      cloneValue(&d.Index, subst),
			Value:      cloneValue(&d.Value, subst),
		}
	case LStmtReturn:
		d := s.Data.(*LReturn)
		ret := &LReturn{}
		if len(d.Values) > 0 {
			ret.Values = make([]LValue, len(d.Values))
			for i := range d.Values {
				ret.Values[i] = cloneValue(&d.Values[i], subst)
			}
		}
		out.Data = ret
	case LStmtIf:
		d := s.Data.(*LIf)
		out.Data = &LIf{
			Cond: cloneValue(&d.Cond, subst),
			Then: cloneStmts(d.Then, subst),
			Else: cloneStmts(d.Else, subst),
		}
	case LStmtWhile:
		d := s.Data.(*LWhile)
		out.Data = &LWhile{
			CondBlock: cloneStmts(d.CondBlock, subst),
			CondVar:   cloneValue(&d.CondVar, subst),
			Body:      cloneStmts(d.Body, subst),
		}
	case LStmtFor:
		d := s.Data.(*LFor)
		out.Data = &LFor{
			Var:        d.Var,
			VarType:    substType(d.VarType, subst),
			IndexVar:   d.IndexVar,
			Collection: cloneValue(&d.Collection, subst),
			Body:       cloneStmts(d.Body, subst),
		}
	case LStmtSwitch:
		d := s.Data.(*LSwitch)
		cases := make([]LSwitchCase, len(d.Cases))
		for i, c := range d.Cases {
			cases[i] = LSwitchCase{
				Tag:     c.Tag,
				Binding: c.Binding,
				Body:    cloneStmts(c.Body, subst),
			}
		}
		out.Data = &LSwitch{Tag: cloneValue(&d.Tag, subst), Cases: cases, EnumName: d.EnumName}
	case LStmtTypeSwitch:
		d := s.Data.(*LTypeSwitch)
		cases := make([]LTypeSwitchCase, len(d.Cases))
		for i, c := range d.Cases {
			cases[i] = LTypeSwitchCase{
				Type:    substType(c.Type, subst),
				Binding: c.Binding,
				Body:    cloneStmts(c.Body, subst),
			}
		}
		out.Data = &LTypeSwitch{Value: cloneValue(&d.Value, subst), Cases: cases}
	case LStmtBlock:
		d := s.Data.(*LBlock)
		out.Data = &LBlock{Stmts: cloneStmts(d.Stmts, subst)}
	case LStmtSideEffect:
		d := s.Data.(*LSideEffect)
		out.Data = &LSideEffect{Expr: cloneExpr(&d.Expr, subst)}
	case LStmtMultiAssign:
		d := s.Data.(*LMultiAssign)
		out.Data = &LMultiAssign{
			Names: append([]string{}, d.Names...),
			Types: substTypes(d.Types, subst),
			Expr:  cloneExpr(&d.Expr, subst),
		}
	case LStmtSpawn:
		d := s.Data.(*LSpawn)
		out.Data = &LSpawn{
			Body:     cloneStmts(d.Body, subst),
			Captures: append([]string{}, d.Captures...),
		}
	case LStmtSelect:
		d := s.Data.(*LSelect)
		cases := make([]LSelectCase, len(d.Cases))
		for i, c := range d.Cases {
			cases[i] = LSelectCase{
				Kind:    c.Kind,
				Channel: cloneValue(&c.Channel, subst),
				Value:   cloneValue(&c.Value, subst),
				Binding: c.Binding,
				Body:    cloneStmts(c.Body, subst),
			}
		}
		out.Data = &LSelect{Cases: cases}
	case LStmtDefer:
		d := s.Data.(*LDefer)
		out.Data = &LDefer{Body: cloneStmts(d.Body, subst)}
	case LStmtLock:
		d := s.Data.(*LLock)
		out.Data = &LLock{
			Mutex: cloneValue(&d.Mutex, subst),
			Body:  cloneStmts(d.Body, subst),
		}
	case LStmtSend:
		d := s.Data.(*LSend)
		out.Data = &LSend{
			Channel: cloneValue(&d.Channel, subst),
			Value:   cloneValue(&d.Value, subst),
		}
	case LStmtExpr:
		d := s.Data.(*LExprStmt)
		out.Data = &LExprStmt{TempID: d.TempID}
	case LStmtBreak, LStmtContinue:
		out.Data = s.Data
	default:
		out.Data = s.Data
	}
	return out
}

// cloneValue clones an LValue with type substitution. LValues are flat structs.
func cloneValue(v *LValue, subst map[string]*LType) LValue {
	return LValue{
		Kind:     v.Kind,
		Name:     v.Name,
		TempID:   v.TempID,
		IntVal:   v.IntVal,
		UintVal:  v.UintVal,
		FloatVal: v.FloatVal,
		StrVal:   v.StrVal,
		BoolVal:  v.BoolVal,
		Type:     substType(v.Type, subst),
	}
}

func cloneExpr(e *LExpr, subst map[string]*LType) LExpr {
	out := LExpr{
		Kind: e.Kind,
		Type: substType(e.Type, subst),
	}
	switch e.Kind {
	case LExprCall:
		d := e.Data.(*LCallData)
		out.Data = &LCallData{
			Func:       d.Func,
			Args:       cloneValues(d.Args, subst),
			TypeArgs:   substTypes(d.TypeArgs, subst),
			IsExported: d.IsExported,
		}
	case LExprMethodCall:
		d := e.Data.(*LMethodCallData)
		recv := cloneValue(&d.Receiver, subst)
		out.Data = &LMethodCallData{
			Receiver:   recv,
			Method:     d.Method,
			Args:       cloneValues(d.Args, subst),
			TypeArgs:   substTypes(d.TypeArgs, subst),
			IsExported: d.IsExported,
		}
	case LExprClassAlloc:
		d := e.Data.(*LClassAllocData)
		fields := make([]LFieldInit, len(d.Fields))
		for i, f := range d.Fields {
			fields[i] = LFieldInit{Name: f.Name, Value: cloneValue(&f.Value, subst)}
		}
		out.Data = &LClassAllocData{
			Class:    d.Class,
			Fields:   fields,
			TypeArgs: substTypes(d.TypeArgs, subst),
		}
	case LExprStructLit:
		d := e.Data.(*LStructLitData)
		fields := make([]LFieldInit, len(d.Fields))
		for i, f := range d.Fields {
			fields[i] = LFieldInit{Name: f.Name, Value: cloneValue(&f.Value, subst)}
		}
		out.Data = &LStructLitData{Fields: fields}
	case LExprBinOp:
		d := e.Data.(*LBinOpData)
		out.Data = &LBinOpData{Op: d.Op, Left: cloneValue(&d.Left, subst), Right: cloneValue(&d.Right, subst)}
	case LExprUnOp:
		d := e.Data.(*LUnOpData)
		out.Data = &LUnOpData{Op: d.Op, Operand: cloneValue(&d.Operand, subst)}
	case LExprCast:
		d := e.Data.(*LCastData)
		out.Data = &LCastData{Target: substType(d.Target, subst), Operand: cloneValue(&d.Operand, subst)}
	case LExprBuiltin:
		d := e.Data.(*LBuiltinData)
		out.Data = &LBuiltinData{Name: d.Name, Args: cloneValues(d.Args, subst)}
	case LExprFuncLit:
		d := e.Data.(*LFuncLitData)
		params := make([]LParam, len(d.Params))
		for i, p := range d.Params {
			params[i] = LParam{Name: p.Name, Type: substType(p.Type, subst)}
		}
		out.Data = &LFuncLitData{
			Params:     params,
			ReturnType: substType(d.ReturnType, subst),
			Body:       cloneStmts(d.Body, subst),
		}
	case LExprWrapOptional:
		d := e.Data.(*LWrapOptionalData)
		out.Data = &LWrapOptionalData{Value: cloneValue(&d.Value, subst)}
	case LExprUnwrapOptional:
		d := e.Data.(*LUnwrapOptionalData)
		out.Data = &LUnwrapOptionalData{Value: cloneValue(&d.Value, subst)}
	case LExprIsNull:
		d := e.Data.(*LIsNullData)
		out.Data = &LIsNullData{Value: cloneValue(&d.Value, subst)}
	case LExprVariantConstruct:
		d := e.Data.(*LVariantConstructData)
		fields := make([]LValue, len(d.Fields))
		for i := range d.Fields {
			fields[i] = cloneValue(&d.Fields[i], subst)
		}
		out.Data = &LVariantConstructData{Enum: d.Enum, Variant: d.Variant, Tag: d.Tag, Fields: fields}
	case LExprVariantData:
		d := e.Data.(*LVariantDataData)
		out.Data = &LVariantDataData{Value: cloneValue(&d.Value, subst), Enum: d.Enum, Variant: d.Variant, Field: d.Field}
	case LExprVariantTag:
		d := e.Data.(*LVariantTagData)
		out.Data = &LVariantTagData{Value: cloneValue(&d.Value, subst)}
	case LExprStructField:
		d := e.Data.(*LStructFieldData)
		out.Data = &LStructFieldData{Receiver: cloneValue(&d.Receiver, subst), Field: d.Field}
	case LExprClassGet:
		d := e.Data.(*LClassGetData)
		out.Data = &LClassGetData{Handle: cloneValue(&d.Handle, subst), Class: d.Class, Field: d.Field}
	case LExprIndexGet:
		d := e.Data.(*LIndexGetData)
		out.Data = &LIndexGetData{Collection: cloneValue(&d.Collection, subst), Index: cloneValue(&d.Index, subst)}
	case LExprSlice:
		d := e.Data.(*LSliceData)
		sd := &LSliceData{Collection: cloneValue(&d.Collection, subst)}
		if d.Low != nil {
			l := cloneValue(d.Low, subst)
			sd.Low = &l
		}
		if d.High != nil {
			h := cloneValue(d.High, subst)
			sd.High = &h
		}
		out.Data = sd
	case LExprExtractValue:
		d := e.Data.(*LExtractValueData)
		out.Data = &LExtractValueData{Value: cloneValue(&d.Value, subst)}
	case LExprExtractError:
		d := e.Data.(*LExtractErrorData)
		out.Data = &LExtractErrorData{Value: cloneValue(&d.Value, subst)}
	case LExprMakeResult:
		d := e.Data.(*LMakeResultData)
		out.Data = &LMakeResultData{Value: cloneValue(&d.Value, subst), Err: cloneValue(&d.Err, subst)}
	case LExprMakeSlice:
		out.Data = e.Data // no LValues
	case LExprMakeMap:
		out.Data = e.Data // no LValues
	case LExprMakeChannel:
		d := e.Data.(*LMakeChannelData)
		cd := &LMakeChannelData{ElemType: substType(d.ElemType, subst)}
		if d.BufSize != nil {
			bs := cloneValue(d.BufSize, subst)
			cd.BufSize = &bs
		}
		out.Data = cd
	case LExprFormat:
		d := e.Data.(*LFormatData)
		parts := make([]LFormatPart, len(d.Parts))
		for i, p := range d.Parts {
			parts[i] = p
			if !p.IsLiteral {
				parts[i].Value = cloneValue(&p.Value, subst)
			}
		}
		out.Data = &LFormatData{Parts: parts}
	case LExprEnvGet:
		d := e.Data.(*LEnvGetData)
		out.Data = &LEnvGetData{Env: cloneValue(&d.Env, subst), Field: d.Field}
	case LExprFuncRef:
		d := e.Data.(*LFuncRefData)
		fr := &LFuncRefData{Name: d.Name}
		if d.Env != nil {
			env := cloneValue(d.Env, subst)
			fr.Env = &env
		}
		out.Data = fr
	default:
		out.Data = e.Data
	}
	return out
}

func cloneValues(vs []LValue, subst map[string]*LType) []LValue {
	if vs == nil {
		return nil
	}
	out := make([]LValue, len(vs))
	for i := range vs {
		out[i] = cloneValue(&vs[i], subst)
	}
	return out
}

func substTypes(ts []*LType, subst map[string]*LType) []*LType {
	if ts == nil {
		return nil
	}
	out := make([]*LType, len(ts))
	for i, t := range ts {
		out[i] = substType(t, subst)
	}
	return out
}

// ---------------------------------------------------------------------------
// Helpers: name mangling
// ---------------------------------------------------------------------------

// mangleName creates a specialized name like "identity_i32" or "Stack_i32_string".
func mangleName(base string, typeArgs []*LType) string {
	var parts []string
	for _, t := range typeArgs {
		parts = append(parts, typeToMangle(t))
	}
	return base + "_" + strings.Join(parts, "_")
}

func typeToMangle(t *LType) string {
	switch t.Kind {
	case LTyI8:
		return "i8"
	case LTyI16:
		return "i16"
	case LTyI32:
		return "i32"
	case LTyI64:
		return "i64"
	case LTyU8:
		return "u8"
	case LTyU16:
		return "u16"
	case LTyU32:
		return "u32"
	case LTyU64:
		return "u64"
	case LTyF32:
		return "f32"
	case LTyF64:
		return "f64"
	case LTyBool:
		return "bool"
	case LTyString:
		return "string"
	case LTyStruct:
		return "S" + t.Name
	case LTyClassHandle:
		return "C" + t.Name
	case LTySlice:
		return "slice_" + typeToMangle(t.Elem)
	case LTyMap:
		return "map_" + typeToMangle(t.Key) + "_" + typeToMangle(t.Elem)
	case LTyOptional:
		return "opt_" + typeToMangle(t.Elem)
	case LTyPlatformInt:
		return "int"
	case LTyPlatformUint:
		return "uint"
	default:
		return fmt.Sprintf("t%d", t.Kind)
	}
}

// typeKey creates a string key for a set of type arguments, for deduplication.
func typeKey(types []*LType) string {
	var parts []string
	for _, t := range types {
		parts = append(parts, typeToMangle(t))
	}
	return strings.Join(parts, ",")
}

// hasTypeVars returns true if any of the types contain unresolved type variables.
func hasTypeVars(types []*LType) bool {
	for _, t := range types {
		if typeHasVar(t) {
			return true
		}
	}
	return false
}

func typeHasVar(t *LType) bool {
	if t == nil {
		return false
	}
	if t.Kind == LTyTypeVar {
		return true
	}
	if typeHasVar(t.Elem) || typeHasVar(t.Key) || typeHasVar(t.Return) {
		return true
	}
	for _, p := range t.Params {
		if typeHasVar(p) {
			return true
		}
	}
	for _, f := range t.Fields {
		if typeHasVar(f.Type) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Helpers: filtering out generic declarations
// ---------------------------------------------------------------------------

func filterFuncs(funcs []LFuncDecl, funcInst map[string]map[string][]*LType, classInst map[string]map[string][]*LType, methodsByClass map[string][]*LFuncDecl) []LFuncDecl {
	var out []LFuncDecl
	for _, f := range funcs {
		if len(f.TypeParams) > 0 && f.Receiver == "" {
			if _, ok := funcInst[f.Name]; ok {
				continue
			}
		}
		if f.Receiver != "" && len(f.ReceiverTypeParams) > 0 {
			if _, ok := classInst[f.Receiver]; ok {
				continue
			}
		}
		out = append(out, f)
	}
	return out
}

func filterClasses(classes []LClassDecl, classInst map[string]map[string][]*LType) []LClassDecl {
	var out []LClassDecl
	for _, c := range classes {
		if len(c.TypeParams) > 0 {
			if _, ok := classInst[c.Name]; ok {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

func filterStructs(structs []LStructDecl, structInst map[string]map[string][]*LType) []LStructDecl {
	var out []LStructDecl
	for _, s := range structs {
		if len(s.TypeParams) > 0 {
			if _, ok := structInst[s.Name]; ok {
				continue
			}
		}
		out = append(out, s)
	}
	return out
}
