package lir

import (
	"fmt"
	"strings"
)

// GoBackend emits Go source code from an LIR program.
// This should be under 1,500 lines — all semantic complexity
// lives in the lowering pass.
type GoBackend struct {
	buf     strings.Builder
	indent  int
	imports map[string]string // path → alias (empty = no alias)
	prog    *LProgram         // reference to the program being emitted

	// For method receivers
	currentReceiver string

	// Enum type switch support
	tempDefs      map[int]*LTempDef
	enumVariants  map[string][]LVariant
	typeSwitchVar string

	// Visibility tracking (populated from declarations)
	nameExported   map[string]bool // type/func/method name → is exported
	suppressedTemps map[int]bool   // temps consumed by field-writeback patterns
	needsIsNilHelper bool         // emit _isNil helper for generic nil checks
	needsHashStringHelper bool    // emit _hashString helper for FNV-1a
	inGenericFunc    bool         // true when emitting a function with type params
	currentFuncReturnType *LType // return type of the function currently being emitted
}

// scanFieldWritebacks pre-scans statements to find temps that will be consumed
// by field-writeback patterns (append on class/struct fields). These temps are
// suppressed during emission since the backend re-accesses the field directly.
func (g *GoBackend) scanFieldWritebacks(stmts []LStmt) {
	// First pass: collect temp definitions
	tempDefs := make(map[int]*LTempDef)
	for _, s := range stmts {
		if s.Kind == LStmtTempDef {
			td := s.Data.(*LTempDef)
			tempDefs[td.ID] = td
		}
	}
	// Second pass: find SideEffect(append(temp, ...)) where temp is a field access
	for _, s := range stmts {
		if s.Kind == LStmtSideEffect {
			se := s.Data.(*LSideEffect)
			if se.Expr.Kind == LExprBuiltin {
				d := se.Expr.Data.(*LBuiltinData)
				if (d.Name == "append" || d.Name == "slice_push") && len(d.Args) > 0 && d.Args[0].Kind == LValTemp {
					if td, ok := tempDefs[d.Args[0].TempID]; ok {
						if td.Expr.Kind == LExprStructField || td.Expr.Kind == LExprClassGet {
							g.suppressedTemps[d.Args[0].TempID] = true
						}
					}
				}
			}
		}
	}
}

// EmitGo converts an LIR program to Go source code.
func EmitGo(prog *LProgram) string {
	g := &GoBackend{
		imports:         make(map[string]string),
		tempDefs:        make(map[int]*LTempDef),
		enumVariants:    make(map[string][]LVariant),
		nameExported:    make(map[string]bool),
		suppressedTemps: make(map[int]bool),
		prog:            prog,
	}
	return g.emit(prog)
}

func (g *GoBackend) emit(prog *LProgram) string {
	// Collect imports
	for _, imp := range prog.Imports {
		g.imports[imp.Path] = imp.Alias
	}

	// Build visibility map from all declarations
	g.buildVisibilityMap(prog)

	// First pass: emit everything to a buffer to discover auto-imports
	var body strings.Builder

	// Type definitions
	for _, td := range prog.TypeDefs {
		g.buf.Reset()
		g.emitTypeDef(&td)
		body.WriteString(g.buf.String())
	}

	// Structs
	for _, s := range prog.Structs {
		g.buf.Reset()
		g.emitStructDecl(&s)
		body.WriteString(g.buf.String())
	}

	// Enums (tagged unions)
	for _, e := range prog.Enums {
		g.buf.Reset()
		g.emitEnumDecl(&e)
		body.WriteString(g.buf.String())
	}

	// Interfaces
	for _, iface := range prog.Interfaces {
		g.buf.Reset()
		g.emitInterfaceDecl(&iface)
		body.WriteString(g.buf.String())
	}

	// Classes (Go structs with pointer receivers)
	for _, c := range prog.Classes {
		g.buf.Reset()
		g.emitClassDecl(&c)
		body.WriteString(g.buf.String())
	}

	// Functions
	for _, f := range prog.Functions {
		g.buf.Reset()
		g.emitFuncDecl(&f)
		body.WriteString(g.buf.String())
	}

	// Build final output with package + imports + body
	var out strings.Builder
	out.WriteString(fmt.Sprintf("package %s\n\n", prog.Package))

	if len(g.imports) > 0 {
		out.WriteString("import (\n")
		for path, alias := range g.imports {
			if alias != "" {
				out.WriteString(fmt.Sprintf("\t%s %q\n", alias, path))
			} else {
				out.WriteString(fmt.Sprintf("\t%q\n", path))
			}
		}
		out.WriteString(")\n\n")
	}

	// Emit _isNil helper for generic nil checks if needed
	if g.needsIsNilHelper {
		out.WriteString("func _isNil(v any) bool {\n")
		out.WriteString("\tif v == nil { return true }\n")
		out.WriteString("\trv := reflect.ValueOf(v)\n")
		out.WriteString("\treturn rv.Kind() == reflect.Ptr && rv.IsNil()\n")
		out.WriteString("}\n\n")
	}

	// Emit _hashString helper for FNV-1a hashing
	if g.needsHashStringHelper {
		out.WriteString("func _hashString(s string) uint64 {\n")
		out.WriteString("\th := fnv.New64a()\n")
		out.WriteString("\th.Write([]byte(s))\n")
		out.WriteString("\treturn h.Sum64()\n")
		out.WriteString("}\n\n")
	}

	out.WriteString(body.String())
	return out.String()
}

// buildVisibilityMap populates nameExported from all declarations in the program.
func (g *GoBackend) buildVisibilityMap(prog *LProgram) {
	for _, s := range prog.Structs {
		g.nameExported[s.Name] = s.IsExported
	}
	for _, c := range prog.Classes {
		g.nameExported[c.Name] = c.IsExported
	}
	for _, e := range prog.Enums {
		g.nameExported[e.Name] = e.IsExported
		for _, v := range e.Variants {
			g.nameExported[v.Name] = e.IsExported
		}
	}
	for _, td := range prog.TypeDefs {
		g.nameExported[td.Name] = td.IsExported
	}
	for _, iface := range prog.Interfaces {
		g.nameExported[iface.Name] = iface.IsExported
	}
	for _, f := range prog.Functions {
		g.nameExported[f.Name] = f.IsExported
		if f.Receiver != "" {
			// Register method name visibility
			g.nameExported[f.Name] = f.IsExported
		}
	}
}

// ---------------------------------------------------------------------------
// Type emission
// ---------------------------------------------------------------------------

// goTypeForIface emits a Go type string for use in interface method signatures.
// In this context, Optional(TypeVar) collapses to just TypeVar because Go generic
// constraints will be instantiated with pointer types (classes), so *C would become
// **file instead of *file.
func (g *GoBackend) goTypeForIface(t *LType) string {
	if t != nil && t.Kind == LTyOptional && t.Elem != nil && t.Elem.Kind == LTyTypeVar {
		return g.goType(t.Elem)
	}
	return g.goType(t)
}

func (g *GoBackend) goType(t *LType) string {
	if t == nil {
		return ""
	}
	switch t.Kind {
	case LTyI8:
		return "int8"
	case LTyI16:
		return "int16"
	case LTyI32:
		return "int32"
	case LTyI64:
		return "int64"
	case LTyU8:
		return "uint8"
	case LTyU16:
		return "uint16"
	case LTyU32:
		return "uint32"
	case LTyU64:
		return "uint64"
	case LTyF32:
		return "float32"
	case LTyF64:
		return "float64"
	case LTyBool:
		return "bool"
	case LTyString:
		return "string"
	case LTyUnit:
		return ""
	case LTyError:
		return "error"
	case LTyPlatformInt:
		return "int"
	case LTyPlatformUint:
		return "uint"
	case LTyStruct:
		return g.visName(t.Name, t.IsExported)
	case LTyClassHandle:
		return "*" + g.visName(t.Name, t.IsExported)
	case LTyTuple:
		// Go doesn't have tuples — this case shouldn't appear in emitted code
		return "any"
	case LTySlice:
		return "[]" + g.goType(t.Elem)
	case LTyMap:
		return "map[" + g.goType(t.Key) + "]" + g.goType(t.Elem)
	case LTyChannel:
		return "chan " + g.goType(t.Elem)
	case LTyGenerator:
		return "func() (" + g.goType(t.Elem) + ", bool)"
	case LTyMutex:
		g.autoImport("sync")
		return "sync.Mutex"
	case LTyOptional:
		// Classes are already pointers in Go — *ClassName is sufficient, no **ClassName
		if t.Elem != nil && t.Elem.Kind == LTyClassHandle {
			return "*" + g.visName(t.Elem.Name, t.Elem.IsExported)
		}
		return "*" + g.goType(t.Elem)
	case LTyTaggedUnion:
		return g.visName(t.Name, t.IsExported)
	case LTyFuncPtr:
		var params []string
		for _, p := range t.Params {
			params = append(params, g.goType(p))
		}
		ret := ""
		if t.Return != nil && t.Return.Kind != LTyUnit {
			ret = " " + g.goType(t.Return)
		}
		return fmt.Sprintf("func(%s)%s", strings.Join(params, ", "), ret)
	case LTyErrorResult:
		// This is (T, error) — emitted as multi-return, not a single type
		return g.goType(t.Elem)
	case LTyAny:
		if t.Name != "" {
			return g.visName(t.Name, t.IsExported) // interface name
		}
		return "any"
	case LTyUnion:
		return "any" // ad-hoc unions use Go's any with type switches
	case LTyTypeVar:
		return t.Name // type parameter, e.g. "T"
	}
	return "any"
}

