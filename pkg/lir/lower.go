package lir

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/waywardgeek/forge/pkg/ast"
	"github.com/waywardgeek/forge/pkg/checker"
)

// dataAs extracts a value from an any-typed Data field, handling both pointer and value storage.
func dataAs[T any](data any) *T {
	switch v := data.(type) {
	case *T:
		return v
	case T:
		return &v
	}
	panic(fmt.Sprintf("dataAs: unexpected type %T", data))
}

// Lowerer converts a type-checked AST into LIR.
type Lowerer struct {
	nextTemp int
	stmts    []LStmt // current statement accumulator

	// Checker state for type resolution
	registry *checker.Registry

	// Current function context
	currentReturnType *LType

	// Enum variant info (populated during registration)
	variantCtors map[string]*variantCtorInfo // variant name → ctor info
	unitVariants map[string]string           // variant name → enum name
	enumVariants map[string][]LVariant       // enum name → variants

	// Class info
	classCtorFields map[string][]string // class name → ctor field names
	classFields     map[string][]LField // class name → all fields

	// Visibility tracking
	exported map[string]bool // name → is exported (pub) — types, functions, methods

	// Import tracking — alias → path (e.g. "fmt" → "fmt", "errors" → "errors")
	importAliases map[string]string

	// Type parameters in scope for the current generic function/class
	typeParamsInScope map[string]bool

	// Class type params (for method receiver emission)
	classTypeParams map[string][]LTypeParam // class name → type params

	// Variable types for assignment coercion
	varTypes map[string]*LType

	// Type alias resolution (name → underlying LType)
	typeAliasTypes map[string]*LType

	// Interface declarations for impl block resolution
	ifaceDecls map[string]*LInterfaceDecl // interface name → lowered decl
}

type variantCtorInfo struct {
	enumName   string
	fieldNames []string
	tag        int
}

// NewLowerer creates a new AST→LIR lowering pass.
func NewLowerer() *Lowerer {
	return &Lowerer{
		variantCtors:    make(map[string]*variantCtorInfo),
		unitVariants:    make(map[string]string),
		enumVariants:    make(map[string][]LVariant),
		classCtorFields: make(map[string][]string),
		classFields:     make(map[string][]LField),
		importAliases:   make(map[string]string),
		exported:        make(map[string]bool),
		classTypeParams: make(map[string][]LTypeParam),
		varTypes:        make(map[string]*LType),
		ifaceDecls:      make(map[string]*LInterfaceDecl),
		typeAliasTypes:  make(map[string]*LType),
	}
}

// Lower converts an entire AST file to an LIR program.
func (l *Lowerer) Lower(file *ast.File) *LProgram {
	prog := &LProgram{}

	for _, block := range file.Blocks {
		// First pass: register types
		l.registerTypes(&block)

		// Set package name from first block
		if prog.Package == "" {
			prog.Package = block.Name
		}

		// Lower imports
		for _, imp := range block.Imports {
			prog.Imports = append(prog.Imports, LImport{
				Alias: imp.Alias,
				Path:  imp.Path,
			})
			// Track import alias → path for package-qualified call detection
			alias := imp.Alias
			if alias == "" {
				// Use last segment of path as alias
				parts := strings.Split(imp.Path, "/")
				alias = parts[len(parts)-1]
			}
			l.importAliases[alias] = imp.Path
		}

		// Lower type aliases
		for _, ta := range block.TypeAliases {
			loweredType := l.lowerTypeExpr(&ta.Type)
			l.typeAliasTypes[ta.Name] = loweredType
			prog.TypeDefs = append(prog.TypeDefs, LTypeDef{
				Name:       ta.Name,
				Type:       loweredType,
				IsExported: ta.IsPublic,
			})
		}

		// Lower structs
		for _, s := range block.Structs {
			prog.Structs = append(prog.Structs, l.lowerStructDecl(&s))
		}

		// Lower enums
		for _, e := range block.Enums {
			prog.Enums = append(prog.Enums, l.lowerEnumDecl(&e))
		}

		// Lower classes
		// Pre-register class names so lowerTypeExpr can identify class types
	// even when a class references another class declared later in the same block.
	for _, cls := range block.Classes {
		if _, exists := l.classFields[cls.Name]; !exists {
			l.classFields[cls.Name] = nil // placeholder — populated below
		}
	}

	for _, cls := range block.Classes {
			prog.Classes = append(prog.Classes, l.lowerClassDecl(&cls))
		}

		// Lower interfaces
		for _, iface := range block.Interfaces {
			lowered := l.lowerInterfaceDecl(&iface)
			prog.Interfaces = append(prog.Interfaces, lowered)
			l.ifaceDecls[iface.Name] = &prog.Interfaces[len(prog.Interfaces)-1]
		}

		// Lower functions (including class methods)
		for _, fn := range block.Functions {
			prog.Functions = append(prog.Functions, l.lowerFuncDecl(&fn, ""))
		}
		for _, cls := range block.Classes {
			// Set class type params in scope for method lowering
			if len(cls.TypeParams) > 0 {
				l.typeParamsInScope = make(map[string]bool)
				for _, tp := range cls.TypeParams {
					l.typeParamsInScope[tp.Name] = true
				}
			}
			for _, m := range cls.Methods {
				prog.Functions = append(prog.Functions, l.lowerFuncDecl(&m, cls.Name))
			}
			l.typeParamsInScope = nil
		}

		// Process impl blocks — generate wrapper methods for aliased methods
		for _, implBlock := range block.ImplBlocks {
			wrappers := l.lowerImplBlock(&implBlock)
			prog.Functions = append(prog.Functions, wrappers...)
		}
	}

	return prog
}

// registerTypes populates variant/class lookup tables before lowering.
func (l *Lowerer) registerTypes(block *ast.ForgeBlock) {
	// Register visibility for all named types
	for _, e := range block.Enums {
		l.exported[e.Name] = e.IsPublic
		for _, v := range e.Variants {
			l.exported[v.Name] = e.IsPublic
		}
	}
	for _, s := range block.Structs {
		l.exported[s.Name] = s.IsPublic
	}
	for _, c := range block.Classes {
		l.exported[c.Name] = c.IsPublic
	}
	for _, ta := range block.TypeAliases {
		l.exported[ta.Name] = ta.IsPublic
	}
	for _, iface := range block.Interfaces {
		l.exported[iface.Name] = iface.IsPublic
	}

	// Pre-register impl block method names as exported.
	// Impl-generated wrappers are always exported but are lowered after user code.
	// User code (e.g. destructor bodies) may call these methods, so they need
	// to be in the exported map before lowering begins.
	for _, impl := range block.ImplBlocks {
		for _, m := range impl.Mappings {
			l.exported[m.MethodName] = true
		}
	}

	for _, e := range block.Enums {
		var variants []LVariant
		for i, v := range e.Variants {
			var fields []LField
			for _, f := range v.Fields {
				fields = append(fields, LField{
					Name: f.Name,
					Type: l.lowerTypeExpr(&f.Type),
				})
			}
			lv := LVariant{Name: v.Name, Tag: i, Fields: fields}
			variants = append(variants, lv)

			if len(v.Fields) > 0 {
				var fieldNames []string
				for _, f := range v.Fields {
					name := f.Name
					if name == "" {
						name = fmt.Sprintf("V%d", len(fieldNames))
					}
					fieldNames = append(fieldNames, name)
				}
				l.variantCtors[v.Name] = &variantCtorInfo{
					enumName:   e.Name,
					fieldNames: fieldNames,
					tag:        i,
				}
			} else {
				l.unitVariants[v.Name] = e.Name
			}
		}
		l.enumVariants[e.Name] = variants
	}

	// Pre-register all class names so cross-references resolve to LTyClassHandle
	for _, cls := range block.Classes {
		l.classFields[cls.Name] = nil
	}

	for _, cls := range block.Classes {
		// Register class type params
		if len(cls.TypeParams) > 0 {
			var tps []LTypeParam
			for _, tp := range cls.TypeParams {
				tps = append(tps, LTypeParam{Name: tp.Name, Constraint: tp.Constraint})
			}
			l.classTypeParams[cls.Name] = tps
		}
		// Set type params in scope for field type resolution
		if len(cls.TypeParams) > 0 {
			l.typeParamsInScope = make(map[string]bool)
			for _, tp := range cls.TypeParams {
				l.typeParamsInScope[tp.Name] = true
			}
		}
		var fieldNames []string
		var fields []LField
		for _, p := range cls.CtorParams {
			fieldNames = append(fieldNames, p.Name)
			fields = append(fields, LField{Name: p.Name, Type: l.lowerTypeExpr(&p.Type)})
		}
		for _, f := range cls.Fields {
			fields = append(fields, LField{Name: f.Name, Type: l.lowerTypeExpr(&f.Type)})
		}
		l.classCtorFields[cls.Name] = fieldNames
		l.classFields[cls.Name] = fields
		l.typeParamsInScope = nil
	}

	// Register function and method visibility
	for _, fn := range block.Functions {
		l.exported[fn.Name] = fn.IsPublic
	}
	for _, cls := range block.Classes {
		for _, m := range cls.Methods {
			l.exported[m.Name] = m.IsPublic
		}
	}
}

// ---------------------------------------------------------------------------
// Type lowering
// ---------------------------------------------------------------------------

func (l *Lowerer) lowerTypeExpr(te *ast.TypeExpr) *LType {
	if te == nil {
		return &LType{Kind: LTyUnit}
	}
	switch te.Kind {
	case ast.TypeNamed:
		nt := dataAs[ast.NamedType](te.Data)
		return l.lowerNamedType(nt)
	case ast.TypeOptional:
		ot := dataAs[ast.OptionalType](te.Data)
		return &LType{Kind: LTyOptional, Elem: l.lowerTypeExpr(&ot.Inner)}
	case ast.TypeSequence:
		st := dataAs[ast.SequenceType](te.Data)
		return &LType{Kind: LTySlice, Elem: l.lowerTypeExpr(&st.Elem)}
	case ast.TypeMap:
		mt := dataAs[ast.MapType](te.Data)
		return &LType{Kind: LTyMap, Key: l.lowerTypeExpr(&mt.Key), Elem: l.lowerTypeExpr(&mt.Value)}
	case ast.TypeTuple:
		tt := dataAs[ast.TupleType](te.Data)
		var fields []LField
		for i, f := range tt.Fields {
			name := f.Name
			if name == "" {
				name = fmt.Sprintf("_%d", i)
			}
			fields = append(fields, LField{Name: name, Type: l.lowerTypeExpr(&f.Type)})
		}
		// Special case: (T, error) → ErrorResult
		if len(fields) == 2 && fields[1].Type.Kind == LTyError {
			return &LType{Kind: LTyErrorResult, Elem: fields[0].Type}
		}
		return &LType{Kind: LTyTuple, Fields: fields}
	case ast.TypeFunc:
		ft := dataAs[ast.FuncType](te.Data)
		var params []*LType
		for _, p := range ft.Params {
			params = append(params, l.lowerTypeExpr(&p))
		}
		return &LType{Kind: LTyFuncPtr, Params: params, Return: l.lowerTypeExpr(&ft.Return)}
	case ast.TypeChannel:
		ct := dataAs[ast.ChannelType](te.Data)
		return &LType{Kind: LTyChannel, Elem: l.lowerTypeExpr(&ct.Elem)}
	case ast.TypeGenerator:
		gt := dataAs[ast.GeneratorType](te.Data)
		return &LType{Kind: LTyGenerator, Elem: l.lowerTypeExpr(&gt.Elem)}
	case ast.TypeLock:
		return &LType{Kind: LTyMutex}
	case ast.TypeUnit:
		return &LType{Kind: LTyUnit}
	case ast.TypeUnion:
		// Preserve union member types for C backend
		ut := dataAs[ast.UnionType](te.Data)
		var members []LField
		for i, m := range ut.Variants {
			members = append(members, LField{
				Name: fmt.Sprintf("m%d", i),
				Type: l.lowerTypeExpr(&m),
			})
		}
		return &LType{Kind: LTyUnion, Fields: members}
	}
	return &LType{Kind: LTyUnit}
}

