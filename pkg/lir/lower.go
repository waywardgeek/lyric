package lir

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/waywardgeek/grok/pkg/ast"
	"github.com/waywardgeek/grok/pkg/checker"
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
		exported:   make(map[string]bool),
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
			prog.TypeDefs = append(prog.TypeDefs, LTypeDef{
				Name:       ta.Name,
				Type:       l.lowerTypeExpr(&ta.Type),
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
		for _, cls := range block.Classes {
			prog.Classes = append(prog.Classes, l.lowerClassDecl(&cls))
		}

		// Lower interfaces
		for _, iface := range block.Interfaces {
			prog.Interfaces = append(prog.Interfaces, l.lowerInterfaceDecl(&iface))
		}

		// Lower functions (including class methods)
		for _, fn := range block.Functions {
			prog.Functions = append(prog.Functions, l.lowerFuncDecl(&fn, ""))
		}
		for _, cls := range block.Classes {
			for _, m := range cls.Methods {
				prog.Functions = append(prog.Functions, l.lowerFuncDecl(&m, cls.Name))
			}
		}
	}

	return prog
}

// registerTypes populates variant/class lookup tables before lowering.
func (l *Lowerer) registerTypes(block *ast.GrokBlock) {
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

	for _, cls := range block.Classes {
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
	case ast.TypeLock:
		return &LType{Kind: LTyMutex}
	case ast.TypeUnit:
		return &LType{Kind: LTyUnit}
	case ast.TypeUnion:
		// Union types lower to any — backends handle via type switch
		return &LType{Kind: LTyAny}
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
	case checker.TyUnion:
		return &LType{Kind: LTyAny}
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
	var fields []LField
	for _, f := range s.Fields {
		fields = append(fields, LField{Name: f.Name, Type: l.lowerTypeExpr(&f.Type)})
	}
	return LStructDecl{Name: s.Name, Fields: fields, IsExported: s.IsPublic}
}

func (l *Lowerer) lowerEnumDecl(e *ast.EnumDecl) LEnumDecl {
	variants := l.enumVariants[e.Name]
	return LEnumDecl{Name: e.Name, Variants: variants, IsExported: e.IsPublic}
}

func (l *Lowerer) lowerInterfaceDecl(iface *ast.InterfaceDecl) LInterfaceDecl {
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
			Name:       m.Name,
			Params:     params,
			ReturnType: retType,
		})
	}
	return LInterfaceDecl{
		Name:       iface.Name,
		Methods:    methods,
		Embeds:     iface.Implements,
		IsExported: iface.IsPublic,
	}
}

func (l *Lowerer) lowerClassDecl(cls *ast.ClassDecl) LClassDecl {
	fields := l.classFields[cls.Name]
	guardedBy := make(map[string]string)
	for _, f := range cls.Fields {
		if f.GuardedBy != "" {
			guardedBy[f.Name] = f.GuardedBy
		}
	}
	return LClassDecl{
		Name:       cls.Name,
		Fields:     fields,
		GuardedBy:  guardedBy,
		IsExported: cls.IsPublic,
	}
}

func (l *Lowerer) lowerFuncDecl(fn *ast.FuncDecl, receiver string) LFuncDecl {
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

	return LFuncDecl{
		Name:       fn.Name,
		Params:     params,
		ReturnType: retType,
		Body:       body,
		IsExported: fn.IsPublic,
		Receiver:   receiver,
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
		// Emit a tag extraction + switch
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
	if matchVal.Type == nil || matchVal.Type.Kind != LTyAny {
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
				caseType = l.grokNameToLType(ip.Name)
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

// grokNameToLType converts a Grok type name to an LType (fallback for union patterns).
func (l *Lowerer) grokNameToLType(name string) *LType {
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
		return LValue{Kind: LValLitNull, Type: &LType{Kind: LTyAny}}
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
	// Cast the operand that doesn't match the result type
	target := resultType
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
	case "println", "print", "len", "append", "isnull":
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
				Data: &LCallData{Func: funcName, Args: args, IsExported: true},
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
			Data: &LClassAllocData{Class: funcName, Fields: fieldInits},
		})
	}

	// Regular function call
	return l.emitTemp(LExpr{
		Kind: LExprCall,
		Type: l.exprType(expr),
		Data: &LCallData{Func: funcName, Args: args, IsExported: l.exported[funcName]},
	})
}

func (l *Lowerer) lowerMethodCall(expr *ast.Expr) LValue {
	mc := dataAs[ast.MethodCallExpr](expr.Data)

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
				Data: &LCallData{Func: funcName, Args: args, IsExported: true},
			})
		}
	}

	recv := l.lowerExpr(&mc.Receiver)

	var args []LValue
	for _, arg := range mc.Args {
		args = append(args, l.lowerExpr(&arg))
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
			IsExported: l.exported[mc.Method],
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