func (g *GoBackend) autoImport(path string) {
	if _, ok := g.imports[path]; !ok {
		g.imports[path] = ""
	}
}

// emitTypeParams writes Go type parameter list [T any, U cmp.Ordered] if non-empty.
func (g *GoBackend) emitTypeParams(tps []LTypeParam) {
	g.emitTypeParamsWithRelational(tps, nil, nil)
}

// emitFuncTypeParams writes Go type parameter list with relational constraint support.
func (g *GoBackend) emitFuncTypeParams(f *LFuncDecl) {
	g.emitTypeParamsWithRelational(f.TypeParams, f.RelationalConstraints, nil)
}

// emitTypeParamsWithRelational writes type params, using split interface names for
// relational constraints. e.g., where Graph<G, N, E> → G GraphG[N, E], N GraphN[E], E GraphE[N]
func (g *GoBackend) emitTypeParamsWithRelational(tps []LTypeParam, relConstraints []LRelationalConstraint, prog *LProgram) {
	if len(tps) == 0 {
		return
	}

	// Build a map from type param name → Go constraint string
	// from relational constraints (e.g., where Graph<G, N, E>)
	relConstraintMap := make(map[string]string) // type param → constraint like "GraphG[N, E]"
	for _, rc := range relConstraints {
		// Find the corresponding interface in the program to analyze method signatures
		var iface *LInterfaceDecl
		if g.prog != nil {
			for i := range g.prog.Interfaces {
				if g.prog.Interfaces[i].Name == rc.InterfaceName {
					iface = &g.prog.Interfaces[i]
					break
				}
			}
		}

		for i, argName := range rc.TypeArgs {
			isExported := g.nameExported[rc.InterfaceName]
			splitName := g.visName(rc.InterfaceName+argName, isExported)

			// Determine which other type args are actually used in this receiver's methods
			var otherArgs []string
			if iface != nil {
				usedTPs := map[string]bool{}
				for _, m := range iface.Methods {
					if m.ReceiverType != iface.TypeParams[i].Name {
						continue
					}
					g.collectTypeVarRefs(m.Params, m.ReturnType, usedTPs)
				}
				// Map interface type param names to function type param names
				// Include self-referencing type params if used in method signatures
				for j, otherTP := range rc.TypeArgs {
					if usedTPs[iface.TypeParams[j].Name] {
						otherArgs = append(otherArgs, otherTP)
					}
				}
			} else {
				// Fallback: include all other type args
				for j, otherArg := range rc.TypeArgs {
					if j != i {
						otherArgs = append(otherArgs, otherArg)
					}
				}
			}

			constraint := splitName
			if len(otherArgs) > 0 {
				constraint += "[" + strings.Join(otherArgs, ", ") + "]"
			}
			relConstraintMap[argName] = constraint
		}
	}

	g.writef("[")
	for i, tp := range tps {
		if i > 0 {
			g.writef(", ")
		}
		if relC, ok := relConstraintMap[tp.Name]; ok {
			g.writef("%s %s", tp.Name, relC)
		} else {
			g.writef("%s %s", tp.Name, g.goConstraint(tp.Constraint))
		}
	}
	g.writef("]")
}

// resolveTypeVarParam extracts a type variable from param types, unwrapping
// Optional(TypeVar) since Go collapses class-handle optionals to the base type.
func (g *GoBackend) resolveTypeVarParam(paramTypes []*LType, i int) *LType {
	if paramTypes == nil || i >= len(paramTypes) {
		return nil
	}
	pt := paramTypes[i]
	if pt.Kind == LTyTypeVar {
		return pt
	}
	// Optional(TypeVar) → TypeVar (Go collapses optional class handles)
	if pt.Kind == LTyOptional && pt.Elem != nil && pt.Elem.Kind == LTyTypeVar {
		return pt.Elem
	}
	return nil
}

// emitTypeArgs writes Go type argument list [int32, string] if non-empty.
func (g *GoBackend) emitTypeArgs(tas []*LType) {
	if len(tas) == 0 {
		return
	}
	g.writef("[")
	for i, ta := range tas {
		if i > 0 {
			g.writef(", ")
		}
		g.writef("%s", g.goType(ta))
	}
	g.writef("]")
}

// goConstraint maps a Forge constraint name to a Go constraint.
func (g *GoBackend) goConstraint(c string) string {
	switch c {
	case "":
		return "any"
	case "Comparable":
		g.autoImport("cmp")
		return "cmp.Ordered"
	case "Equatable":
		return "comparable"
	case "Hashable":
		return "comparable"
	default:
		return c // user-defined constraint (interface name)
	}
}

// ---------------------------------------------------------------------------
// Declaration emission
// ---------------------------------------------------------------------------

func (g *GoBackend) emitTypeDef(td *LTypeDef) {
	name := g.visName(td.Name, td.IsExported)
	g.writef("type %s = %s\n\n", name, g.goType(td.Type))
}

func (g *GoBackend) emitStructDecl(s *LStructDecl) {
	name := g.visName(s.Name, s.IsExported)
	g.writef("type %s", name)
	g.emitTypeParams(s.TypeParams)
	g.writef(" struct {\n")
	for _, f := range s.Fields {
		g.writef("\t%s %s\n", exportedFieldName(f.Name), g.goType(f.Type))
	}
	g.writef("}\n\n")
}

func (g *GoBackend) emitEnumDecl(e *LEnumDecl) {
	name := g.visName(e.Name, e.IsExported)
	g.enumVariants[e.Name] = e.Variants

	// Interface type
	g.writef("type %s interface {\n", name)
	g.writef("\tis%s()\n", name)
	g.writef("}\n\n")

	// Variant structs
	for _, v := range e.Variants {
		vName := g.visName(v.Name, e.IsExported)
		g.writef("type %s struct {\n", vName)
		for _, f := range v.Fields {
			g.writef("\t%s %s\n", exportedFieldName(f.Name), g.goType(f.Type))
		}
		g.writef("}\n\n")
		g.writef("func (%s) is%s() {}\n\n", vName, name)
	}
}