func (l *Lowerer) lowerNamedType(nt *ast.NamedType) *LType {
	switch nt.Name {
	case "i8":
		return &LType{Kind: LTyI8, Bits: 8}
	case "i16":
		return &LType{Kind: LTyI16, Bits: 16}
	case "i32":
		return &LType{Kind: LTyI32, Bits: 32}
	case "i64":
		return &LType{Kind: LTyI64, Bits: 64}
	case "u8":
		return &LType{Kind: LTyU8, Bits: 8}
	case "u16":
		return &LType{Kind: LTyU16, Bits: 16}
	case "u32":
		return &LType{Kind: LTyU32, Bits: 32}
	case "u64":
		return &LType{Kind: LTyU64, Bits: 64}
	case "f32":
		return &LType{Kind: LTyF32, Bits: 32}
	case "f64":
		return &LType{Kind: LTyF64, Bits: 64}
	case "bool":
		return &LType{Kind: LTyBool}
	case "string":
		return &LType{Kind: LTyString}
	case "int":
		return &LType{Kind: LTyPlatformInt, Bits: -1}
	case "uint":
		return &LType{Kind: LTyPlatformUint, Bits: -1}
	case "error":
		return &LType{Kind: LTyError}
	case "any":
		return &LType{Kind: LTyAny}
	}
	// User-defined type — check if it's an enum
	if _, ok := l.enumVariants[nt.Name]; ok {
		return &LType{Kind: LTyTaggedUnion, Name: nt.Name, IsExported: l.exported[nt.Name]}
	}
	// Check if it's a class
	if _, ok := l.classFields[nt.Name]; ok {
		return &LType{Kind: LTyClassHandle, Name: nt.Name, IsExported: l.exported[nt.Name]}
	}
	// Check if it's a type parameter in scope
	if l.typeParamsInScope[nt.Name] {
		return &LType{Kind: LTyTypeVar, Name: nt.Name}
	}
	// Check if it's a type alias — return underlying type with name preserved
	if aliasType, ok := l.typeAliasTypes[nt.Name]; ok {
		// Return a copy with the alias name set for typedef emission
		copied := *aliasType
		copied.Name = nt.Name
		copied.IsExported = l.exported[nt.Name]
		return &copied
	}
	// Check if it's an interface
	if _, ok := l.ifaceDecls[nt.Name]; ok {
		return &LType{Kind: LTyAny, Name: nt.Name, IsExported: l.exported[nt.Name]}
	}
	// Default to struct
	return &LType{Kind: LTyStruct, Name: nt.Name, IsExported: l.exported[nt.Name]}
}

// lowerCheckerType converts a checker.Type to an LType.
func (l *Lowerer) lowerCheckerType(ct *checker.Type) *LType {
	if ct == nil {
		return &LType{Kind: LTyUnit}
	}
	switch ct.Kind {
	case checker.TyInt:
		if ct.Bits == -1 {
			return &LType{Kind: LTyPlatformInt, Bits: -1}
		}
		return lirIntType(ct.Bits)
	case checker.TyUint:
		if ct.Bits == -1 {
			return &LType{Kind: LTyPlatformUint, Bits: -1}
		}
		return lirUintType(ct.Bits)
	case checker.TyFloat:
		if ct.Bits == 32 {
			return &LType{Kind: LTyF32, Bits: 32}
		}
		return &LType{Kind: LTyF64, Bits: 64}
	case checker.TyBool:
		return &LType{Kind: LTyBool}
	case checker.TyString:
		return &LType{Kind: LTyString}
	case checker.TyUnit:
		return &LType{Kind: LTyUnit}
	case checker.TyList:
		return &LType{Kind: LTySlice, Elem: l.lowerCheckerType(ct.Elem)}
	case checker.TyMap:
		return &LType{Kind: LTyMap, Key: l.lowerCheckerType(ct.Key), Elem: l.lowerCheckerType(ct.Val)}
	case checker.TyOptional:
		return &LType{Kind: LTyOptional, Elem: l.lowerCheckerType(ct.Elem)}
	case checker.TyChannel:
		return &LType{Kind: LTyChannel, Elem: l.lowerCheckerType(ct.Elem)}
	case checker.TyGenerator:
		return &LType{Kind: LTyGenerator, Elem: l.lowerCheckerType(ct.Elem)}
	case checker.TyStruct:
		return &LType{Kind: LTyStruct, Name: ct.Name, IsExported: l.exported[ct.Name]}
	case checker.TyClass:
		return &LType{Kind: LTyClassHandle, Name: ct.Name, IsExported: l.exported[ct.Name]}
	case checker.TyEnum:
		return &LType{Kind: LTyTaggedUnion, Name: ct.Name, IsExported: l.exported[ct.Name]}
	case checker.TyInterface:
		return &LType{Kind: LTyAny, Name: ct.Name, IsExported: l.exported[ct.Name]}
	case checker.TyFunc:
		var params []*LType
		for _, p := range ct.Params {
			params = append(params, l.lowerCheckerType(p))
		}
		return &LType{Kind: LTyFuncPtr, Params: params, Return: l.lowerCheckerType(ct.Return)}
	case checker.TyTuple:
		var fields []LField
		for i, f := range ct.Fields {
			name := f.Name
			if name == "" {
				name = fmt.Sprintf("_%d", i)
			}
			fields = append(fields, LField{Name: name, Type: l.lowerCheckerType(f.Type)})
		}
		if len(fields) == 2 && fields[1].Type.Kind == LTyError {
			return &LType{Kind: LTyErrorResult, Elem: fields[0].Type}
		}
		return &LType{Kind: LTyTuple, Fields: fields}
	case checker.TyError:
		return &LType{Kind: LTyError}
	case checker.TyVar:
		return &LType{Kind: LTyTypeVar, Name: ct.Name}
	case checker.TyUnion:
		// Preserve member types for C backend tagged union support
		var members []LField
		for i, v := range ct.Variants {
			members = append(members, LField{
				Name: fmt.Sprintf("m%d", i),
				Type: l.lowerCheckerType(v),
			})
		}
		return &LType{Kind: LTyUnion, Fields: members}
	case checker.TyUnknown:
		return &LType{Kind: LTyAny}
	}
	return &LType{Kind: LTyUnit}
}

func lirIntType(bits int) *LType {
	switch bits {
	case 8:
		return &LType{Kind: LTyI8, Bits: 8}
	case 16:
		return &LType{Kind: LTyI16, Bits: 16}
	case 32:
		return &LType{Kind: LTyI32, Bits: 32}
	case 64:
		return &LType{Kind: LTyI64, Bits: 64}
	}
	return &LType{Kind: LTyI32, Bits: 32}
}

func lirUintType(bits int) *LType {
	switch bits {
	case 8:
		return &LType{Kind: LTyU8, Bits: 8}
	case 16:
		return &LType{Kind: LTyU16, Bits: 16}
	case 32:
		return &LType{Kind: LTyU32, Bits: 32}
	case 64:
		return &LType{Kind: LTyU64, Bits: 64}
	}
	return &LType{Kind: LTyU32, Bits: 32}
}

// ---------------------------------------------------------------------------
// Declaration lowering
// ---------------------------------------------------------------------------

func (l *Lowerer) lowerStructDecl(s *ast.StructDecl) LStructDecl {
	// Set up type parameters scope
	var typeParams []LTypeParam
	savedTypeParams := l.typeParamsInScope
	if len(s.TypeParams) > 0 {
		l.typeParamsInScope = make(map[string]bool)
		for k, v := range savedTypeParams {
			l.typeParamsInScope[k] = v
		}
		for _, tp := range s.TypeParams {
			typeParams = append(typeParams, LTypeParam{Name: tp.Name, Constraint: tp.Constraint})
			l.typeParamsInScope[tp.Name] = true
		}
	}

	var fields []LField
	for _, f := range s.Fields {
		fields = append(fields, LField{Name: f.Name, Type: l.lowerTypeExpr(&f.Type)})
	}
	l.typeParamsInScope = savedTypeParams
	return LStructDecl{Name: s.Name, Fields: fields, TypeParams: typeParams, IsExported: s.IsPublic}
}

func (l *Lowerer) lowerEnumDecl(e *ast.EnumDecl) LEnumDecl {
	variants := l.enumVariants[e.Name]
	return LEnumDecl{Name: e.Name, Variants: variants, IsExported: e.IsPublic}
}

func (l *Lowerer) lowerInterfaceDecl(iface *ast.InterfaceDecl) LInterfaceDecl {
	// Set up type parameters scope for resolving method types
	savedTypeParams := l.typeParamsInScope
	var typeParams []LTypeParam
	if len(iface.TypeParams) > 0 {
		l.typeParamsInScope = make(map[string]bool)
		for k, v := range savedTypeParams {
			l.typeParamsInScope[k] = v
		}
		for _, tp := range iface.TypeParams {
			typeParams = append(typeParams, LTypeParam{Name: tp.Name, Constraint: tp.Constraint})
			l.typeParamsInScope[tp.Name] = true
		}
	}

	var methods []LInterfaceMethod
	for _, m := range iface.Methods {
		var params []LParam
		// Skip self param (first param)
		for i, p := range m.Params {
			if i == 0 && p.Name == "self" {
				continue
			}
			params = append(params, LParam{Name: p.Name, Type: l.lowerTypeExpr(&p.Type)})
		}
		var retType *LType
		if m.ReturnType != nil {
			retType = l.lowerTypeExpr(m.ReturnType)
		}
		methods = append(methods, LInterfaceMethod{
			Name:         m.Name,
			ReceiverType: m.ReceiverType,
			Params:       params,
			ReturnType:   retType,
		})
	}
	l.typeParamsInScope = savedTypeParams
	return LInterfaceDecl{
		Name:       iface.Name,
		TypeParams: typeParams,
		Methods:    methods,
		Embeds:     iface.Implements,
		IsExported: iface.IsPublic,
	}
}

// lowerImplBlock generates forwarding wrapper methods for impl block aliases.
// For `G.nodes = SimpleGraph.get_nodes`, it emits:
//
// lowerImplBlock generates forwarding wrapper methods for impl block aliases.
// For `G.nodes = SimpleGraph.get_nodes`, it emits a method on SimpleGraph
// that forwards to get_nodes, making it satisfy the split Go interface.
func (l *Lowerer) lowerImplBlock(impl *ast.ImplBlock) []LFuncDecl {
	iface := l.ifaceDecls[impl.InterfaceName]
	if iface == nil {
		return nil
	}

	// Build type param → concrete type mapping from impl block type args
	typeArgMap := make(map[string]string)
	for i, tp := range iface.TypeParams {
		if i < len(impl.TypeArgs) {
			if nt, ok := impl.TypeArgs[i].Data.(ast.NamedType); ok {
				typeArgMap[tp.Name] = nt.Name
			}
		}
	}

	var wrappers []LFuncDecl

	for _, mapping := range impl.Mappings {
		if mapping.Kind == ast.ImplAlias {
			// Find the interface method this alias maps to
			var ifaceMethod *LInterfaceMethod
			for i := range iface.Methods {
				if iface.Methods[i].ReceiverType == mapping.TypeParam && iface.Methods[i].Name == mapping.MethodName {
					ifaceMethod = &iface.Methods[i]
					break
				}
			}
			if ifaceMethod == nil {
				continue
			}

			className := typeArgMap[mapping.TypeParam]
			if className == "" {
				continue
			}

			// Substitute type vars with concrete types
			params := l.substImplParams(ifaceMethod.Params, typeArgMap)
			retType := l.substImplType(ifaceMethod.ReturnType, typeArgMap)

			// Build forwarding body using proper LIR temps
			var body []LStmt

			// Build LValue args from params
			var callArgs []LValue
			for _, p := range params {
				callArgs = append(callArgs, LValue{Kind: LValVar, Name: p.Name, Type: p.Type})
			}

			// _t0 = self.targetMethod(args...)
			callExpr := LExpr{
				Kind: LExprMethodCall,
				Data: &LMethodCallData{
					Receiver:   LValue{Kind: LValVar, Name: "self", Type: &LType{Kind: LTyClassHandle, Name: className}},
					Method:     mapping.TargetMember,
					Args:       callArgs,
					IsExported: l.exported[mapping.TargetMember],
				},
				Type: retType,
			}

			if retType != nil && retType.Kind != LTyUnit {
				body = append(body, LStmt{Kind: LStmtTempDef, Data: &LTempDef{ID: 0, Expr: callExpr}})
				body = append(body, LStmt{Kind: LStmtReturn, Data: &LReturn{Values: []LValue{{Kind: LValTemp, TempID: 0, Type: retType}}}})
			} else {
				body = append(body, LStmt{Kind: LStmtTempDef, Data: &LTempDef{ID: 0, Expr: callExpr}})
				body = append(body, LStmt{Kind: LStmtExpr, Data: &LExprStmt{TempID: 0}})
			}

			wrappers = append(wrappers, LFuncDecl{
				Name:       ifaceMethod.Name,
				Receiver:   className,
				Params:     params,
				ReturnType: retType,
				Body:       body,
				IsExported: true,
			})
		}
		if mapping.Kind == ast.ImplFieldBind {
			// Field binding: P.children <-> Folder.items
			// For getters: func (self *Folder) Children() []*File { return self.Items }
			// For setters: func (self *Folder) Set_children(val []*File) { self.Items = val }
			var ifaceMethod *LInterfaceMethod
			for i := range iface.Methods {
				if iface.Methods[i].ReceiverType == mapping.TypeParam && iface.Methods[i].Name == mapping.MethodName {
					ifaceMethod = &iface.Methods[i]
					break
				}
			}
			if ifaceMethod == nil {
				continue
			}

			className := typeArgMap[mapping.TypeParam]
			if className == "" {
				continue
			}

			isSetter := len(ifaceMethod.Params) > 0
			if isSetter {
				// Setter body: self.field = val
				valType := l.substImplType(ifaceMethod.Params[0].Type, typeArgMap)
				var body []LStmt
				body = append(body, LStmt{Kind: LStmtClassSet, Data: &LClassSet{
					Handle: LValue{Kind: LValVar, Name: "self", Type: &LType{Kind: LTyClassHandle, Name: className}},
					Class:  className,
					Field:  mapping.TargetMember,
					Value:  LValue{Kind: LValVar, Name: "val", Type: valType},
				}})

				wrappers = append(wrappers, LFuncDecl{
					Name:       ifaceMethod.Name,
					Receiver:   className,
					Params:     []LParam{{Name: "val", Type: valType}},
					ReturnType: nil,
					Body:       body,
					IsExported: true,
				})
			} else {
				// Getter body: return self.field
				retType := l.substImplType(ifaceMethod.ReturnType, typeArgMap)
				var body []LStmt
				fieldAccess := LExpr{
					Kind: LExprClassGet,
					Data: &LClassGetData{
						Handle: LValue{Kind: LValVar, Name: "self", Type: &LType{Kind: LTyClassHandle, Name: className}},
						Class:  className,
						Field:  mapping.TargetMember,
					},
					Type: retType,
				}
				body = append(body, LStmt{Kind: LStmtTempDef, Data: &LTempDef{ID: 0, Expr: fieldAccess}})
				body = append(body, LStmt{Kind: LStmtReturn, Data: &LReturn{Values: []LValue{{Kind: LValTemp, TempID: 0, Type: retType}}}})

				wrappers = append(wrappers, LFuncDecl{
					Name:       ifaceMethod.Name,
					Receiver:   className,
					Params:     nil, // getter has no params
					ReturnType: retType,
					Body:       body,
					IsExported: true,
				})
			}
		}
		// TODO: ImplInline ({body})
	}

	return wrappers
}

// substImplParams substitutes type variables in method params with concrete types.
func (l *Lowerer) substImplParams(params []LParam, typeArgMap map[string]string) []LParam {
	result := make([]LParam, len(params))
	for i, p := range params {
		result[i] = LParam{Name: p.Name, Type: l.substImplType(p.Type, typeArgMap)}
	}
	return result
}

// substImplType replaces type variables with their concrete types from the impl block.
func (l *Lowerer) substImplType(t *LType, typeArgMap map[string]string) *LType {
	if t == nil {
		return nil
	}
	switch t.Kind {
	case LTyTypeVar:
		if concrete, ok := typeArgMap[t.Name]; ok {
			if _, isClass := l.classFields[concrete]; isClass {
				return &LType{Kind: LTyClassHandle, Name: concrete}
			}
			return &LType{Kind: LTyStruct, Name: concrete}
		}
		return t
	case LTySlice:
		return &LType{Kind: LTySlice, Elem: l.substImplType(t.Elem, typeArgMap)}
	case LTyOptional:
		inner := l.substImplType(t.Elem, typeArgMap)
		// Classes are already pointers — optional class = class handle (no double pointer)
		if inner.Kind == LTyClassHandle {
			return inner
		}
		return &LType{Kind: LTyOptional, Elem: inner}
	case LTyMap:
		cp := *t
		cp.Key = l.substImplType(t.Key, typeArgMap)
		cp.Elem = l.substImplType(t.Elem, typeArgMap)
		return &cp
	default:
		return t
	}
}

func (l *Lowerer) lowerClassDecl(cls *ast.ClassDecl) LClassDecl {
	// Set up type parameters scope
	var typeParams []LTypeParam
	savedTypeParams := l.typeParamsInScope
	if len(cls.TypeParams) > 0 {
		l.typeParamsInScope = make(map[string]bool)
		for k, v := range savedTypeParams {
			l.typeParamsInScope[k] = v
		}
		for _, tp := range cls.TypeParams {
			typeParams = append(typeParams, LTypeParam{Name: tp.Name, Constraint: tp.Constraint})
			l.typeParamsInScope[tp.Name] = true
		}
	}

	fields := l.classFields[cls.Name]
	guardedBy := make(map[string]string)
	for _, f := range cls.Fields {
		if f.GuardedBy != "" {
			guardedBy[f.Name] = f.GuardedBy
		}
	}
	result := LClassDecl{
		Name:       cls.Name,
		Fields:     fields,
		TypeParams: typeParams,
		GuardedBy:  guardedBy,
		IsExported: cls.IsPublic,
		Implements: cls.Implements,
	}
	l.typeParamsInScope = savedTypeParams
	return result
}

func (l *Lowerer) lowerFuncDecl(fn *ast.FuncDecl, receiver string) LFuncDecl {
	// Set up type parameters scope
	var typeParams []LTypeParam
	var relConstraints []LRelationalConstraint
	savedTypeParams := l.typeParamsInScope
	if len(fn.TypeParams) > 0 {
		l.typeParamsInScope = make(map[string]bool)
		for k, v := range savedTypeParams {
			l.typeParamsInScope[k] = v
		}
		for _, tp := range fn.TypeParams {
			typeParams = append(typeParams, LTypeParam{Name: tp.Name, Constraint: tp.Constraint})
			l.typeParamsInScope[tp.Name] = true
		}
		// Merge where clause constraints
		for _, wc := range fn.Where {
			if wc.Variable != "" {
				// Single-type constraint: where T: Integer
				for i := range typeParams {
					if typeParams[i].Name == wc.Variable && typeParams[i].Constraint == "" {
						typeParams[i].Constraint = wc.Constraint
					}
				}
			} else if len(wc.TypeArgs) > 0 {
				// Relational constraint: where Graph<G, N, E>
				var argNames []string
				for _, ta := range wc.TypeArgs {
					if ta.Kind == ast.TypeNamed {
						nt := ta.Data.(ast.NamedType)
						argNames = append(argNames, nt.Name)
					}
				}
				relConstraints = append(relConstraints, LRelationalConstraint{
					InterfaceName: wc.Constraint,
					TypeArgs:      argNames,
				})
			}
		}
	}

	var params []LParam
	for _, p := range fn.Params {
		if p.IsSelf {
			continue
		}
		params = append(params, LParam{
			Name: p.Name,
			Type: l.lowerTypeExpr(&p.Type),
		})
	}

	var retType *LType
	if fn.ReturnType != nil {
		retType = l.lowerTypeExpr(fn.ReturnType)
	} else {
		retType = &LType{Kind: LTyUnit}
	}

	// If this is a method, prepend self parameter
	if receiver != "" {
		selfType := l.lowerNamedType(&ast.NamedType{Name: receiver})
		selfParam := LParam{Name: "self", Type: selfType}
		params = append([]LParam{selfParam}, params...)
	}

	l.currentReturnType = retType
	var body []LStmt
	if fn.Body != nil {
		body = l.lowerBlock(fn.Body)
	}
	l.currentReturnType = nil
	l.typeParamsInScope = savedTypeParams

	return LFuncDecl{
		Name:                  fn.Name,
		TypeParams:            typeParams,
		Params:                params,
		ReturnType:            retType,
		Body:                  body,
		IsExported:            fn.IsPublic,
		Receiver:              receiver,
		ReceiverTypeParams:    l.classTypeParams[receiver],
		RelationalConstraints: relConstraints,
	}
}

// ---------------------------------------------------------------------------
// Statement lowering
// ---------------------------------------------------------------------------

func (l *Lowerer) lowerBlock(block *ast.Block) []LStmt {
	if block == nil {
		return nil
	}
	saved := l.stmts
	l.stmts = nil
	for _, stmt := range block.Stmts {
		l.lowerStmt(&stmt)
	}
	result := l.stmts
	l.stmts = saved
	return result
}

func (l *Lowerer) emit(stmt LStmt) {
	l.stmts = append(l.stmts, stmt)
}

func (l *Lowerer) lowerStmt(stmt *ast.Stmt) {
	switch stmt.Kind {
	case ast.StmtVarDecl:
		l.lowerVarDeclStmt(stmt)
	case ast.StmtAssign:
		l.lowerAssignStmt(stmt)
	case ast.StmtReturn:
		l.lowerReturnStmt(stmt)
	case ast.StmtExpr:
		l.lowerExprStmt(stmt)
	case ast.StmtIf:
		l.lowerIfStmt(stmt)
	case ast.StmtFor:
		l.lowerForStmt(stmt)
	case ast.StmtWhile:
		l.lowerWhileStmt(stmt)
	case ast.StmtMatch:
		l.lowerMatchStmt(stmt)
	case ast.StmtBlock:
		l.lowerBlockStmt(stmt)
	case ast.StmtBreak:
		l.emit(LStmt{Kind: LStmtBreak})
	case ast.StmtContinue:
		l.emit(LStmt{Kind: LStmtContinue})
	case ast.StmtCascade:
		cs := dataAs[ast.CascadeStmt](stmt.Data)
		l.emit(LStmt{Kind: LStmtDefer, Data: &LDefer{Body: l.lowerBlock(&cs.Body)}})
	case ast.StmtSpawn:
		ss := dataAs[ast.SpawnStmt](stmt.Data)
		l.emit(LStmt{Kind: LStmtSpawn, Data: &LSpawn{Body: l.lowerBlock(&ss.Body)}})
	case ast.StmtSelect:
		l.lowerSelectStmt(stmt)
	case ast.StmtLock:
		ls := dataAs[ast.LockStmt](stmt.Data)
		mutexVal := l.lowerExpr(&ls.Mutex)
		l.emit(LStmt{Kind: LStmtLock, Data: &LLock{
			Mutex: mutexVal,
			Body:  l.lowerBlock(&ls.Body),
		}})
	case ast.StmtYield:
		ys := dataAs[ast.YieldStmt](stmt.Data)
		val := l.lowerExpr(ys.Value)
		l.emit(LStmt{Kind: LStmtYield, Data: val})
	}
}

func (l *Lowerer) lowerVarDeclStmt(stmt *ast.Stmt) {
	vd := dataAs[ast.VarDeclStmt](stmt.Data)

	// Tuple destructuring: let (a, b) = expr
	if len(vd.Names) > 0 && vd.Value != nil {
		val := l.lowerExpr(vd.Value)
		// If the value is a temp referencing an ErrorResult or Tuple, extract fields
		for i, name := range vd.Names {
			// We emit the tuple value and destructure via extract
			var extractType *LType
			if val.Type != nil && val.Type.Kind == LTyTuple && i < len(val.Type.Fields) {
				extractType = val.Type.Fields[i].Type
			} else if val.Type != nil && val.Type.Kind == LTyErrorResult {
				if i == 0 {
					extractType = val.Type.Elem
				} else {
					extractType = &LType{Kind: LTyError}
				}
			} else {
				extractType = &LType{Kind: LTyAny}
			}
			l.emit(LStmt{Kind: LStmtVarDecl, Data: &LVarDecl{
				Name:    name,
				Type:    extractType,
				Init:    &val, // simplified: backend handles multi-return
				Mutable: vd.IsMut,
			}})
		}
		return
	}

	var varType *LType
	if vd.Type != nil {
		varType = l.lowerTypeExpr(vd.Type)
	}

	var init *LValue
	if vd.Value != nil {
		v := l.lowerExpr(vd.Value)
		init = &v
		if varType == nil {
			varType = v.Type
		}
	}

	if varType == nil {
		varType = &LType{Kind: LTyAny}
	}

	l.varTypes[vd.Name] = varType
	l.emit(LStmt{Kind: LStmtVarDecl, Data: &LVarDecl{
		Name:    vd.Name,
		Type:    varType,
		Init:    init,
		Mutable: vd.IsMut,
	}})
}