func (g *GoBackend) emitInterfaceDecl(iface *LInterfaceDecl) {
	// Check if this is a multi-class interface (has typed methods)
	isMultiClass := false
	for _, m := range iface.Methods {
		if m.ReceiverType != "" {
			isMultiClass = true
			break
		}
	}

	if !isMultiClass {
		// Traditional single-class interface
		g.emitSingleInterface(iface)
		return
	}

	// Multi-class interface: emit one Go interface per unique ReceiverType
	// Collect unique receiver types in order
	seen := map[string]bool{}
	var receiverTypes []string
	for _, m := range iface.Methods {
		if m.ReceiverType != "" && !seen[m.ReceiverType] {
			seen[m.ReceiverType] = true
			receiverTypes = append(receiverTypes, m.ReceiverType)
		}
	}

	// For each receiver type, emit an interface with methods for that type param.
	// Only include type params that actually appear in method signatures.
	for _, recvTP := range receiverTypes {
		subName := g.visName(iface.Name+recvTP, iface.IsExported)

		// Collect which other type params are referenced in this receiver's methods
		usedTPs := map[string]bool{}
		for _, m := range iface.Methods {
			if m.ReceiverType != recvTP {
				continue
			}
			g.collectTypeVarRefs(m.Params, m.ReturnType, usedTPs)
		}
		// Only remove self-reference if it's not actually used in method params/return types
		// (e.g. C.prev() -> C? needs C in scope even though C is the receiver type)
		selfUsedInSignatures := false
		for _, m := range iface.Methods {
			if m.ReceiverType != recvTP {
				continue
			}
			sigRefs := map[string]bool{}
			g.collectTypeVarRefs(m.Params, m.ReturnType, sigRefs)
			if sigRefs[recvTP] {
				selfUsedInSignatures = true
				break
			}
		}
		if !selfUsedInSignatures {
			delete(usedTPs, recvTP)
		}

		// Build ordered list of used type params (preserve original order)
		var otherTPs []LTypeParam
		for _, tp := range iface.TypeParams {
			if usedTPs[tp.Name] {
				otherTPs = append(otherTPs, tp)
			}
		}

		g.writef("type %s", subName)
		if len(otherTPs) > 0 {
			g.writef("[")
			for i, tp := range otherTPs {
				if i > 0 {
					g.writef(", ")
				}
				constraint := "any"
				if tp.Constraint != "" {
					constraint = g.goConstraint(tp.Constraint)
				}
				g.writef("%s %s", tp.Name, constraint)
			}
			g.writef("]")
		}
		g.writef(" interface {\n")

		for _, m := range iface.Methods {
			if m.ReceiverType != recvTP {
				continue
			}
			// Multi-class interface methods must be exported to match pub class methods
			methodName := g.visName(m.Name, true)
			g.writef("\t%s(", methodName)
			for i, p := range m.Params {
				if i > 0 {
					g.writef(", ")
				}
				g.writef("%s %s", p.Name, g.goTypeForIface(p.Type))
			}
			g.writef(")")
			if m.ReturnType != nil && m.ReturnType.Kind != LTyUnit {
				g.writef(" %s", g.goTypeForIface(m.ReturnType))
			}
			g.writef("\n")
		}
		g.writef("}\n\n")
	}

	// Also emit free functions (no ReceiverType) as standalone functions
	// These will be emitted by the function lowering, not as interface methods
}

// collectTypeVarRefs finds all type variable names referenced in method params and return type.
func (g *GoBackend) collectTypeVarRefs(params []LParam, retType *LType, out map[string]bool) {
	for _, p := range params {
		g.collectTypeVarRefsFromType(p.Type, out)
	}
	if retType != nil {
		g.collectTypeVarRefsFromType(retType, out)
	}
}

func (g *GoBackend) collectTypeVarRefsFromType(t *LType, out map[string]bool) {
	if t == nil {
		return
	}
	if t.Kind == LTyTypeVar {
		out[t.Name] = true
	}
	g.collectTypeVarRefsFromType(t.Elem, out)
	g.collectTypeVarRefsFromType(t.Key, out)
	g.collectTypeVarRefsFromType(t.Return, out)
	for _, p := range t.Params {
		g.collectTypeVarRefsFromType(p, out)
	}
}

func (g *GoBackend) emitSingleInterface(iface *LInterfaceDecl) {
	name := g.visName(iface.Name, iface.IsExported)
	g.writef("type %s interface {\n", name)
	for _, embed := range iface.Embeds {
		g.writef("\t%s\n", g.visName(embed, g.nameExported[embed]))
	}
	for _, m := range iface.Methods {
		// Method visibility: use the visibility map (methods may be pub on implementing classes)
		methodExported := g.nameExported[m.Name]
		if !methodExported {
			// If not found in map, fall back to interface export status
			methodExported = iface.IsExported
		}
		methodName := g.visName(m.Name, methodExported)
		g.writef("\t%s(", methodName)
		for i, p := range m.Params {
			if i > 0 {
				g.writef(", ")
			}
			g.writef("%s %s", p.Name, g.goType(p.Type))
		}
		g.writef(")")
		if m.ReturnType != nil && m.ReturnType.Kind != LTyUnit {
			g.writef(" %s", g.goType(m.ReturnType))
		}
		g.writef("\n")
	}
	g.writef("}\n\n")
}

func (g *GoBackend) emitClassDecl(c *LClassDecl) {
	name := g.visName(c.Name, c.IsExported)
	g.writef("type %s", name)
	g.emitTypeParams(c.TypeParams)
	g.writef(" struct {\n")
	for _, f := range c.Fields {
		g.writef("\t%s %s\n", exportedFieldName(f.Name), g.goType(f.Type))
	}
	g.writef("}\n\n")
}

func (g *GoBackend) emitFuncDecl(f *LFuncDecl) {
	g.inGenericFunc = len(f.TypeParams) > 0
	g.currentFuncReturnType = f.ReturnType
	defer func() { g.inGenericFunc = false; g.currentFuncReturnType = nil }()
	name := g.visName(f.Name, f.IsExported)

	// Build parameter list
	var params []string
	startIdx := 0
	if f.Receiver != "" {
		// Method — emit as (self *ReceiverType[T, ...])
		// Skip self param if present (wrapper methods may not have self in Params)
		if len(f.Params) > 0 && f.Params[0].Name == "self" {
			startIdx = 1
		}
		receiverType := g.visName(f.Receiver, g.nameExported[f.Receiver])
		if len(f.ReceiverTypeParams) > 0 {
			var tpNames []string
			for _, tp := range f.ReceiverTypeParams {
				tpNames = append(tpNames, tp.Name)
			}
			g.writef("func (self *%s[%s]) %s(", receiverType, strings.Join(tpNames, ", "), name)
		} else {
			g.writef("func (self *%s) %s(", receiverType, name)
		}
	} else {
		g.writef("func %s", name)
		g.emitFuncTypeParams(f)
		g.writef("(")
	}

	for i := startIdx; i < len(f.Params); i++ {
		p := f.Params[i]
		params = append(params, fmt.Sprintf("%s %s", p.Name, g.goType(p.Type)))
	}
	g.buf.WriteString(strings.Join(params, ", "))
	g.buf.WriteString(")")

	// Return type
	if f.ReturnType != nil && f.ReturnType.Kind != LTyUnit {
		if f.ReturnType.Kind == LTyErrorResult {
			g.writef(" (%s, error)", g.goType(f.ReturnType.Elem))
		} else {
			g.writef(" %s", g.goTypeForIface(f.ReturnType))
		}
	}

	g.writef(" {\n")
	g.indent++
	g.scanFieldWritebacks(f.Body)

	// Generator function: wrap body in goroutine+channel pattern
	if f.ReturnType != nil && f.ReturnType.Kind == LTyGenerator {
		elemType := g.goType(f.ReturnType.Elem)
		g.writeIndent()
		g.writef("_ch := make(chan %s)\n", elemType)
		g.writeIndent()
		g.writef("go func() {\n")
		g.indent++
		g.emitStmts(f.Body)
		g.writeIndent()
		g.writef("close(_ch)\n")
		g.indent--
		g.writeIndent()
		g.writef("}()\n")
		g.writeIndent()
		g.writef("return func() (%s, bool) {\n", elemType)
		g.indent++
		g.writeIndent()
		g.writef("v, ok := <-_ch\n")
		g.writeIndent()
		g.writef("return v, ok\n")
		g.indent--
		g.writeIndent()
		g.writef("}\n")
	} else {
		g.emitStmts(f.Body)
	}

	g.indent--
	g.writef("}\n\n")
}

// ---------------------------------------------------------------------------
// Statement emission
// ---------------------------------------------------------------------------

func (g *GoBackend) emitStmts(stmts []LStmt) {
	for _, s := range stmts {
		g.emitStmt(&s)
	}
}

func (g *GoBackend) emitStmt(s *LStmt) {
	switch s.Kind {
	case LStmtTempDef:
		td := s.Data.(*LTempDef)
		g.tempDefs[td.ID] = td
		// Suppress emission of VariantTag temps — consumed by type switch
		if td.Expr.Kind == LExprVariantTag {
			break
		}
		// Suppress temps consumed by field-writeback patterns
		if g.suppressedTemps[td.ID] {
			break
		}
		g.writeIndent()
		// Use cast for temps with specific numeric types to handle type mismatches
		needsCast := td.Expr.Type != nil && g.needsTypedDecl(td.Expr.Type) && isSimpleExpr(&td.Expr)
		if needsCast {
			g.writef("%s := %s(", g.tempName(td.ID), g.goType(td.Expr.Type))
		} else if g.exprIsNil(&td.Expr) {
			// nil can't appear in := context; use var declaration
			g.writef("var %s %s\n", g.tempName(td.ID), g.goType(td.Expr.Type))
			break
		} else {
			g.writef("%s := ", g.tempName(td.ID))
		}
		g.emitExpr(&td.Expr)
		if needsCast {
			g.writef(")")
		}
		g.writef("\n")

	case LStmtVarDecl:
		vd := s.Data.(*LVarDecl)
		g.writeIndent()
		if vd.Init != nil {
			if vd.Name == "_" {
				g.writef("_ = ")
				g.emitValue(vd.Init)
			} else if vd.Type != nil && g.needsTypedDecl(vd.Type) {
				// Use typed declaration when the Go type might differ from inference
				// (e.g. i32 from len() which returns int, or literal defaults)
				g.writef("%s := %s(", vd.Name, g.goType(vd.Type))
				g.emitValue(vd.Init)
				g.writef(")")
			} else if vd.Type != nil && (vd.Type.Kind == LTyTaggedUnion || vd.Type.Kind == LTyUnion) {
				// Interface/union type — must use typed declaration to avoid concrete type
				g.writef("var %s %s = ", vd.Name, g.goType(vd.Type))
				g.emitUnionWrappedValue(vd.Init)
			} else {
				g.writef("%s := ", vd.Name)
				g.emitValue(vd.Init)
			}
		} else {
			g.writef("var %s %s", vd.Name, g.goType(vd.Type))
		}
		g.writef("\n")

	case LStmtAssign:
		a := s.Data.(*LAssign)
		g.writeIndent()
		g.writef("%s = ", a.Target)
		g.emitValue(&a.Value)
		g.writef("\n")

	case LStmtStructSet:
		ss := s.Data.(*LStructSet)
		g.writeIndent()
		g.emitValue(&ss.Receiver)
		g.writef(".%s = ", exportedFieldName(ss.Field))
		g.emitValue(&ss.Value)
		g.writef("\n")

	case LStmtClassSet:
		cs := s.Data.(*LClassSet)
		g.writeIndent()
		g.emitValue(&cs.Handle)
		g.writef(".%s = ", exportedFieldName(cs.Field))
		g.emitValue(&cs.Value)
		g.writef("\n")

	case LStmtIndexSet:
		is := s.Data.(*LIndexSet)
		g.writeIndent()
		g.emitValue(&is.Collection)
		g.writef("[")
		g.emitValue(&is.Index)
		g.writef("] = ")
		g.emitValue(&is.Value)
		g.writef("\n")

	case LStmtIf:
		ifData := s.Data.(*LIf)
		g.writeIndent()
		g.writef("if ")
		g.emitValue(&ifData.Cond)
		g.writef(" {\n")
		g.indent++
		g.emitStmts(ifData.Then)
		g.indent--
		if len(ifData.Else) > 0 {
			// Check if else is a single if (else-if chain)
			if len(ifData.Else) == 1 && ifData.Else[0].Kind == LStmtIf {
				g.writeIndent()
				g.writef("} else ")
				// Emit the inner if without leading indent
				innerIf := ifData.Else[0].Data.(*LIf)
				g.writef("if ")
				g.emitValue(&innerIf.Cond)
				g.writef(" {\n")
				g.indent++
				g.emitStmts(innerIf.Then)
				g.indent--
				if len(innerIf.Else) > 0 {
					g.writeIndent()
					g.writef("} else {\n")
					g.indent++
					g.emitStmts(innerIf.Else)
					g.indent--
				}
				g.writeIndent()
				g.writef("}\n")
			} else {
				g.writeIndent()
				g.writef("} else {\n")
				g.indent++
				g.emitStmts(ifData.Else)
				g.indent--
				g.writeIndent()
				g.writef("}\n")
			}
		} else {
			g.writeIndent()
			g.writef("}\n")
		}

	case LStmtWhile:
		w := s.Data.(*LWhile)
		g.writeIndent()
		if len(w.CondBlock) == 0 {
			// Simple condition
			g.writef("for ")
			g.emitValue(&w.CondVar)
			g.writef(" {\n")
		} else {
			// Complex condition: emit as for { condBlock; if !cond { break } ... }
			g.writef("for {\n")
			g.indent++
			g.emitStmts(w.CondBlock)
			g.writeIndent()
			g.writef("if !")
			g.emitValue(&w.CondVar)
			g.writef(" {\n")
			g.indent++
			g.writeIndent()
			g.writef("break\n")
			g.indent--
			g.writeIndent()
			g.writef("}\n")
			g.indent--
		}
		g.indent++
		g.emitStmts(w.Body)
		g.indent--
		g.writeIndent()
		g.writef("}\n")

	case LStmtFor:
		f := s.Data.(*LFor)
		// Generator iteration: for x in gen_expr → call closure repeatedly
		if f.Collection.Type != nil && f.Collection.Type.Kind == LTyGenerator {
			g.writeIndent()
			g.writef("for {\n")
			g.indent++
			g.writeIndent()
			g.writef("%s, _ok := ", f.Var)
			g.emitValue(&f.Collection)
			g.writef("()\n")
			g.writeIndent()
			g.writef("if !_ok { break }\n")
			if f.IndexVar != "" {
				g.writeIndent()
				g.writef("%s++\n", f.IndexVar)
			}
			g.emitStmts(f.Body)
			g.indent--
			g.writeIndent()
			g.writef("}\n")
			break
		}
		g.writeIndent()
		if f.IndexVar != "" {
			g.writef("for _idx_%s, %s := range ", f.IndexVar, f.Var)
		} else {
			g.writef("for _, %s := range ", f.Var)
		}
		g.emitValue(&f.Collection)
		g.writef(" {\n")
		g.indent++
		if f.IndexVar != "" {
			// Cast index to int32 (Forge's default integer type)
			g.writeIndent()
			g.writef("%s := int32(_idx_%s)\n", f.IndexVar, f.IndexVar)
		}
		g.emitStmts(f.Body)
		g.indent--
		g.writeIndent()
		g.writef("}\n")

	case LStmtSwitch:
		sw := s.Data.(*LSwitch)
		// Enum type switch
		if sw.EnumName != "" {
			if td, ok := g.tempDefs[sw.Tag.TempID]; ok && td.Expr.Kind == LExprVariantTag {
				vtd := td.Expr.Data.(*LVariantTagData)
				g.emitEnumTypeSwitch(sw, &vtd.Value)
				break
			}
		}
		g.writeIndent()
		g.writef("switch ")
		g.emitValue(&sw.Tag)
		g.writef(" {\n")
		for _, c := range sw.Cases {
			g.writeIndent()
			if c.Tag == -1 {
				g.writef("default:\n")
			} else {
				g.writef("case %d:\n", c.Tag)
			}
			g.indent++
			g.emitStmts(c.Body)
			g.indent--
		}
		g.writeIndent()
		g.writef("}\n")

	case LStmtTypeSwitch:
		ts := s.Data.(*LTypeSwitch)
		g.writeIndent()
		g.writef("switch ")
		g.emitValue(&ts.Value)
		g.writef(".(type) {\n")
		for _, c := range ts.Cases {
			g.writeIndent()
			if c.Type == nil {
				g.writef("default:\n")
			} else {
				g.writef("case %s:\n", g.goType(c.Type))
			}
			g.indent++
			g.emitStmts(c.Body)
			g.indent--
		}
		g.writeIndent()
		g.writef("}\n")

	case LStmtBlock:
		b := s.Data.(*LBlock)
		g.writeIndent()
		g.writef("{\n")
		g.indent++
		g.emitStmts(b.Stmts)
		g.indent--
		g.writeIndent()
		g.writef("}\n")

	case LStmtReturn:
		r := s.Data.(*LReturn)
		g.writeIndent()
		if len(r.Values) == 0 {
			g.writef("return\n")
		} else {
			g.writef("return ")
			for i, v := range r.Values {
				if i > 0 {
					g.writef(", ")
				}
				g.emitValue(&v)
			}
			g.writef("\n")
		}

	case LStmtBreak:
		g.writeIndent()
		g.writef("break\n")

	case LStmtContinue:
		g.writeIndent()
		g.writef("continue\n")

	case LStmtExpr:
		es := s.Data.(*LExprStmt)
		g.writeIndent()
		g.writef("_ = %s\n", g.tempName(es.TempID))

	case LStmtDefer:
		d := s.Data.(*LDefer)
		g.writeIndent()
		g.writef("defer func() {\n")
		g.indent++
		g.emitStmts(d.Body)
		g.indent--
		g.writeIndent()
		g.writef("}()\n")

	case LStmtSpawn:
		sp := s.Data.(*LSpawn)
		g.writeIndent()
		g.writef("go func() {\n")
		g.indent++
		g.emitStmts(sp.Body)
		g.indent--
		g.writeIndent()
		g.writef("}()\n")

	case LStmtLock:
		lk := s.Data.(*LLock)
		g.writeIndent()
		g.autoImport("sync")
		g.emitValue(&lk.Mutex)
		g.writef(".Lock()\n")
		g.writeIndent()
		g.writef("defer ")
		g.emitValue(&lk.Mutex)
		g.writef(".Unlock()\n")
		g.emitStmts(lk.Body)

	case LStmtSend:
		snd := s.Data.(*LSend)
		g.writeIndent()
		g.emitValue(&snd.Channel)
		g.writef(" <- ")
		g.emitValue(&snd.Value)
		g.writef("\n")

	case LStmtYield:
		val := s.Data.(LValue)
		g.writeIndent()
		g.writef("_ch <- ")
		g.emitValue(&val)
		g.writef("\n")

	case LStmtSelect:
		sel := s.Data.(*LSelect)
		g.writeIndent()
		g.writef("select {\n")
		for _, c := range sel.Cases {
			g.writeIndent()
			switch c.Kind {
			case LSelectDefault:
				g.writef("default:\n")
			case LSelectRecv:
				if c.Binding != "" {
					g.writef("case %s := <-", c.Binding)
				} else {
					g.writef("case <-")
				}
				g.emitValue(&c.Channel)
				g.writef(":\n")
			case LSelectSend:
				g.writef("case ")
				g.emitValue(&c.Channel)
				g.writef(" <- ")
				g.emitValue(&c.Value)
				g.writef(":\n")
			}
			g.indent++
			g.emitStmts(c.Body)
			g.indent--
		}
		g.writeIndent()
		g.writef("}\n")

	case LStmtSideEffect:
		se := s.Data.(*LSideEffect)
		// Special case: append/push must reassign to the slice variable
		if se.Expr.Kind == LExprBuiltin {
			d := se.Expr.Data.(*LBuiltinData)
			if (d.Name == "append" || d.Name == "slice_push") && len(d.Args) > 0 {
				// Direct variable reference
				if d.Args[0].Kind == LValVar {
					g.writeIndent()
					g.writef("%s = ", d.Args[0].Name)
					g.emitExpr(&se.Expr)
					g.writef("\n")
					break
				}
				// Temp referencing a field access: trace back to emit field writeback
				if d.Args[0].Kind == LValTemp {
					if td, ok := g.tempDefs[d.Args[0].TempID]; ok {
						var obj *LValue
						var field string
						switch td.Expr.Kind {
						case LExprStructField:
							sf := td.Expr.Data.(*LStructFieldData)
							obj = &sf.Receiver
							field = sf.Field
						case LExprClassGet:
							cg := td.Expr.Data.(*LClassGetData)
							obj = &cg.Handle
							field = cg.Field
						}
						if obj != nil {
							g.writeIndent()
							g.emitValue(obj)
							g.writef(".%s = ", exportedFieldName(field))
							g.writef("append(")
							g.emitValue(obj)
							g.writef(".%s", exportedFieldName(field))
							for _, a := range d.Args[1:] {
								g.writef(", ")
								g.emitValue(&a)
							}
							g.writef(")\n")
							break
						}
					}
				}
			}
		}
		g.writeIndent()
		g.emitExpr(&se.Expr)
		g.writef("\n")

	case LStmtMultiAssign:
		ma := s.Data.(*LMultiAssign)
		g.writeIndent()
		for i, name := range ma.Names {
			if i > 0 {
				g.writef(", ")
			}
			g.writef("%s", name)
		}
		g.writef(" := ")
		g.emitExpr(&ma.Expr)
		g.writef("\n")
	}
}