func (l *Lowerer) lowerAssignStmt(stmt *ast.Stmt) {
	as := dataAs[ast.AssignStmt](stmt.Data)
	val := l.lowerExpr(&as.Value)

	// Handle field assignment: target.field = value
	if as.Target.Kind == ast.ExprFieldAccess {
		fa := dataAs[ast.FieldAccessExpr](as.Target.Data)
		recv := l.lowerExpr(&fa.Receiver)
		// Check if receiver is a class (use ClassSet) or struct (use StructSet)
		if recv.Type != nil && recv.Type.Kind == LTyClassHandle {
			l.emit(LStmt{Kind: LStmtClassSet, Data: &LClassSet{
				Handle: recv,
				Class:  recv.Type.Name,
				Field:  fa.Field,
				Value:  val,
			}})
			return
		}
		l.emit(LStmt{Kind: LStmtStructSet, Data: &LStructSet{
			Receiver: recv,
			Field:    fa.Field,
			Value:    val,
		}})
		return
	}

	// Handle index assignment: collection[index] = value
	if as.Target.Kind == ast.ExprIndex {
		idx := dataAs[ast.IndexExpr](as.Target.Data)
		coll := l.lowerExpr(&idx.Receiver)
		index := l.lowerExpr(&idx.Index)
		l.emit(LStmt{Kind: LStmtIndexSet, Data: &LIndexSet{
			Collection: coll,
			Index:      index,
			Value:      val,
		}})
		return
	}

	// Simple variable assignment
	if as.Target.Kind == ast.ExprIdent {
		ident := dataAs[ast.IdentExpr](as.Target.Data)
		// Auto-cast numeric value when target type differs
		if targetType, ok := l.varTypes[ident.Name]; ok && targetType != nil && val.Type != nil &&
			isNumericKind(targetType.Kind) && isNumericKind(val.Type.Kind) &&
			targetType.Kind != val.Type.Kind {
			val = l.emitTemp(LExpr{
				Kind: LExprCast,
				Type: targetType,
				Data: &LCastData{Operand: val, Target: targetType},
			})
		}
		l.emit(LStmt{Kind: LStmtAssign, Data: &LAssign{
			Target: ident.Name,
			Value:  val,
		}})
		return
	}

	// Fallback: emit as variable assignment with stringified target
	l.emit(LStmt{Kind: LStmtAssign, Data: &LAssign{
		Target: "???",
		Value:  val,
	}})
}

func (l *Lowerer) lowerReturnStmt(stmt *ast.Stmt) {
	rs := dataAs[ast.ReturnStmt](stmt.Data)
	var values []LValue
	if rs.Value != nil {
		// Handle tuple literal returns as multiple values
		if rs.Value.Kind == ast.ExprTupleLit {
			tl := dataAs[ast.TupleLitExpr](rs.Value.Data)
			for _, elem := range tl.Elems {
				values = append(values, l.lowerExpr(&elem))
			}
		} else {
			val := l.lowerExpr(rs.Value)
			// Auto-wrap non-optional value when function returns optional
			if l.currentReturnType != nil && l.currentReturnType.Kind == LTyOptional {
				if val.Type == nil || val.Type.Kind != LTyOptional {
					if val.Kind != LValLitNull {
						val = l.emitTemp(LExpr{
							Kind: LExprWrapOptional,
							Type: l.currentReturnType,
							Data: &LWrapOptionalData{Value: val},
						})
					}
				}
			}
			// Auto-cast numeric return value when type mismatches
			if l.currentReturnType != nil && val.Type != nil &&
				isNumericKind(l.currentReturnType.Kind) && isNumericKind(val.Type.Kind) &&
				l.currentReturnType.Kind != val.Type.Kind {
				val = l.emitTemp(LExpr{
					Kind: LExprCast,
					Type: l.currentReturnType,
					Data: &LCastData{Operand: val, Target: l.currentReturnType},
				})
			}
			values = append(values, val)
		}
	}
	l.emit(LStmt{Kind: LStmtReturn, Data: &LReturn{Values: values}})
}

func (l *Lowerer) lowerExprStmt(stmt *ast.Stmt) {
	es := dataAs[ast.ExprStmt](stmt.Data)
	val := l.lowerExpr(&es.Expr)
	// Emit as a TempDef that is immediately discarded
	if val.Kind == LValTemp {
		l.emit(LStmt{Kind: LStmtExpr, Data: &LExprStmt{TempID: val.TempID}})
	}
}

func (l *Lowerer) lowerIfStmt(stmt *ast.Stmt) {
	is := dataAs[ast.IfStmt](stmt.Data)
	cond := l.lowerExpr(&is.Condition)
	then := l.lowerBlock(&is.Then)

	var elseStmts []LStmt
	if len(is.ElseIfs) > 0 {
		// Chain else-ifs: each becomes a nested if in the else branch
		elseStmts = l.lowerElseIfs(is.ElseIfs, is.Else)
	} else if is.Else != nil {
		elseStmts = l.lowerBlock(is.Else)
	}

	l.emit(LStmt{Kind: LStmtIf, Data: &LIf{
		Cond: cond,
		Then: then,
		Else: elseStmts,
	}})
}

func (l *Lowerer) lowerElseIfs(elseIfs []ast.ElseIf, finalElse *ast.Block) []LStmt {
	if len(elseIfs) == 0 {
		if finalElse != nil {
			return l.lowerBlock(finalElse)
		}
		return nil
	}
	ei := &elseIfs[0]
	cond := l.lowerExprInto(&ei.Condition)

	then := l.lowerBlock(&ei.Body)
	elseStmts := l.lowerElseIfs(elseIfs[1:], finalElse)

	return []LStmt{{Kind: LStmtIf, Data: &LIf{
		Cond: cond,
		Then: then,
		Else: elseStmts,
	}}}
}

func (l *Lowerer) lowerForStmt(stmt *ast.Stmt) {
	fs := dataAs[ast.ForStmt](stmt.Data)
	coll := l.lowerExpr(&fs.Collection)

	// Generator: desugar to while loop calling iterator
	if coll.Type != nil && coll.Type.Kind == LTyGenerator {
		l.lowerForGenerator(fs, coll)
		return
	}

	// Infer element type from collection
	var varType *LType
	if coll.Type != nil {
		if coll.Type.Kind == LTySlice {
			varType = coll.Type.Elem
		} else if coll.Type.Kind == LTyMap {
			varType = coll.Type.Key
		} else if coll.Type.Kind == LTyString {
			varType = &LType{Kind: LTyU8, Bits: 8}
		}
	}
	if varType == nil {
		varType = &LType{Kind: LTyAny}
	}

	l.emit(LStmt{Kind: LStmtFor, Data: &LFor{
		Var:        fs.Var,
		VarType:    varType,
		IndexVar:   fs.IndexVar,
		Collection: coll,
		Body:       l.lowerBlock(&fs.Body),
	}})
}

// lowerForGenerator desugars `for x in gen_expr` into:
//
//	_iter := gen_expr
//	for {
//	    _val, _ok := _iter()
//	    if !_ok { break }
//	    x := _val
//	    // body
//	}
//
// In LIR terms, we emit this as a LStmtWhile with the iterator call in the cond block.
func (l *Lowerer) lowerForGenerator(fs *ast.ForStmt, iterVal LValue) {
	elemType := iterVal.Type.Elem

	// The generator value IS the iterator closure already.
	// Emit: for x in iterVal — the Go backend will handle gen types specially.
	// For now, we lower it as a regular for-in and let the backend handle gen types.
	l.emit(LStmt{Kind: LStmtFor, Data: &LFor{
		Var:        fs.Var,
		VarType:    elemType,
		IndexVar:   fs.IndexVar,
		Collection: iterVal,
		Body:       l.lowerBlock(&fs.Body),
	}})
}

func (l *Lowerer) lowerWhileStmt(stmt *ast.Stmt) {
	ws := dataAs[ast.WhileStmt](stmt.Data)

	// Build the condition block: flatten the condition expression,
	// then the last temp is the condition value.
	saved := l.stmts
	l.stmts = nil
	condVal := l.lowerExpr(&ws.Condition)
	condBlock := l.stmts
	l.stmts = saved

	l.emit(LStmt{Kind: LStmtWhile, Data: &LWhile{
		CondBlock: condBlock,
		CondVar:   condVal,
		Body:      l.lowerBlock(&ws.Body),
	}})
}

func (l *Lowerer) lowerMatchStmt(stmt *ast.Stmt) {
	ms := dataAs[ast.MatchStmt](stmt.Data)
	l.emitMatch(ms, nil)
}

func (l *Lowerer) lowerMatchExpr(expr *ast.Expr) LValue {
	ms := dataAs[ast.MatchStmt](expr.Data)
	resultType := l.exprType(expr)

	// Declare result variable
	resultName := fmt.Sprintf("_matchResult%d", l.nextTemp)
	l.nextTemp++
	l.emit(LStmt{Kind: LStmtVarDecl, Data: &LVarDecl{
		Name: resultName,
		Type: resultType,
	}})

	l.emitMatch(ms, &matchResultInfo{name: resultName, typ: resultType})

	return LValue{Kind: LValVar, Name: resultName, Type: resultType}
}

type matchResultInfo struct {
	name string
	typ  *LType
}

func (l *Lowerer) lowerLambda(expr *ast.Expr) LValue {
	lam := dataAs[ast.LambdaExpr](expr.Data)

	var params []LParam
	for _, p := range lam.Params {
		params = append(params, LParam{Name: p.Name, Type: l.lowerTypeExpr(&p.Type)})
	}

	var retType *LType
	if lam.ReturnType != nil {
		retType = l.lowerTypeExpr(lam.ReturnType)
	}

	// Lower the body in an isolated scope
	var body []LStmt
	if lam.Body != nil {
		saved := l.stmts
		l.stmts = nil
		for i, s := range lam.Body.Stmts {
			// If there's a return type and the last statement is an expression,
			// emit it as a return
			if retType != nil && i == len(lam.Body.Stmts)-1 && s.Kind == ast.StmtExpr {
				es := dataAs[ast.ExprStmt](s.Data)
				val := l.lowerExpr(&es.Expr)
				l.emit(LStmt{Kind: LStmtReturn, Data: &LReturn{Values: []LValue{val}}})
			} else {
				l.lowerStmt(&s)
			}
		}
		body = l.stmts
		l.stmts = saved
	}

	// Build the function type
	var paramTypes []*LType
	for _, p := range params {
		paramTypes = append(paramTypes, p.Type)
	}
	funcType := &LType{Kind: LTyFuncPtr, Params: paramTypes, Return: retType}

	return l.emitTemp(LExpr{
		Kind: LExprFuncLit,
		Type: funcType,
		Data: &LFuncLitData{Params: params, ReturnType: retType, Body: body},
	})
}

// emitMatch lowers a match statement/expression. If result is non-nil, each arm's
// last expression is assigned to the result variable (match-as-expression).
func (l *Lowerer) emitMatch(ms *ast.MatchStmt, result *matchResultInfo) {
	matchVal := l.lowerExpr(&ms.Value)

	// Check if this is an enum match
	isEnum := false
	var enumName string
	if matchVal.Type != nil && matchVal.Type.Kind == LTyTaggedUnion {
		isEnum = true
		enumName = matchVal.Type.Name
	}

	if isEnum {
		// Check for nested enum patterns (e.g., Some(Circle(r)))
		if l.hasNestedVariantPatterns(ms) {
			l.emitNestedEnumMatch(ms, matchVal, enumName, result)
		} else {
			// Simple enum match — one arm per variant
			tagTemp := l.emitTemp(LExpr{
				Kind: LExprVariantTag,
				Type: &LType{Kind: LTyI32, Bits: 32},
				Data: &LVariantTagData{Value: matchVal},
			})

			var cases []LSwitchCase
			for _, arm := range ms.Arms {
				sc := l.lowerMatchArm(&arm, matchVal, enumName, result)
				cases = append(cases, sc)
			}

			l.emit(LStmt{Kind: LStmtSwitch, Data: &LSwitch{
				Tag:      tagTemp,
				Cases:    cases,
				EnumName: enumName,
			}})
		}
	} else if l.isUnionMatch(matchVal, ms) {
		// Union type match: emit type switch
		l.emitUnionTypeSwitch(ms, matchVal, result)
	} else {
		// Non-enum match: emit if-else chain
		l.lowerMatchAsIfElse(ms, matchVal, result)
	}
}

// isUnionMatch checks if this is a union type match (match on any with type patterns).
func (l *Lowerer) isUnionMatch(matchVal LValue, ms *ast.MatchStmt) bool {
	if matchVal.Type == nil || (matchVal.Type.Kind != LTyAny && matchVal.Type.Kind != LTyUnion) {
		return false
	}
	for _, arm := range ms.Arms {
		if arm.Pattern.ResolvedType != nil {
			return true
		}
	}
	return false
}

// emitUnionTypeSwitch emits a type switch for union type matching.
func (l *Lowerer) emitUnionTypeSwitch(ms *ast.MatchStmt, matchVal LValue, result *matchResultInfo) {
	var cases []LTypeSwitchCase
	for _, arm := range ms.Arms {
		if arm.Pattern.Kind == ast.PatWildcard {
			// Default case
			body := l.lowerArmBody(&arm.Body, result)
			cases = append(cases, LTypeSwitchCase{
				Type: nil, // nil = default
				Body: body,
			})
			continue
		}
		if arm.Pattern.Kind == ast.PatIdent {
			// Type pattern — resolve the type from ResolvedType
			var caseType *LType
			if rt, ok := arm.Pattern.ResolvedType.(*checker.Type); ok {
				caseType = l.lowerCheckerType(rt)
			} else {
				// Fallback: use pattern name as-is
				ip := dataAs[ast.IdentPattern](arm.Pattern.Data)
				caseType = l.forgeNameToLType(ip.Name)
			}
			body := l.lowerArmBody(&arm.Body, result)
			cases = append(cases, LTypeSwitchCase{
				Type: caseType,
				Body: body,
			})
		}
	}
	l.emit(LStmt{Kind: LStmtTypeSwitch, Data: &LTypeSwitch{
		Value: matchVal,
		Cases: cases,
	}})
}

// forgeNameToLType converts a Forge type name to an LType (fallback for union patterns).
func (l *Lowerer) forgeNameToLType(name string) *LType {
	switch name {
	case "string":
		return &LType{Kind: LTyString}
	case "bool":
		return &LType{Kind: LTyBool}
	case "i8":
		return &LType{Kind: LTyI8, Bits: 8}
	case "i16":
		return &LType{Kind: LTyI16, Bits: 16}
	case "i32":
		return &LType{Kind: LTyI32, Bits: 32}
	case "i64":
		return &LType{Kind: LTyI64, Bits: 64}
	case "f32":
		return &LType{Kind: LTyF32}
	case "f64":
		return &LType{Kind: LTyF64}
	default:
		return &LType{Kind: LTyAny}
	}
}

// lowerArmBody lowers a match arm body. If result is non-nil (match-as-expression),
// the last expression in the body is assigned to the result variable.
func (l *Lowerer) lowerArmBody(block *ast.Block, result *matchResultInfo) []LStmt {
	if result == nil || len(block.Stmts) == 0 {
		return l.lowerBlock(block)
	}

	// Lower all statements except the last
	saved := l.stmts
	l.stmts = nil
	for i := 0; i < len(block.Stmts)-1; i++ {
		l.lowerStmt(&block.Stmts[i])
	}

	// Lower the last statement as an expression and assign to result
	lastStmt := &block.Stmts[len(block.Stmts)-1]
	if lastStmt.Kind == ast.StmtExpr {
		es := dataAs[ast.ExprStmt](lastStmt.Data)
		val := l.lowerExpr(&es.Expr)
		l.emit(LStmt{Kind: LStmtAssign, Data: &LAssign{
			Target: result.name,
			Value:  val,
		}})
	} else {
		// Non-expression last statement — lower normally
		l.lowerStmt(lastStmt)
	}

	body := l.stmts
	l.stmts = saved
	return body
}

// hasNestedVariantPatterns checks if any arm in an enum match has a binding
// that is itself a PatVariant (e.g., Some(Circle(r))).
func (l *Lowerer) hasNestedVariantPatterns(ms *ast.MatchStmt) bool {
	for _, arm := range ms.Arms {
		if arm.Pattern.Kind == ast.PatVariant {
			vp := dataAs[ast.VariantPattern](arm.Pattern.Data)
			for _, b := range vp.Bindings {
				if b.Kind == ast.PatVariant {
					return true
				}
			}
		}
	}
	return false
}

// emitNestedEnumMatch handles match expressions where arms have nested variant
// patterns like Some(Circle(r)), Some(Rect(w, h)). Groups arms by outer variant
// and emits nested type switches for the inner enum values.
func (l *Lowerer) emitNestedEnumMatch(ms *ast.MatchStmt, matchVal LValue, enumName string, result *matchResultInfo) {
	tagTemp := l.emitTemp(LExpr{
		Kind: LExprVariantTag,
		Type: &LType{Kind: LTyI32, Bits: 32},
		Data: &LVariantTagData{Value: matchVal},
	})

	// Group arms by outer variant name (or "" for wildcard/ident)
	type armGroup struct {
		outerName string
		arms      []ast.MatchArm
	}
	var groups []armGroup
	groupIdx := map[string]int{}

	for _, arm := range ms.Arms {
		key := ""
		if arm.Pattern.Kind == ast.PatVariant {
			vp := dataAs[ast.VariantPattern](arm.Pattern.Data)
			key = vp.Name
		} else if arm.Pattern.Kind == ast.PatIdent {
			ip := dataAs[ast.IdentPattern](arm.Pattern.Data)
			if _, ok := l.unitVariants[ip.Name]; ok {
				key = ip.Name
			}
		}

		if idx, ok := groupIdx[key]; ok {
			groups[idx].arms = append(groups[idx].arms, arm)
		} else {
			groupIdx[key] = len(groups)
			groups = append(groups, armGroup{outerName: key, arms: []ast.MatchArm{arm}})
		}
	}

	var cases []LSwitchCase
	for _, g := range groups {
		if len(g.arms) == 1 && !l.armHasNestedVariant(&g.arms[0]) {
			// Simple arm — no nesting, use normal lowering
			sc := l.lowerMatchArm(&g.arms[0], matchVal, enumName, result)
			cases = append(cases, sc)
			continue
		}

		// Multiple arms for same outer variant, or nested variant patterns.
		// Emit one case with a nested type switch on the inner value.
		outerVP := dataAs[ast.VariantPattern](g.arms[0].Pattern.Data)
		tag := l.findVariantTag(enumName, outerVP.Name)

		// Extract the inner value (the first field of the outer variant)
		innerFieldName := ""
		var innerFieldType *LType
		if info, ok := l.variantCtors[outerVP.Name]; ok && len(info.fieldNames) > 0 {
			innerFieldName = info.fieldNames[0]
		}
		if variants, ok := l.enumVariants[enumName]; ok {
			for _, v := range variants {
				if v.Name == outerVP.Name && len(v.Fields) > 0 {
					innerFieldType = v.Fields[0].Type
				}
			}
		}
		if innerFieldType == nil {
			innerFieldType = &LType{Kind: LTyAny}
		}

		saved := l.stmts
		l.stmts = nil

		// Extract inner value from outer variant
		innerVal := l.emitTemp(LExpr{
			Kind: LExprVariantData,
			Type: innerFieldType,
			Data: &LVariantDataData{
				Value:   matchVal,
				Enum:    enumName,
				Variant: outerVP.Name,
				Field:   innerFieldName,
			},
		})

		// Determine inner enum name from the first nested variant
		innerEnumName := ""
		for _, arm := range g.arms {
			if arm.Pattern.Kind == ast.PatVariant {
				vp := dataAs[ast.VariantPattern](arm.Pattern.Data)
				for _, b := range vp.Bindings {
					if b.Kind == ast.PatVariant {
						innerVP := dataAs[ast.VariantPattern](b.Data)
						if info, ok := l.variantCtors[innerVP.Name]; ok {
							innerEnumName = info.enumName
						}
					}
				}
			}
			if innerEnumName != "" {
				break
			}
		}

		// Emit nested tag extraction + switch
		innerTagTemp := l.emitTemp(LExpr{
			Kind: LExprVariantTag,
			Type: &LType{Kind: LTyI32, Bits: 32},
			Data: &LVariantTagData{Value: innerVal},
		})

		var innerCases []LSwitchCase
		for _, arm := range g.arms {
			vp := dataAs[ast.VariantPattern](arm.Pattern.Data)
			if len(vp.Bindings) == 1 && vp.Bindings[0].Kind == ast.PatVariant {
				// Nested variant: e.g., Some(Circle(r))
				innerVP := dataAs[ast.VariantPattern](vp.Bindings[0].Data)
				innerTag := l.findVariantTag(innerEnumName, innerVP.Name)

				savedInner := l.stmts
				l.stmts = nil

				// Extract fields from inner variant
				if info, ok := l.variantCtors[innerVP.Name]; ok {
					for i, binding := range innerVP.Bindings {
						if binding.Kind == ast.PatIdent {
							bp := dataAs[ast.IdentPattern](binding.Data)
							fieldName := ""
							if i < len(info.fieldNames) {
								fieldName = info.fieldNames[i]
							}
							var fieldType *LType
							if variants, ok := l.enumVariants[innerEnumName]; ok {
								for _, v := range variants {
									if v.Name == innerVP.Name && i < len(v.Fields) {
										fieldType = v.Fields[i].Type
									}
								}
							}
							if fieldType == nil {
								fieldType = &LType{Kind: LTyAny}
							}

							extractTemp := l.emitTemp(LExpr{
								Kind: LExprVariantData,
								Type: fieldType,
								Data: &LVariantDataData{
									Value:   innerVal,
									Enum:    innerEnumName,
									Variant: innerVP.Name,
									Field:   fieldName,
								},
							})
							l.emit(LStmt{Kind: LStmtVarDecl, Data: &LVarDecl{
								Name: bp.Name,
								Type: fieldType,
								Init: &extractTemp,
							}})
						}
					}
				}

				bindingStmts := l.stmts
				l.stmts = savedInner

				bodyStmts := l.lowerArmBody(&arm.Body, result)
				innerCases = append(innerCases, LSwitchCase{
					Tag:  innerTag,
					Body: append(bindingStmts, bodyStmts...),
				})
			} else {
				// Non-nested binding within group — shouldn't happen in well-formed code
				sc := l.lowerMatchArm(&arm, matchVal, enumName, result)
				sc.Tag = -1 // default within inner switch
				innerCases = append(innerCases, sc)
			}
		}

		l.emit(LStmt{Kind: LStmtSwitch, Data: &LSwitch{
			Tag:      innerTagTemp,
			Cases:    innerCases,
			EnumName: innerEnumName,
		}})

		bodyStmts := l.stmts
		l.stmts = saved

		cases = append(cases, LSwitchCase{Tag: tag, Body: bodyStmts})
	}

	l.emit(LStmt{Kind: LStmtSwitch, Data: &LSwitch{
		Tag:      tagTemp,
		Cases:    cases,
		EnumName: enumName,
	}})
}

// armHasNestedVariant checks if a single arm has nested variant patterns.
func (l *Lowerer) armHasNestedVariant(arm *ast.MatchArm) bool {
	if arm.Pattern.Kind == ast.PatVariant {
		vp := dataAs[ast.VariantPattern](arm.Pattern.Data)
		for _, b := range vp.Bindings {
			if b.Kind == ast.PatVariant {
				return true
			}
		}
	}
	return false
}

func (l *Lowerer) lowerMatchArm(arm *ast.MatchArm, matchVal LValue, enumName string, result *matchResultInfo) LSwitchCase {
	switch arm.Pattern.Kind {
	case ast.PatWildcard:
		body := l.lowerArmBody(&arm.Body, result)
		if arm.Guard != nil {
			guardVal := l.lowerExpr(arm.Guard)
			body = []LStmt{{Kind: LStmtIf, Data: &LIf{Cond: guardVal, Then: body}}}
		}
		return LSwitchCase{Tag: -1, Body: body}

	case ast.PatIdent:
		ip := dataAs[ast.IdentPattern](arm.Pattern.Data)
		// Check if this is a unit variant
		if en, ok := l.unitVariants[ip.Name]; ok && en == enumName {
			tag := l.findVariantTag(enumName, ip.Name)
			body := l.lowerArmBody(&arm.Body, result)
			return LSwitchCase{Tag: tag, Body: body}
		}
		// Otherwise it's a binding — default case that binds the value
		body := l.lowerArmBody(&arm.Body, result)
		return LSwitchCase{Tag: -1, Binding: ip.Name, Body: body}

	case ast.PatVariant:
		vp := dataAs[ast.VariantPattern](arm.Pattern.Data)
		tag := l.findVariantTag(enumName, vp.Name)

		// Extract variant data fields into bindings
		saved := l.stmts
		l.stmts = nil

		if info, ok := l.variantCtors[vp.Name]; ok {
			for i, binding := range vp.Bindings {
				if binding.Kind == ast.PatIdent {
					bp := dataAs[ast.IdentPattern](binding.Data)
					fieldName := ""
					if i < len(info.fieldNames) {
						fieldName = info.fieldNames[i]
					}
					var fieldType *LType
					if variants, ok := l.enumVariants[enumName]; ok {
						for _, v := range variants {
							if v.Name == vp.Name && i < len(v.Fields) {
								fieldType = v.Fields[i].Type
							}
						}
					}
					if fieldType == nil {
						fieldType = &LType{Kind: LTyAny}
					}

					extractTemp := l.emitTemp(LExpr{
						Kind: LExprVariantData,
						Type: fieldType,
						Data: &LVariantDataData{
							Value:   matchVal,
							Enum:    enumName,
							Variant: vp.Name,
							Field:   fieldName,
						},
					})
					l.emit(LStmt{Kind: LStmtVarDecl, Data: &LVarDecl{
						Name: bp.Name,
						Type: fieldType,
						Init: &extractTemp,
					}})
				}
			}
		}

		bindingStmts := l.stmts
		l.stmts = saved

		bodyStmts := l.lowerArmBody(&arm.Body, result)
		allStmts := append(bindingStmts, bodyStmts...)

		if arm.Guard != nil {
			guardVal := l.lowerExpr(arm.Guard)
			allStmts = []LStmt{{Kind: LStmtIf, Data: &LIf{Cond: guardVal, Then: allStmts}}}
		}

		return LSwitchCase{Tag: tag, Body: allStmts}

	case ast.PatLiteral:
		// Literal patterns in enum match — shouldn't happen, fallback to default
		body := l.lowerArmBody(&arm.Body, result)
		return LSwitchCase{Tag: -1, Body: body}
	}

	return LSwitchCase{Tag: -1, Body: l.lowerArmBody(&arm.Body, result)}
}