// ---------------------------------------------------------------------------
// Enum type switch emission
// ---------------------------------------------------------------------------

// emitEnumTypeSwitch emits a Go type switch for an enum match.
// Input: LSwitch with integer tags + EnumName, and the match value.
// Output: switch _v := matchVal.(type) { case VariantName: ... }
func (g *GoBackend) emitEnumTypeSwitch(sw *LSwitch, matchVal *LValue) {
	variants, _ := g.enumVariants[sw.EnumName]
	exported := g.nameExported[sw.EnumName]

	// Check if any case body uses variant data (needs _v binding)
	needsBinding := g.switchUsesVariantData(sw)

	g.writeIndent()
	if needsBinding {
		g.writef("switch _v := ")
		g.emitValue(matchVal)
		g.writef(".(type) {\n")
	} else {
		g.writef("switch ")
		g.emitValue(matchVal)
		g.writef(".(type) {\n")
	}

	for _, c := range sw.Cases {
		g.writeIndent()
		if c.Tag == -1 {
			if c.Binding != "" {
				// default with binding: bind the value
				g.writef("default:\n")
				g.indent++
				g.writeIndent()
				g.writef("%s := ", c.Binding)
				g.emitValue(matchVal)
				g.writef("\n")
				g.emitStmtsRewritingVariantData(c.Body, sw.EnumName)
				g.indent--
			} else {
				g.writef("default:\n")
				g.indent++
				g.emitStmtsRewritingVariantData(c.Body, sw.EnumName)
				g.indent--
			}
		} else {
			// Look up variant name by tag index
			variantName := ""
			if c.Tag >= 0 && c.Tag < len(variants) {
				variantName = g.visName(variants[c.Tag].Name, exported)
			} else {
				variantName = fmt.Sprintf("/* unknown tag %d */", c.Tag)
			}
			g.writef("case %s:\n", variantName)
			g.indent++
			g.emitStmtsRewritingVariantData(c.Body, sw.EnumName)
			g.indent--
		}
	}

	g.writeIndent()
	g.writef("}\n")
}

// emitStmtsRewritingVariantData emits statements, rewriting LExprVariantData
// type assertions to use the type switch bound variable `_v` with a direct field access.
func (g *GoBackend) emitStmtsRewritingVariantData(stmts []LStmt, enumName string) {
	saved := g.typeSwitchVar
	g.typeSwitchVar = "_v"
	g.emitStmts(stmts)
	g.typeSwitchVar = saved
}

// switchUsesVariantData checks if any case body in the switch uses LExprVariantData.
func (g *GoBackend) switchUsesVariantData(sw *LSwitch) bool {
	for _, c := range sw.Cases {
		if g.stmtsUseVariantData(c.Body) {
			return true
		}
	}
	return false
}

func (g *GoBackend) stmtsUseVariantData(stmts []LStmt) bool {
	for _, s := range stmts {
		if g.stmtUsesVariantData(&s) {
			return true
		}
	}
	return false
}

func (g *GoBackend) stmtUsesVariantData(s *LStmt) bool {
	switch s.Kind {
	case LStmtTempDef:
		td := s.Data.(*LTempDef)
		return g.exprUsesVariantData(&td.Expr)
	case LStmtVarDecl:
		vd := s.Data.(*LVarDecl)
		if vd.Init != nil {
			return g.valueUsesVariantData(vd.Init)
		}
	case LStmtAssign:
		a := s.Data.(*LAssign)
		return g.valueUsesVariantData(&a.Value)
	case LStmtIf:
		d := s.Data.(*LIf)
		return g.stmtsUseVariantData(d.Then) || g.stmtsUseVariantData(d.Else)
	case LStmtReturn:
		r := s.Data.(*LReturn)
		for _, v := range r.Values {
			if g.valueUsesVariantData(&v) {
				return true
			}
		}
	}
	return false
}