func (l *Lowerer) lowerMatchAsIfElse(ms *ast.MatchStmt, matchVal LValue, result *matchResultInfo) {
	// For non-enum matches, emit an if-else chain comparing matchVal to each pattern
	for i, arm := range ms.Arms {
		if arm.Pattern.Kind == ast.PatWildcard {
			// Default — just emit the body
			body := l.lowerArmBody(&arm.Body, result)
			for _, s := range body {
				l.emit(s)
			}
			return
		}

		if arm.Pattern.Kind == ast.PatIdent {
			// Binding pattern: x or x if guard
			ip := dataAs[ast.IdentPattern](arm.Pattern.Data)
			// Emit binding: let x = matchVal
			l.emit(LStmt{Kind: LStmtVarDecl, Data: &LVarDecl{
				Name: ip.Name,
				Type: matchVal.Type,
				Init: &matchVal,
			}})

			if arm.Guard != nil {
				// Guarded binding: x if x < 0 => { ... }
				guardVal := l.lowerExpr(arm.Guard)
				body := l.lowerArmBody(&arm.Body, result)

				var elseStmts []LStmt
				if i < len(ms.Arms)-1 {
					saved := l.stmts
					l.stmts = nil
					remaining := &ast.MatchStmt{Value: ms.Value, Arms: ms.Arms[i+1:]}
					l.lowerMatchAsIfElse(remaining, matchVal, result)
					elseStmts = l.stmts
					l.stmts = saved
				}

				l.emit(LStmt{Kind: LStmtIf, Data: &LIf{
					Cond: guardVal,
					Then: body,
					Else: elseStmts,
				}})
			} else {
				// Unguarded binding: treat like wildcard with binding
				body := l.lowerArmBody(&arm.Body, result)
				for _, s := range body {
					l.emit(s)
				}
			}
			return
		}

		if arm.Pattern.Kind == ast.PatTuple {
			tp := dataAs[ast.TuplePattern](arm.Pattern.Data)
			// Build condition from tuple element comparisons
			// For each non-wildcard element, compare against matchVal elements
			// matchVal is the tuple value; we need to extract elements
			var conds []LValue
			var bindings []LStmt
			for ei, elemPat := range tp.Elems {
				// Extract the element from the tuple
				// For now, assume matchVal was lowered from a tuple expression
				// and the individual values are accessible via the tuple's components
				// We need the original tuple expressions
				_ = ei
				switch elemPat.Kind {
				case ast.PatWildcard:
					// No condition needed
				case ast.PatLiteral:
					lp := dataAs[ast.LiteralPattern](elemPat.Data)
					litVal := l.lowerExpr(&lp.Expr)
					// Get the ei-th element of the tuple
					elemVal := l.getTupleElement(ms, matchVal, ei)
					cmpTemp := l.emitTemp(LExpr{
						Kind: LExprBinOp,
						Type: &LType{Kind: LTyBool},
						Data: &LBinOpData{Op: LBinEq, Left: elemVal, Right: litVal},
					})
					conds = append(conds, cmpTemp)
				case ast.PatIdent:
					ip := dataAs[ast.IdentPattern](elemPat.Data)
					elemVal := l.getTupleElement(ms, matchVal, ei)
					bindings = append(bindings, LStmt{Kind: LStmtVarDecl, Data: &LVarDecl{
						Name: ip.Name,
						Type: elemVal.Type,
						Init: &elemVal,
					}})
				}
			}

			// Combine conditions with &&
			var condVal LValue
			if len(conds) == 0 {
				condVal = LValue{Kind: LValLitBool, BoolVal: true, Type: &LType{Kind: LTyBool}}
			} else {
				condVal = conds[0]
				for _, c := range conds[1:] {
					condVal = l.emitTemp(LExpr{
						Kind: LExprBinOp,
						Type: &LType{Kind: LTyBool},
						Data: &LBinOpData{Op: LBinAnd, Left: condVal, Right: c},
					})
				}
			}

			// Emit bindings inside the then block
			saved := l.stmts
			l.stmts = nil
			for _, b := range bindings {
				l.emit(b)
			}
			armBody := l.lowerArmBody(&arm.Body, result)
			thenBody := append(l.stmts, armBody...)
			l.stmts = saved

			var elseStmts []LStmt
			if i < len(ms.Arms)-1 {
				saved2 := l.stmts
				l.stmts = nil
				remaining := &ast.MatchStmt{Value: ms.Value, Arms: ms.Arms[i+1:]}
				l.lowerMatchAsIfElse(remaining, matchVal, result)
				elseStmts = l.stmts
				l.stmts = saved2
			}

			l.emit(LStmt{Kind: LStmtIf, Data: &LIf{
				Cond: condVal,
				Then: thenBody,
				Else: elseStmts,
			}})
			return
		}

		if arm.Pattern.Kind == ast.PatLiteral {
			lp := dataAs[ast.LiteralPattern](arm.Pattern.Data)
			litVal := l.lowerExpr(&lp.Expr)
			cmpTemp := l.emitTemp(LExpr{
				Kind: LExprBinOp,
				Type: &LType{Kind: LTyBool},
				Data: &LBinOpData{Op: LBinEq, Left: matchVal, Right: litVal},
			})
			body := l.lowerArmBody(&arm.Body, result)

			var elseStmts []LStmt
			if i < len(ms.Arms)-1 {
				// Remaining arms become else
				saved := l.stmts
				l.stmts = nil
				remaining := &ast.MatchStmt{Value: ms.Value, Arms: ms.Arms[i+1:]}
				l.lowerMatchAsIfElse(remaining, matchVal, result)
				elseStmts = l.stmts
				l.stmts = saved
			}

			l.emit(LStmt{Kind: LStmtIf, Data: &LIf{
				Cond: cmpTemp,
				Then: body,
				Else: elseStmts,
			}})
			return
		}

	}
}

// getTupleElement extracts the i-th element from a tuple match expression.
// Since tuple literals lower each element independently, we re-lower from the AST.
func (l *Lowerer) getTupleElement(ms *ast.MatchStmt, matchVal LValue, idx int) LValue {
	if ms.Value.Kind == ast.ExprTupleLit {
		tl := dataAs[ast.TupleLitExpr](ms.Value.Data)
		if idx < len(tl.Elems) {
			return l.lowerExpr(&tl.Elems[idx])
		}
	}
	// Fallback: return matchVal (shouldn't happen for valid tuple patterns)
	return matchVal
}

func (l *Lowerer) findVariantTag(enumName, variantName string) int {
	if variants, ok := l.enumVariants[enumName]; ok {
		for _, v := range variants {
			if v.Name == variantName {
				return v.Tag
			}
		}
	}
	return -1
}

func (l *Lowerer) lowerBlockStmt(stmt *ast.Stmt) {
	bs := dataAs[ast.Block](stmt.Data)
	l.emit(LStmt{Kind: LStmtBlock, Data: &LBlock{Stmts: l.lowerBlock(bs)}})
}

func (l *Lowerer) lowerSelectStmt(stmt *ast.Stmt) {
	ss := dataAs[ast.SelectStmt](stmt.Data)
	var cases []LSelectCase
	for _, c := range ss.Cases {
		sc := l.lowerSelectCase(&c)
		cases = append(cases, sc)
	}
	l.emit(LStmt{Kind: LStmtSelect, Data: &LSelect{Cases: cases}})
}

func (l *Lowerer) lowerSelectCase(c *ast.SelectCase) LSelectCase {
	if c.IsDefault {
		return LSelectCase{
			Kind: LSelectDefault,
			Body: l.lowerBlock(&c.Body),
		}
	}

	// Determine if this is a send or receive by inspecting the expression
	if c.Expr != nil && c.Expr.Kind == ast.ExprMethodCall {
		mc := dataAs[ast.MethodCallExpr](c.Expr.Data)
		ch := l.lowerExpr(&mc.Receiver)
		if mc.Method == "send" && len(mc.Args) > 0 {
			val := l.lowerExpr(&mc.Args[0])
			return LSelectCase{
				Kind:    LSelectSend,
				Channel: ch,
				Value:   val,
				Body:    l.lowerBlock(&c.Body),
			}
		}
		if mc.Method == "receive" {
			return LSelectCase{
				Kind:    LSelectRecv,
				Channel: ch,
				Binding: c.BindVar,
				Body:    l.lowerBlock(&c.Body),
			}
		}
	}
	// Fallback
	return LSelectCase{Kind: LSelectDefault, Body: l.lowerBlock(&c.Body)}
}

// ---------------------------------------------------------------------------
// Expression lowering — flattens all expressions into temps
// ---------------------------------------------------------------------------

// lowerExpr flattens an AST expression into a sequence of TempDefs and returns
// the final LValue referencing the result.
func (l *Lowerer) lowerExpr(expr *ast.Expr) LValue {
	if expr == nil {
		return LValue{Kind: LValLitNull, Type: &LType{Kind: LTyUnit}}
	}

	switch expr.Kind {
	case ast.ExprIdent:
		return l.lowerIdent(expr)
	case ast.ExprIntLit:
		return l.lowerIntLit(expr)
	case ast.ExprFloatLit:
		return l.lowerFloatLit(expr)
	case ast.ExprStringLit:
		return l.lowerStringLit(expr)
	case ast.ExprBoolLit:
		return l.lowerBoolLit(expr)
	case ast.ExprNil:
		// Use checker's resolved type if available (important for generics —
		// Go rejects bare nil for type parameters)
		nilType := l.exprType(expr)
		if nilType.Kind == LTyAny {
			nilType = &LType{Kind: LTyAny}
		}
		return LValue{Kind: LValLitNull, Type: nilType}
	case ast.ExprBinary:
		return l.lowerBinary(expr)
	case ast.ExprUnary:
		return l.lowerUnary(expr)
	case ast.ExprCall:
		return l.lowerCall(expr)
	case ast.ExprMethodCall:
		return l.lowerMethodCall(expr)
	case ast.ExprFieldAccess:
		return l.lowerFieldAccess(expr)
	case ast.ExprIndex:
		return l.lowerIndex(expr)
	case ast.ExprSlice:
		return l.lowerSliceExpr(expr)
	case ast.ExprListLit:
		return l.lowerListLit(expr)
	case ast.ExprMapLit:
		return l.lowerMapLit(expr)
	case ast.ExprTupleLit:
		return l.lowerTupleLit(expr)
	case ast.ExprStructLit:
		return l.lowerStructLit(expr)
	case ast.ExprStringInterp:
		return l.lowerStringInterp(expr)
	case ast.ExprCast:
		return l.lowerCast(expr)
	case ast.ExprUnwrap:
		return l.lowerUnwrap(expr)
	case ast.ExprTry:
		return l.lowerTry(expr)
	case ast.ExprLambda:
		return l.lowerLambda(expr)
	case ast.ExprMatch:
		return l.lowerMatchExpr(expr)
	}
	return LValue{Kind: LValLitNull, Type: &LType{Kind: LTyUnit}}
}

// lowerExprInto is like lowerExpr but used in contexts where we need the
// expression flattened into the current statement list (e.g., else-if conditions).
func (l *Lowerer) lowerExprInto(expr *ast.Expr) LValue {
	return l.lowerExpr(expr)
}

// emitTemp creates a new temporary and emits a TempDef statement.
func (l *Lowerer) emitTemp(expr LExpr) LValue {
	id := l.nextTemp
	l.nextTemp++
	l.emit(LStmt{Kind: LStmtTempDef, Data: &LTempDef{ID: id, Expr: expr}})
	return LValue{Kind: LValTemp, TempID: id, Type: expr.Type}
}

// exprType extracts the checker-annotated type from an AST expression.
func (l *Lowerer) exprType(expr *ast.Expr) *LType {
	if expr.ResolvedType != nil {
		if ct, ok := expr.ResolvedType.(*checker.Type); ok {
			return l.lowerCheckerType(ct)
		}
	}
	return &LType{Kind: LTyAny}
}

// --- Individual expression lowering ---

func (l *Lowerer) lowerIdent(expr *ast.Expr) LValue {
	ie := dataAs[ast.IdentExpr](expr.Data)

	// Check if it's a unit variant
	if enumName, ok := l.unitVariants[ie.Name]; ok {
		tag := l.findVariantTag(enumName, ie.Name)
		return l.emitTemp(LExpr{
			Kind: LExprVariantConstruct,
			Type: &LType{Kind: LTyTaggedUnion, Name: enumName, IsExported: l.exported[enumName]},
			Data: &LVariantConstructData{
				Enum:    enumName,
				Variant: ie.Name,
				Tag:     tag,
			},
		})
	}

	return LValue{Kind: LValVar, Name: ie.Name, Type: l.exprType(expr)}
}

func (l *Lowerer) lowerIntLit(expr *ast.Expr) LValue {
	il := dataAs[ast.IntLitExpr](expr.Data)
	val, _ := strconv.ParseInt(il.Value, 0, 64)
	typ := l.exprType(expr)
	if typ.Kind == LTyAny {
		typ = &LType{Kind: LTyI32, Bits: 32}
	}
	return LValue{Kind: LValLitInt, IntVal: val, Type: typ}
}

func (l *Lowerer) lowerFloatLit(expr *ast.Expr) LValue {
	fl := dataAs[ast.FloatLitExpr](expr.Data)
	val, _ := strconv.ParseFloat(fl.Value, 64)
	typ := l.exprType(expr)
	if typ.Kind == LTyAny {
		typ = &LType{Kind: LTyF64, Bits: 64}
	}
	return LValue{Kind: LValLitFloat, FloatVal: val, Type: typ}
}

func (l *Lowerer) lowerStringLit(expr *ast.Expr) LValue {
	sl := dataAs[ast.StringLitExpr](expr.Data)
	return LValue{Kind: LValLitString, StrVal: sl.Value, Type: &LType{Kind: LTyString}}
}

func (l *Lowerer) lowerBoolLit(expr *ast.Expr) LValue {
	bl := dataAs[ast.BoolLitExpr](expr.Data)
	return LValue{Kind: LValLitBool, BoolVal: bl.Value, Type: &LType{Kind: LTyBool}}
}

func (l *Lowerer) lowerBinary(expr *ast.Expr) LValue {
	be := dataAs[ast.BinaryExpr](expr.Data)
	left := l.lowerExpr(&be.Left)
	right := l.lowerExpr(&be.Right)

	op := mapBinaryOp(be.Op)
	resultType := l.exprType(expr)
	if resultType.Kind == LTyAny {
		// Infer: comparison → bool, arithmetic → left type
		if op >= LBinEq && op <= LBinGe || op == LBinAnd || op == LBinOr {
			resultType = &LType{Kind: LTyBool}
		} else {
			resultType = left.Type
		}
	}

	// Insert numeric casts when operand types mismatch
	left, right = l.coerceNumericBinary(left, right, resultType)

	return l.emitTemp(LExpr{
		Kind: LExprBinOp,
		Type: resultType,
		Data: &LBinOpData{Op: op, Left: left, Right: right},
	})
}

// coerceNumericBinary inserts LExprCast when binary operands have different numeric types.
func (l *Lowerer) coerceNumericBinary(left, right LValue, resultType *LType) (LValue, LValue) {
	if left.Type == nil || right.Type == nil || resultType == nil {
		return left, right
	}
	if !isNumericKind(left.Type.Kind) || !isNumericKind(right.Type.Kind) {
		return left, right
	}
	if left.Type.Kind == right.Type.Kind {
		return left, right
	}
	// For comparisons (result is bool), coerce to the wider operand type, not bool
	target := resultType
	if resultType.Kind == LTyBool {
		target = l.widerNumericType(left.Type, right.Type)
	}
	if left.Type.Kind != target.Kind {
		left = l.emitTemp(LExpr{
			Kind: LExprCast,
			Type: target,
			Data: &LCastData{Operand: left, Target: target},
		})
	}
	if right.Type.Kind != target.Kind {
		right = l.emitTemp(LExpr{
			Kind: LExprCast,
			Type: target,
			Data: &LCastData{Operand: right, Target: target},
		})
	}
	return left, right
}

func isNumericKind(k LTypeKind) bool {
	switch k {
	case LTyI8, LTyI16, LTyI32, LTyI64, LTyU8, LTyU16, LTyU32, LTyU64,
		LTyF32, LTyF64, LTyPlatformInt, LTyPlatformUint:
		return true
	}
	return false
}

// widerNumericType returns the wider of two numeric types for coercion.
// Platform int/uint dominate fixed-width types.
func (l *Lowerer) widerNumericType(a, b *LType) *LType {
	if a.Kind == b.Kind {
		return a
	}
	// Platform int dominates
	if a.Kind == LTyPlatformInt || a.Kind == LTyPlatformUint {
		return a
	}
	if b.Kind == LTyPlatformInt || b.Kind == LTyPlatformUint {
		return b
	}
	// Float dominates int
	if a.Kind == LTyF64 || a.Kind == LTyF32 {
		return a
	}
	if b.Kind == LTyF64 || b.Kind == LTyF32 {
		return b
	}
	// Wider bits wins
	if a.Bits >= b.Bits {
		return a
	}
	return b
}

func mapBinaryOp(op ast.BinaryOp) LBinOpKind {
	switch op {
	case ast.OpAdd:
		return LBinAdd
	case ast.OpSub:
		return LBinSub
	case ast.OpMul:
		return LBinMul
	case ast.OpDiv:
		return LBinDiv
	case ast.OpMod:
		return LBinMod
	case ast.OpEq:
		return LBinEq
	case ast.OpNeq:
		return LBinNe
	case ast.OpLt:
		return LBinLt
	case ast.OpLe:
		return LBinLe
	case ast.OpGt:
		return LBinGt
	case ast.OpGe:
		return LBinGe
	case ast.OpAnd:
		return LBinAnd
	case ast.OpOr:
		return LBinOr
	case ast.OpBitAnd:
		return LBinBitAnd
	case ast.OpBitOr:
		return LBinBitOr
	case ast.OpBitXor:
		return LBinBitXor
	case ast.OpShl:
		return LBinShl
	case ast.OpShr:
		return LBinShr
	}
	return LBinAdd
}

func (l *Lowerer) lowerUnary(expr *ast.Expr) LValue {
	ue := dataAs[ast.UnaryExpr](expr.Data)
	operand := l.lowerExpr(&ue.Operand)

	var op LUnOpKind
	switch ue.Op {
	case ast.OpNeg:
		op = LUnNeg
	case ast.OpNot:
		op = LUnNot
	}

	resultType := l.exprType(expr)
	if resultType.Kind == LTyAny {
		resultType = operand.Type
	}

	return l.emitTemp(LExpr{
		Kind: LExprUnOp,
		Type: resultType,
		Data: &LUnOpData{Op: op, Operand: operand},
	})
}

func (l *Lowerer) lowerCall(expr *ast.Expr) LValue {
	ce := dataAs[ast.CallExpr](expr.Data)

	// Extract type arguments (explicit or inferred)
	typeArgs := l.extractTypeArgs(ce)

	// Lower arguments
	var args []LValue
	for _, arg := range ce.Args {
		args = append(args, l.lowerExpr(&arg))
	}

	// Get function name
	var funcName string
	if ce.Func.Kind == ast.ExprIdent {
		funcName = dataAs[ast.IdentExpr](ce.Func.Data).Name
	} else if ce.Func.Kind == ast.ExprFieldAccess {
		fa := dataAs[ast.FieldAccessExpr](ce.Func.Data)
		if fa.Receiver.Kind == ast.ExprIdent {
			funcName = dataAs[ast.IdentExpr](fa.Receiver.Data).Name + "." + fa.Field
		}
	}

	// Check for built-in functions
	switch funcName {
	case "println", "print", "len", "append", "isnull", "hash_string":
		return l.emitTemp(LExpr{
			Kind: LExprBuiltin,
			Type: l.exprType(expr),
			Data: &LBuiltinData{Name: funcName, Args: args},
		})
	case "make_channel":
		// make_channel<T>() or make_channel<T>(bufSize)
		resultType := l.exprType(expr)
		var elemType *LType
		if resultType != nil && resultType.Kind == LTyChannel {
			elemType = resultType.Elem
		} else {
			elemType = &LType{Kind: LTyAny}
		}
		var bufSize *LValue
		if len(args) > 0 {
			bufSize = &args[0]
		}
		return l.emitTemp(LExpr{
			Kind: LExprMakeChannel,
			Type: resultType,
			Data: &LMakeChannelData{ElemType: elemType, BufSize: bufSize},
		})
	}

	// Check for package-qualified calls (e.g., fmt.Println, errors.New)
	if strings.Contains(funcName, ".") {
		parts := strings.SplitN(funcName, ".", 2)
		if _, isImport := l.importAliases[parts[0]]; isImport {
			return l.emitTemp(LExpr{
				Kind: LExprCall,
				Type: l.exprType(expr),
				Data: &LCallData{Func: funcName, Args: args, TypeArgs: typeArgs, IsExported: true},
			})
		}
	}

	// Check if it's a variant constructor
	if info, ok := l.variantCtors[funcName]; ok {
		var fieldVals []LValue
		fieldVals = append(fieldVals, args...)
		return l.emitTemp(LExpr{
			Kind: LExprVariantConstruct,
			Type: &LType{Kind: LTyTaggedUnion, Name: info.enumName, IsExported: l.exported[info.enumName]},
			Data: &LVariantConstructData{
				Enum:    info.enumName,
				Variant: funcName,
				Tag:     info.tag,
				Fields:  fieldVals,
			},
		})
	}

	// Check if it's a class constructor
	if _, ok := l.classCtorFields[funcName]; ok {
		// Build struct literal from args using ctor field names
		var fieldInits []LFieldInit
		ctorFields := l.classCtorFields[funcName]
		allFields := l.classFields[funcName]
		for i, arg := range args {
			name := ""
			if i < len(ctorFields) {
				name = ctorFields[i]
			}
			// Auto-wrap non-optional arg when field is optional
			val := arg
			// Propagate field type to untyped slice args (e.g. empty list literals)
			if i < len(allFields) && allFields[i].Type != nil && allFields[i].Type.Kind == LTySlice &&
				val.Type != nil && val.Type.Kind == LTySlice &&
				(val.Type.Elem == nil || val.Type.Elem.Kind == LTyAny) {
				val.Type = allFields[i].Type
				// Also update the underlying temp's expression type so the Go backend emits correct type
				if val.Kind == LValTemp {
					for si := len(l.stmts) - 1; si >= 0; si-- {
						if l.stmts[si].Kind == LStmtTempDef {
							td := l.stmts[si].Data.(*LTempDef)
							if td.ID == val.TempID {
								td.Expr.Type = allFields[i].Type
								break
							}
						}
					}
				}
			}
			if i < len(allFields) && allFields[i].Type != nil && allFields[i].Type.Kind == LTyOptional {
				if val.Type == nil || val.Type.Kind != LTyOptional {
					if val.Kind != LValLitNull {
						val = l.emitTemp(LExpr{
							Kind: LExprWrapOptional,
							Type: allFields[i].Type,
							Data: &LWrapOptionalData{Value: val},
						})
					}
				}
			}
			fieldInits = append(fieldInits, LFieldInit{Name: name, Value: val})
		}
		return l.emitTemp(LExpr{
			Kind: LExprClassAlloc,
			Type: &LType{Kind: LTyClassHandle, Name: funcName, IsExported: l.exported[funcName]},
			Data: &LClassAllocData{Class: funcName, Fields: fieldInits, TypeArgs: typeArgs},
		})
	}

	// Regular function call
	return l.emitTemp(LExpr{
		Kind: LExprCall,
		Type: l.exprType(expr),
		Data: &LCallData{Func: funcName, Args: args, TypeArgs: typeArgs, IsExported: l.exported[funcName]},
	})
}

// extractTypeArgs gets type arguments from a CallExpr, either explicit or inferred by the checker.
func (l *Lowerer) extractTypeArgs(ce *ast.CallExpr) []*LType {
	if len(ce.TypeArgs) > 0 {
		// Explicit type arguments: f<i32>(x)
		var typeArgs []*LType
		for _, ta := range ce.TypeArgs {
			typeArgs = append(typeArgs, l.lowerTypeExpr(&ta))
		}
		return typeArgs
	}
	if len(ce.InferredTypeArgs) > 0 {
		// Inferred by checker: stored as []*checker.Type via any
		var typeArgs []*LType
		for _, ta := range ce.InferredTypeArgs {
			if ct, ok := ta.(*checker.Type); ok {
				typeArgs = append(typeArgs, l.lowerCheckerType(ct))
			}
		}
		return typeArgs
	}
	return nil
}