func (g *GoBackend) exprUsesVariantData(e *LExpr) bool {
	if e.Kind == LExprVariantData {
		return true
	}
	return false
}

func (g *GoBackend) valueUsesVariantData(v *LValue) bool {
	return false // values are vars/temps/literals — variant data only appears in exprs via temps
}

// ---------------------------------------------------------------------------
// Expression emission
// ---------------------------------------------------------------------------

func (g *GoBackend) emitExpr(e *LExpr) {
	switch e.Kind {
	case LExprBinOp:
		d := e.Data.(*LBinOpData)
		g.emitValue(&d.Left)
		g.writef(" %s ", goBinOp(d.Op))
		g.emitValue(&d.Right)

	case LExprUnOp:
		d := e.Data.(*LUnOpData)
		g.writef("%s", goUnOp(d.Op))
		g.emitValue(&d.Operand)

	case LExprCast:
		d := e.Data.(*LCastData)
		g.writef("%s(", g.goType(d.Target))
		g.emitValue(&d.Operand)
		g.writef(")")

	case LExprStructField:
		d := e.Data.(*LStructFieldData)
		g.emitValue(&d.Receiver)
		g.writef(".%s", exportedFieldName(d.Field))

	case LExprClassGet:
		d := e.Data.(*LClassGetData)
		g.emitValue(&d.Handle)
		g.writef(".%s", exportedFieldName(d.Field))

	case LExprIndexGet:
		d := e.Data.(*LIndexGetData)
		g.emitValue(&d.Collection)
		g.writef("[")
		g.emitValue(&d.Index)
		g.writef("]")

	case LExprSlice:
		d := e.Data.(*LSliceData)
		g.emitValue(&d.Collection)
		g.writef("[")
		if d.Low != nil {
			g.emitValue(d.Low)
		}
		g.writef(":")
		if d.High != nil {
			g.emitValue(d.High)
		}
		g.writef("]")

	case LExprCall:
		d := e.Data.(*LCallData)
		// Package-qualified calls (e.g. fmt.Println) — capitalize function part only
		if idx := strings.IndexByte(d.Func, '.'); idx >= 0 {
			pkg := d.Func[:idx]
			fn := d.Func[idx+1:]
			g.writef("%s.%s", pkg, g.visName(fn, true))
		} else {
			g.writef("%s", g.visName(d.Func, d.IsExported))
		}
		g.emitTypeArgs(d.TypeArgs)
		g.writef("(")
		for i, arg := range d.Args {
			if i > 0 {
				g.writef(", ")
			}
			g.emitValue(&arg)
		}
		g.writef(")")

	case LExprMethodCall:
		d := e.Data.(*LMethodCallData)
		g.emitValue(&d.Receiver)
		isExported := d.IsExported
		// Methods on type variables come from interface constraints — always exported in Go
		isTypeVarReceiver := d.Receiver.Type != nil && d.Receiver.Type.Kind == LTyTypeVar
		if isTypeVarReceiver {
			isExported = true
		}
		g.writef(".%s", g.visName(d.Method, isExported))
		g.emitTypeArgs(d.TypeArgs)
		g.writef("(")
		for i, arg := range d.Args {
			if i > 0 {
				g.writef(", ")
			}
			// In generic functions, nil args to type-var receiver methods need
			// zero-value emission since Go rejects bare nil for type parameters.
			if g.inGenericFunc && arg.Kind == LValLitNull && isTypeVarReceiver {
				// Use ParamTypes from the method signature if available
				if pt := g.resolveTypeVarParam(d.ParamTypes, i); pt != nil {
					g.writef("*new(%s)", pt.Name)
				} else if arg.Type != nil && arg.Type.Kind == LTyTypeVar {
					g.writef("*new(%s)", arg.Type.Name)
				} else {
					g.writef("*new(%s)", d.Receiver.Type.Name)
				}
			} else {
				g.emitValue(&arg)
			}
		}
		g.writef(")")

	case LExprBuiltin:
		d := e.Data.(*LBuiltinData)
		g.emitBuiltin(d)

	case LExprStructLit:
		d := e.Data.(*LStructLitData)
		typeName := g.goType(e.Type)
		if e.Type != nil && e.Type.Kind == LTySlice {
			// List literal — empty slice with unknown elem type → nil
			if len(d.Fields) == 0 && e.Type.Elem != nil && e.Type.Elem.Kind == LTyAny {
				g.writef("nil")
			} else {
				g.writef("%s{", typeName)
				for i, f := range d.Fields {
					if i > 0 {
						g.writef(", ")
					}
					g.emitValue(&f.Value)
				}
				g.writef("}")
			}
		} else if e.Type != nil && e.Type.Kind == LTyClassHandle {
			// Class struct literal — emit with & (pointer)
			name := g.visName(e.Type.Name, e.Type.IsExported)
			g.writef("&%s{", name)
			for i, f := range d.Fields {
				if i > 0 {
					g.writef(", ")
				}
				g.writef("%s: ", exportedFieldName(f.Name))
				g.emitValue(&f.Value)
			}
			g.writef("}")
		} else {
			g.writef("%s{", typeName)
			for i, f := range d.Fields {
				if i > 0 {
					g.writef(", ")
				}
				g.writef("%s: ", exportedFieldName(f.Name))
				g.emitValue(&f.Value)
			}
			g.writef("}")
		}

	case LExprClassAlloc:
		d := e.Data.(*LClassAllocData)
		className := d.Class
		if e.Type != nil {
			className = g.visName(d.Class, e.Type.IsExported)
		}
		// Build type arg suffix for generic classes
		typeArgSuffix := ""
		if len(d.TypeArgs) > 0 {
			var parts []string
			for _, ta := range d.TypeArgs {
				parts = append(parts, g.goType(ta))
			}
			typeArgSuffix = "[" + strings.Join(parts, ", ") + "]"
		}
		if len(d.Fields) == 0 {
			g.writef("&%s%s{}", className, typeArgSuffix)
		} else {
			g.writef("&%s%s{", className, typeArgSuffix)
			for i, f := range d.Fields {
				if i > 0 {
					g.writef(", ")
				}
				if f.Name != "" {
					g.writef("%s: ", exportedFieldName(f.Name))
				}
				g.emitValue(&f.Value)
			}
			g.writef("}")
		}

	case LExprMakeSlice:
		elemType := g.goType(e.Type.Elem)
		g.writef("make([]%s, 0)", elemType)

	case LExprMakeMap:
		g.writef("make(%s)", g.goType(e.Type))

	case LExprMakeChannel:
		d := e.Data.(*LMakeChannelData)
		if d.BufSize != nil {
			g.writef("make(chan %s, ", g.goType(d.ElemType))
			g.emitValue(d.BufSize)
			g.writef(")")
		} else {
			g.writef("make(chan %s)", g.goType(d.ElemType))
		}

	case LExprWrapOptional:
		d := e.Data.(*LWrapOptionalData)
		// For Optional(TypeVar), the return type was collapsed to just TypeVar,
		// so wrapping is a no-op.
		if e.Type != nil && e.Type.Kind == LTyOptional && e.Type.Elem != nil && e.Type.Elem.Kind == LTyTypeVar {
			g.emitValue(&d.Value)
		} else {
			// Create pointer: func() *T { v := val; return &v }()
			g.writef("func() %s { _v := ", g.goType(e.Type))
			g.emitValue(&d.Value)
			g.writef("; return &_v }()")
		}

	case LExprUnwrapOptional:
		d := e.Data.(*LUnwrapOptionalData)
		// Determine whether to dereference.
		skipDeref := false
		srcType := d.Value.Type
		if srcType != nil && srcType.Kind == LTyOptional && srcType.Elem != nil {
			if srcType.Elem.Kind == LTyClassHandle {
				// Direct Optional(ClassHandle): *className in Go, no deref
				skipDeref = true
			} else if srcType.Elem.Kind == LTyTypeVar {
				// Optional(TypeVar) was collapsed to TypeVar in function signatures,
				// so the Go value is already the unwrapped type — no deref
				skipDeref = true
			}
		}
		// Fallback: if source type is not Optional, nothing to unwrap
		if srcType != nil && srcType.Kind != LTyOptional {
			skipDeref = true
		}
		if skipDeref {
			g.emitValue(&d.Value)
		} else {
			g.writef("*")
			g.emitValue(&d.Value)
		}

	case LExprIsNull:
		d := e.Data.(*LIsNullData)
		// Use reflect-based _isNil for correct nil checking in all contexts,
		// including Go generics where any(nil_pointer) != nil.
		g.autoImport("reflect")
		g.needsIsNilHelper = true
		g.writef("_isNil(")
		g.emitValue(&d.Value)
		g.writef(")")

	case LExprVariantConstruct:
		d := e.Data.(*LVariantConstructData)
		exported := e.Type != nil && e.Type.IsExported
		vName := g.visName(d.Variant, exported)
		if len(d.Fields) == 0 {
			g.writef("%s{}", vName)
		} else {
			g.writef("%s{", vName)
			for i, f := range d.Fields {
				if i > 0 {
					g.writef(", ")
				}
				g.emitValue(&f)
			}
			g.writef("}")
		}

	case LExprVariantTag:
		// Emit a type switch expression — this needs to be handled at the statement level
		g.writef("/* variant tag */")

	case LExprVariantData:
		d := e.Data.(*LVariantDataData)
		if g.typeSwitchVar != "" {
			// Inside a type switch — _v is already the concrete variant type
			g.writef("%s.%s", g.typeSwitchVar, exportedFieldName(d.Field))
		} else {
			exported := g.nameExported[d.Enum]
			g.emitValue(&d.Value)
			g.writef(".(%s).%s", g.visName(d.Variant, exported), exportedFieldName(d.Field))
		}

	case LExprExtractValue:
		// First element of multi-return — handled by caller context
		d := e.Data.(*LExtractValueData)
		g.emitValue(&d.Value)

	case LExprExtractError:
		d := e.Data.(*LExtractErrorData)
		g.emitValue(&d.Value)

	case LExprMakeResult:
		d := e.Data.(*LMakeResultData)
		g.emitValue(&d.Value)
		g.writef(", ")
		g.emitValue(&d.Err)

	case LExprFuncRef:
		d := e.Data.(*LFuncRefData)
		g.writef("%s", d.Name)

	case LExprFuncLit:
		d := e.Data.(*LFuncLitData)
		g.writef("func(")
		for i, p := range d.Params {
			if i > 0 {
				g.writef(", ")
			}
			g.writef("%s %s", p.Name, g.goType(p.Type))
		}
		g.writef(")")
		if d.ReturnType != nil {
			g.writef(" %s", g.goType(d.ReturnType))
		}
		g.writef(" {\n")
		g.indent++
		g.emitStmts(d.Body)
		g.indent--
		g.writeIndent()
		g.writef("}")

	case LExprFormat:
		d := e.Data.(*LFormatData)
		g.autoImport("fmt")
		var fmtStr strings.Builder
		var args []LValue
		for _, p := range d.Parts {
			if p.IsLiteral {
				fmtStr.WriteString(p.Text)
			} else {
				fmtStr.WriteString(p.Format)
				args = append(args, p.Value)
			}
		}
		g.writef("fmt.Sprintf(%q", fmtStr.String())
		for _, arg := range args {
			g.writef(", ")
			g.emitValue(&arg)
		}
		g.writef(")")
	}
}