func (l *Lowerer) lowerMethodCall(expr *ast.Expr) LValue {
	mc := dataAs[ast.MethodCallExpr](expr.Data)

	// Extract explicit type arguments
	var typeArgs []*LType
	for _, ta := range mc.TypeArgs {
		typeArgs = append(typeArgs, l.lowerTypeExpr(&ta))
	}

	// Check if receiver is an import package (e.g., fmt.Println, errors.New)
	if mc.Receiver.Kind == ast.ExprIdent {
		identName := dataAs[ast.IdentExpr](mc.Receiver.Data).Name
		if _, isImport := l.importAliases[identName]; isImport {
			// Package-qualified call — lower args and emit as LExprCall
			var args []LValue
			for _, arg := range mc.Args {
				args = append(args, l.lowerExpr(&arg))
			}
			funcName := identName + "." + mc.Method
			return l.emitTemp(LExpr{
				Kind: LExprCall,
				Type: l.exprType(expr),
				Data: &LCallData{Func: funcName, Args: args, TypeArgs: typeArgs, IsExported: true},
			})
		}
	}

	recv := l.lowerExpr(&mc.Receiver)

	var args []LValue
	var paramTypes []*LType
	for _, arg := range mc.Args {
		args = append(args, l.lowerExpr(&arg))
		// Capture checker-resolved param type for nil arg handling in Go backend
		paramTypes = append(paramTypes, l.exprType(&arg))
	}

	resultType := l.exprType(expr)

	// Check for built-in methods on primitives (string, list, map)
	if recv.Type != nil {
		switch recv.Type.Kind {
		case LTyString:
			return l.lowerBuiltinMethod(recv, mc.Method, args, resultType)
		case LTySlice:
			return l.lowerBuiltinMethod(recv, mc.Method, args, resultType)
		case LTyMap:
			return l.lowerBuiltinMethod(recv, mc.Method, args, resultType)
		case LTyChannel:
			return l.lowerChannelMethod(recv, mc.Method, args, resultType)
		}
	}

	// Regular method call: emit as method call on receiver
	return l.emitTemp(LExpr{
		Kind: LExprMethodCall,
		Type: resultType,
		Data: &LMethodCallData{
			Receiver:   recv,
			Method:     mc.Method,
			Args:       args,
			TypeArgs:   typeArgs,
			IsExported: l.exported[mc.Method],
			ParamTypes: paramTypes,
		},
	})
}

func (l *Lowerer) lowerBuiltinMethod(recv LValue, method string, args []LValue, resultType *LType) LValue {
	builtinName := ""
	switch recv.Type.Kind {
	case LTyString:
		builtinName = "string_" + method
	case LTySlice:
		builtinName = "slice_" + method
	case LTyMap:
		builtinName = "map_" + method
	}

	allArgs := append([]LValue{recv}, args...)
	return l.emitTemp(LExpr{
		Kind: LExprBuiltin,
		Type: resultType,
		Data: &LBuiltinData{Name: builtinName, Args: allArgs},
	})
}

func (l *Lowerer) lowerChannelMethod(recv LValue, method string, args []LValue, resultType *LType) LValue {
	switch method {
	case "send":
		if len(args) > 0 {
			l.emit(LStmt{Kind: LStmtSend, Data: &LSend{Channel: recv, Value: args[0]}})
			return LValue{Kind: LValLitNull, Type: &LType{Kind: LTyUnit}}
		}
	case "receive":
		return l.emitTemp(LExpr{
			Kind: LExprBuiltin,
			Type: resultType,
			Data: &LBuiltinData{Name: "channel_receive", Args: []LValue{recv}},
		})
	case "close":
		return l.emitTemp(LExpr{
			Kind: LExprBuiltin,
			Type: &LType{Kind: LTyUnit},
			Data: &LBuiltinData{Name: "channel_close", Args: []LValue{recv}},
		})
	}
	return LValue{Kind: LValLitNull, Type: &LType{Kind: LTyUnit}}
}

func (l *Lowerer) lowerFieldAccess(expr *ast.Expr) LValue {
	fa := dataAs[ast.FieldAccessExpr](expr.Data)
	recv := l.lowerExpr(&fa.Receiver)
	resultType := l.exprType(expr)

	if recv.Type != nil && recv.Type.Kind == LTyClassHandle {
		return l.emitTemp(LExpr{
			Kind: LExprClassGet,
			Type: resultType,
			Data: &LClassGetData{Handle: recv, Class: recv.Type.Name, Field: fa.Field},
		})
	}

	return l.emitTemp(LExpr{
		Kind: LExprStructField,
		Type: resultType,
		Data: &LStructFieldData{Receiver: recv, Field: fa.Field},
	})
}

func (l *Lowerer) lowerIndex(expr *ast.Expr) LValue {
	ie := dataAs[ast.IndexExpr](expr.Data)
	coll := l.lowerExpr(&ie.Receiver)
	idx := l.lowerExpr(&ie.Index)

	return l.emitTemp(LExpr{
		Kind: LExprIndexGet,
		Type: l.exprType(expr),
		Data: &LIndexGetData{Collection: coll, Index: idx},
	})
}

func (l *Lowerer) lowerSliceExpr(expr *ast.Expr) LValue {
	se := dataAs[ast.SliceExpr](expr.Data)
	coll := l.lowerExpr(&se.Receiver)

	var low, high *LValue
	if se.Low != nil {
		v := l.lowerExpr(se.Low)
		low = &v
	}
	if se.High != nil {
		v := l.lowerExpr(se.High)
		high = &v
	}

	return l.emitTemp(LExpr{
		Kind: LExprSlice,
		Type: l.exprType(expr),
		Data: &LSliceData{Collection: coll, Low: low, High: high},
	})
}

func (l *Lowerer) lowerListLit(expr *ast.Expr) LValue {
	ll := dataAs[ast.ListLitExpr](expr.Data)
	var fields []LFieldInit
	for i, elem := range ll.Elems {
		val := l.lowerExpr(&elem)
		fields = append(fields, LFieldInit{Name: fmt.Sprintf("_%d", i), Value: val})
	}

	resultType := l.exprType(expr)
	return l.emitTemp(LExpr{
		Kind: LExprStructLit, // Reuse struct lit for list construction
		Type: resultType,
		Data: &LStructLitData{Fields: fields},
	})
}

func (l *Lowerer) lowerMapLit(expr *ast.Expr) LValue {
	ml := dataAs[ast.MapLitExpr](expr.Data)
	// Maps need a different approach — emit as make + index_set
	mapType := l.exprType(expr)
	mapVal := l.emitTemp(LExpr{
		Kind: LExprMakeMap,
		Type: mapType,
		Data: nil,
	})

	for _, entry := range ml.Entries {
		key := l.lowerExpr(&entry.Key)
		val := l.lowerExpr(&entry.Value)
		l.emit(LStmt{Kind: LStmtIndexSet, Data: &LIndexSet{
			Collection: mapVal,
			Index:      key,
			Value:      val,
		}})
	}

	return mapVal
}

func (l *Lowerer) lowerTupleLit(expr *ast.Expr) LValue {
	tl := dataAs[ast.TupleLitExpr](expr.Data)
	var fields []LFieldInit
	for i, elem := range tl.Elems {
		val := l.lowerExpr(&elem)
		fields = append(fields, LFieldInit{Name: fmt.Sprintf("_%d", i), Value: val})
	}

	return l.emitTemp(LExpr{
		Kind: LExprStructLit,
		Type: l.exprType(expr),
		Data: &LStructLitData{Fields: fields},
	})
}

func (l *Lowerer) lowerStructLit(expr *ast.Expr) LValue {
	sl := dataAs[ast.StructLitExpr](expr.Data)
	var fields []LFieldInit
	for _, f := range sl.Fields {
		val := l.lowerExpr(&f.Value)
		fields = append(fields, LFieldInit{Name: f.Name, Value: val})
	}

	resultType := l.exprType(expr)
	if resultType.Kind == LTyAny {
		typeName := sl.TypeName
		// Check if it's a qualified import (e.g., sync.Mutex)
		if alias, ok := l.importAliases[typeName]; ok {
			typeName = alias
		}
		resultType = &LType{Kind: LTyStruct, Name: typeName, IsExported: l.exported[sl.TypeName]}
	}

	// Fix qualified struct literals whose type wasn't resolved by the checker
	// (e.g., sync.Mutex{} resolves to error type because checker doesn't know external structs)
	if strings.Contains(sl.TypeName, ".") && resultType.Kind != LTyStruct && resultType.Kind != LTyClassHandle && resultType.Kind != LTyMutex {
		// Special case: sync.Mutex
		if sl.TypeName == "sync.Mutex" {
			resultType = &LType{Kind: LTyMutex}
		} else {
			resultType = &LType{Kind: LTyStruct, Name: sl.TypeName, IsExported: true}
		}
	}

	return l.emitTemp(LExpr{
		Kind: LExprStructLit,
		Type: resultType,
		Data: &LStructLitData{Fields: fields},
	})
}

func (l *Lowerer) lowerStringInterp(expr *ast.Expr) LValue {
	si := dataAs[ast.StringInterpExpr](expr.Data)
	var parts []LFormatPart
	for i, part := range si.Parts {
		if i%2 == 0 {
			// String literal part
			if part.Kind == ast.ExprStringLit {
				sl := dataAs[ast.StringLitExpr](part.Data)
				parts = append(parts, LFormatPart{IsLiteral: true, Text: sl.Value})
			}
		} else {
			// Expression part
			val := l.lowerExpr(&part)
			format := "%v"
			if val.Type != nil {
				switch val.Type.Kind {
				case LTyI8, LTyI16, LTyI32, LTyI64, LTyPlatformInt:
					format = "%d"
				case LTyU8, LTyU16, LTyU32, LTyU64, LTyPlatformUint:
					format = "%d"
				case LTyF32, LTyF64:
					format = "%f"
				case LTyString:
					format = "%s"
				case LTyBool:
					format = "%t"
				}
			}
			parts = append(parts, LFormatPart{IsLiteral: false, Value: val, Format: format})
		}
	}

	return l.emitTemp(LExpr{
		Kind: LExprFormat,
		Type: &LType{Kind: LTyString},
		Data: &LFormatData{Parts: parts},
	})
}

func (l *Lowerer) lowerCast(expr *ast.Expr) LValue {
	ce := dataAs[ast.CastExpr](expr.Data)
	operand := l.lowerExpr(&ce.Operand)
	targetType := l.lowerTypeExpr(&ce.TargetType)

	return l.emitTemp(LExpr{
		Kind: LExprCast,
		Type: targetType,
		Data: &LCastData{Target: targetType, Operand: operand},
	})
}

func (l *Lowerer) lowerUnwrap(expr *ast.Expr) LValue {
	ue := dataAs[ast.UnwrapExpr](expr.Data)
	operand := l.lowerExpr(&ue.Operand)

	resultType := l.exprType(expr)
	return l.emitTemp(LExpr{
		Kind: LExprUnwrapOptional,
		Type: resultType,
		Data: &LUnwrapOptionalData{Value: operand},
	})
}

func (l *Lowerer) lowerTry(expr *ast.Expr) LValue {
	te := dataAs[ast.TryExpr](expr.Data)
	operand := l.lowerExpr(&te.Operand)

	// Extract value and error
	errTemp := l.emitTemp(LExpr{
		Kind: LExprExtractError,
		Type: &LType{Kind: LTyError},
		Data: &LExtractErrorData{Value: operand},
	})

	// Check error and return early
	nullVal := LValue{Kind: LValLitNull, Type: &LType{Kind: LTyAny}}
	isNotNull := l.emitTemp(LExpr{
		Kind: LExprBinOp,
		Type: &LType{Kind: LTyBool},
		Data: &LBinOpData{Op: LBinNe, Left: errTemp, Right: nullVal},
	})

	// Build early return with zero value + error
	var returnValues []LValue
	if l.currentReturnType != nil && l.currentReturnType.Kind == LTyErrorResult {
		returnValues = []LValue{
			{Kind: LValLitNull, Type: l.currentReturnType.Elem},
			errTemp,
		}
	} else {
		returnValues = []LValue{errTemp}
	}

	l.emit(LStmt{Kind: LStmtIf, Data: &LIf{
		Cond: isNotNull,
		Then: []LStmt{{Kind: LStmtReturn, Data: &LReturn{Values: returnValues}}},
	}})

	// Extract the value
	resultType := l.exprType(expr)
	return l.emitTemp(LExpr{
		Kind: LExprExtractValue,
		Type: resultType,
		Data: &LExtractValueData{Value: operand},
	})
}

// Ensure strings import is used (for strings.Contains etc. in method lowering)
var _ = strings.Contains