func (g *GoBackend) emitBuiltin(d *LBuiltinData) {
	switch d.Name {
	case "println":
		g.autoImport("fmt")
		g.writef("fmt.Println(")
		g.emitArgs(d.Args)
		g.writef(")")
	case "print":
		g.autoImport("fmt")
		g.writef("fmt.Print(")
		g.emitArgs(d.Args)
		g.writef(")")
	case "len":
		g.writef("len(")
		g.emitArgs(d.Args)
		g.writef(")")
	case "append":
		g.writef("append(")
		g.emitArgs(d.Args)
		g.writef(")")
	case "isnull":
		if len(d.Args) > 0 {
			g.autoImport("reflect")
			g.needsIsNilHelper = true
			g.writef("_isNil(")
			g.emitValue(&d.Args[0])
			g.writef(")")
		}
	case "channel_receive":
		if len(d.Args) > 0 {
			g.writef("<-")
			g.emitValue(&d.Args[0])
		}
	case "channel_close":
		if len(d.Args) > 0 {
			g.writef("close(")
			g.emitValue(&d.Args[0])
			g.writef(")")
		}
	case "hash_string":
		g.autoImport("hash/fnv")
		g.needsHashStringHelper = true
		if len(d.Args) > 0 {
			g.writef("_hashString(")
			g.emitValue(&d.Args[0])
			g.writef(")")
		}
	default:
		// Built-in methods: string_X, slice_X, map_X
		if strings.HasPrefix(d.Name, "string_") || strings.HasPrefix(d.Name, "slice_") || strings.HasPrefix(d.Name, "map_") {
			g.emitBuiltinMethod(d)
		} else {
			g.writef("%s(", d.Name)
			g.emitArgs(d.Args)
			g.writef(")")
		}
	}
}

func (g *GoBackend) emitBuiltinMethod(d *LBuiltinData) {
	parts := strings.SplitN(d.Name, "_", 2)
	if len(parts) != 2 || len(d.Args) == 0 {
		g.writef("/* unknown builtin %s */", d.Name)
		return
	}
	prefix := parts[0]
	method := parts[1]
	recv := d.Args[0]
	args := d.Args[1:]

	switch method {
	case "contains":
		if prefix == "string" {
			g.autoImport("strings")
			g.writef("strings.Contains(")
			g.emitValue(&recv)
			g.writef(", ")
			g.emitArgs(args)
			g.writef(")")
		} else {
			// slice.contains — linear search
			g.autoImport("slices")
			g.writef("slices.Contains(")
			g.emitValue(&recv)
			g.writef(", ")
			g.emitArgs(args)
			g.writef(")")
		}
	case "split":
		g.autoImport("strings")
		g.writef("strings.Split(")
		g.emitValue(&recv)
		g.writef(", ")
		g.emitArgs(args)
		g.writef(")")
	case "trim":
		g.autoImport("strings")
		g.writef("strings.TrimSpace(")
		g.emitValue(&recv)
		g.writef(")")
	case "to_upper":
		g.autoImport("strings")
		g.writef("strings.ToUpper(")
		g.emitValue(&recv)
		g.writef(")")
	case "to_lower":
		g.autoImport("strings")
		g.writef("strings.ToLower(")
		g.emitValue(&recv)
		g.writef(")")
	case "push":
		// slice.push(x) → append(slice, x)
		g.writef("append(")
		g.emitValue(&recv)
		g.writef(", ")
		g.emitArgs(args)
		g.writef(")")
	case "get":
		// map.get(key)
		g.emitValue(&recv)
		g.writef("[")
		g.emitArgs(args)
		g.writef("]")
	case "has_key":
		if len(args) > 0 {
			g.writef("func() bool { _, ok := ")
			g.emitValue(&recv)
			g.writef("[")
			g.emitValue(&args[0])
			g.writef("]; return ok }()")
		}
	case "len":
		g.writef("len(")
		g.emitValue(&recv)
		g.writef(")")
	case "has_prefix":
		g.autoImport("strings")
		g.writef("strings.HasPrefix(")
		g.emitValue(&recv)
		g.writef(", ")
		g.emitArgs(args)
		g.writef(")")
	case "has_suffix":
		g.autoImport("strings")
		g.writef("strings.HasSuffix(")
		g.emitValue(&recv)
		g.writef(", ")
		g.emitArgs(args)
		g.writef(")")
	case "index_of":
		g.autoImport("strings")
		g.writef("strings.Index(")
		g.emitValue(&recv)
		g.writef(", ")
		g.emitArgs(args)
		g.writef(")")
	case "join":
		g.autoImport("strings")
		g.writef("strings.Join(")
		g.emitValue(&recv)
		g.writef(", ")
		g.emitArgs(args)
		g.writef(")")
	case "replace":
		g.autoImport("strings")
		g.writef("strings.ReplaceAll(")
		g.emitValue(&recv)
		g.writef(", ")
		g.emitArgs(args)
		g.writef(")")
	case "repeat":
		g.autoImport("strings")
		g.writef("strings.Repeat(")
		g.emitValue(&recv)
		g.writef(", ")
		if len(args) > 0 {
			g.writef("int(")
			g.emitValue(&args[0])
			g.writef(")")
		}
		g.writef(")")
	case "pop":
		// slice.pop() → func() T { last := s[len(s)-1]; s = s[:len(s)-1]; return last }()
		// Emitted as a compound expression
		g.writef("func() any { _s := ")
		g.emitValue(&recv)
		g.writef("; _last := _s[len(_s)-1]; return _last }()")
	case "contains_key":
		if len(args) > 0 {
			g.writef("func() bool { _, ok := ")
			g.emitValue(&recv)
			g.writef("[")
			g.emitValue(&args[0])
			g.writef("]; return ok }()")
		}
	case "keys":
		// Derive key type from receiver map type
		keyType := "any"
		if recv.Type != nil && recv.Type.Key != nil {
			keyType = g.goType(recv.Type.Key)
		}
		g.writef("func() []%s { _ks := make([]%s, 0, len(", keyType, keyType)
		g.emitValue(&recv)
		g.writef(")); for k := range ")
		g.emitValue(&recv)
		g.writef(" { _ks = append(_ks, k) }; return _ks }()")
	case "values":
		valType := "any"
		if recv.Type != nil && recv.Type.Elem != nil {
			valType = g.goType(recv.Type.Elem)
		}
		g.writef("func() []%s { _vs := make([]%s, 0, len(", valType, valType)
		g.emitValue(&recv)
		g.writef(")); for _, v := range ")
		g.emitValue(&recv)
		g.writef(" { _vs = append(_vs, v) }; return _vs }()")
	default:
		g.emitValue(&recv)
		g.writef(".%s(", method)
		g.emitArgs(args)
		g.writef(")")
	}
}

func (g *GoBackend) emitArgs(args []LValue) {
	for i, a := range args {
		if i > 0 {
			g.writef(", ")
		}
		g.emitValue(&a)
	}
}

// ---------------------------------------------------------------------------
// Value emission
// ---------------------------------------------------------------------------

func (g *GoBackend) emitValue(v *LValue) {
	switch v.Kind {
	case LValVar:
		g.writef("%s", v.Name)
	case LValTemp:
		g.writef("%s", g.tempName(v.TempID))
	case LValGlobal:
		g.writef("%s", v.Name)
	case LValLitInt:
		g.writef("%d", v.IntVal)
	case LValLitUint:
		g.writef("%d", v.UintVal)
	case LValLitFloat:
		g.writef("%g", v.FloatVal)
	case LValLitString:
		g.writef("%q", v.StrVal)
	case LValLitBool:
		if v.BoolVal {
			g.writef("true")
		} else {
			g.writef("false")
		}
	case LValLitNull:
		// In generic functions, bare nil is invalid for type parameters.
		// Emit *new(T) to get the zero value of the type param.
		if g.inGenericFunc && v.Type != nil && v.Type.Kind == LTyTypeVar {
			g.writef("*new(%s)", v.Type.Name)
		} else if g.inGenericFunc && v.Type != nil && v.Type.Kind == LTyOptional && v.Type.Elem != nil && v.Type.Elem.Kind == LTyTypeVar {
			// Optional(TypeVar) collapsed to TypeVar — zero value
			g.writef("*new(%s)", v.Type.Elem.Name)
		} else if g.inGenericFunc && (v.Type == nil || v.Type.Kind == LTyAny) && g.currentFuncReturnType != nil &&
			g.currentFuncReturnType.Kind == LTyOptional && g.currentFuncReturnType.Elem != nil && g.currentFuncReturnType.Elem.Kind == LTyTypeVar {
			// Null with unresolved type in function returning collapsed Optional(TypeVar)
			g.writef("*new(%s)", g.currentFuncReturnType.Elem.Name)
		} else {
			g.writef("nil")
		}
	}
}


// emitUnionWrappedValue emits a value with an explicit type cast when the value
// is a literal being stored into an any-typed union variable. Without the cast,
// Go infers default types (int, float64) which don't match type switch cases
// (int32, float32, etc.).
func (g *GoBackend) emitUnionWrappedValue(v *LValue) {
	if v.Type != nil && (v.Kind == LValLitInt || v.Kind == LValLitUint || v.Kind == LValLitFloat || v.Kind == LValLitBool) {
		goT := g.goType(v.Type)
		// Only wrap if the Go type differs from Go's default inference
		needsWrap := false
		switch v.Kind {
		case LValLitInt:
			needsWrap = goT != "int"
		case LValLitUint:
			needsWrap = goT != "uint"
		case LValLitFloat:
			needsWrap = goT != "float64"
		case LValLitBool:
			needsWrap = false // bool is always bool
		}
		if needsWrap {
			g.writef("%s(", goT)
			g.emitValue(v)
			g.writef(")")
			return
		}
	}
	g.emitValue(v)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (g *GoBackend) tempName(id int) string {
	return fmt.Sprintf("_t%d", id)
}

func (g *GoBackend) visName(name string, exported bool) string {
	if exported {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return strings.ToLower(name[:1]) + name[1:]
}

func exportedFieldName(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// needsTypedDecl returns true when a literal-initialized variable declaration
// should use `var x Type = val` instead of `x := val` to avoid Go type inference
// defaulting to a different type (e.g. int literal → int instead of int32).
func (g *GoBackend) needsTypedDecl(t *LType) bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case LTyI8, LTyI16, LTyI32, LTyI64, LTyU8, LTyU16, LTyU32, LTyU64:
		return true // Go would infer int/uint
	case LTyF32:
		return true // Go would infer float64
	}
	return false
}

// needsNumericCast returns true when a variable declaration's init value has a
// different numeric type than the declared type (e.g. len() returns int but
// the variable is declared as i32).
func (g *GoBackend) needsNumericCast(declType *LType, init *LValue) bool {
	if declType == nil || init == nil || init.Type == nil {
		return false
	}
	if !isNumericType(declType) || !isNumericType(init.Type) {
		return false
	}
	return declType.Kind != init.Type.Kind || declType.Bits != init.Type.Bits
}

func isNumericType(t *LType) bool {
	switch t.Kind {
	case LTyI8, LTyI16, LTyI32, LTyI64, LTyU8, LTyU16, LTyU32, LTyU64, LTyF32, LTyF64, LTyPlatformInt, LTyPlatformUint:
		return true
	}
	return false
}

// exprIsNil returns true if the expression would emit bare "nil" (e.g. empty slice with unknown elem).
func (g *GoBackend) exprIsNil(e *LExpr) bool {
	if e.Kind == LExprStructLit && e.Type != nil && e.Type.Kind == LTySlice {
		d := e.Data.(*LStructLitData)
		if len(d.Fields) == 0 && e.Type.Elem != nil && e.Type.Elem.Kind == LTyAny {
			return true
		}
	}
	return false
}

// isSimpleExpr returns true for expressions where Go would infer a numeric type
// that might differ from the declared type (unary ops on literals, plain values).
func isSimpleExpr(e *LExpr) bool {
	switch e.Kind {
	case LExprUnOp:
		return true
	case LExprBinOp:
		// Binary ops between literals/values
		return true
	}
	return false
}

func goBinOp(op LBinOpKind) string {
	switch op {
	case LBinAdd:
		return "+"
	case LBinSub:
		return "-"
	case LBinMul:
		return "*"
	case LBinDiv:
		return "/"
	case LBinMod:
		return "%"
	case LBinEq:
		return "=="
	case LBinNe:
		return "!="
	case LBinLt:
		return "<"
	case LBinLe:
		return "<="
	case LBinGt:
		return ">"
	case LBinGe:
		return ">="
	case LBinAnd:
		return "&&"
	case LBinOr:
		return "||"
	case LBinBitAnd:
		return "&"
	case LBinBitOr:
		return "|"
	case LBinBitXor:
		return "^"
	case LBinShl:
		return "<<"
	case LBinShr:
		return ">>"
	}
	return "?"
}

func goUnOp(op LUnOpKind) string {
	switch op {
	case LUnNeg:
		return "-"
	case LUnNot:
		return "!"
	case LUnBitNot:
		return "^"
	}
	return "?"
}

func (g *GoBackend) writef(format string, args ...interface{}) {
	if len(args) == 0 {
		g.buf.WriteString(format)
	} else {
		g.buf.WriteString(fmt.Sprintf(format, args...))
	}
}

func (g *GoBackend) writeIndent() {
	for i := 0; i < g.indent; i++ {
		g.buf.WriteByte('\t')
	}
}
