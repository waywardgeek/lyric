// Package checker implements type checking and inference for Grok.
package checker

import (
	"fmt"
	"strings"

	"github.com/waywardgeek/grok/pkg/ast"
)

// TypeKind discriminates type categories.
type TypeKind int

const (
	TyInt     TypeKind = iota // i8, i16, i32, i64, i128, i256
	TyUint                    // u8, u16, u32, u64, u128, u256
	TyFloat                   // f32, f64, f128
	TyBool                    // bool
	TyString                  // string
	TyUnit                    // unit (void)
	TyList                    // [T]
	TyMap                     // map[K]V
	TyTuple                   // (T, U)
	TyFunc                    // T -> U
	TyOptional                // T?
	TyChannel                 // channel<T>
	TyStruct                  // named struct type
	TyClass                   // named class type
	TyEnum                    // named enum type
	TyInterface               // named interface type
	TyVar                     // type variable (for generics)
	TyUnknown                 // not yet resolved
	TyError                   // error sentinel
)

// Type represents a resolved type.
type Type struct {
	Kind TypeKind
	Name string  // for named types (struct, class, enum, type vars)
	Bits int     // for int/uint/float (8, 16, 32, 64, 128, 256)
	Elem *Type   // for list, optional, channel
	Key  *Type   // for map
	Val  *Type   // for map
	Fields []TypeField // for tuple
	Params []*Type     // for func: param types
	Return *Type       // for func: return type
}

// TypeField is a named or positional field in a tuple/struct.
type TypeField struct {
	Name string
	Type *Type
}

func (t *Type) String() string {
	if t == nil {
		return "<nil>"
	}
	switch t.Kind {
	case TyInt:
		return fmt.Sprintf("i%d", t.Bits)
	case TyUint:
		return fmt.Sprintf("u%d", t.Bits)
	case TyFloat:
		return fmt.Sprintf("f%d", t.Bits)
	case TyBool:
		return "bool"
	case TyString:
		return "string"
	case TyUnit:
		return "unit"
	case TyList:
		return fmt.Sprintf("[%s]", t.Elem)
	case TyMap:
		return fmt.Sprintf("map[%s]%s", t.Key, t.Val)
	case TyOptional:
		return fmt.Sprintf("%s?", t.Elem)
	case TyChannel:
		return fmt.Sprintf("channel<%s>", t.Elem)
	case TyStruct, TyClass, TyEnum, TyInterface:
		return t.Name
	case TyVar:
		return t.Name
	case TyFunc:
		s := "("
		for i, p := range t.Params {
			if i > 0 {
				s += ", "
			}
			s += p.String()
		}
		s += ") -> " + t.Return.String()
		return s
	case TyUnknown:
		return "?"
	case TyError:
		return "<error>"
	default:
		return "<unknown>"
	}
}

// Convenience constructors
var (
	TypeBool    = &Type{Kind: TyBool}
	TypeString  = &Type{Kind: TyString}
	TypeUnit    = &Type{Kind: TyUnit}
	TypeUnknown = &Type{Kind: TyUnknown}
	TypeError   = &Type{Kind: TyError}
	TypeI32     = &Type{Kind: TyInt, Bits: 32}
	TypeI64     = &Type{Kind: TyInt, Bits: 64}
	TypeF64     = &Type{Kind: TyFloat, Bits: 64}
)

func IntType(bits int) *Type   { return &Type{Kind: TyInt, Bits: bits} }
func UintType(bits int) *Type  { return &Type{Kind: TyUint, Bits: bits} }
func FloatType(bits int) *Type { return &Type{Kind: TyFloat, Bits: bits} }
func ListType(elem *Type) *Type { return &Type{Kind: TyList, Elem: elem} }
func MapType(key, val *Type) *Type { return &Type{Kind: TyMap, Key: key, Val: val} }
func OptionalType(elem *Type) *Type { return &Type{Kind: TyOptional, Elem: elem} }

// Equal checks structural type equality.
func (t *Type) Equal(other *Type) bool {
	if t == other {
		return true
	}
	if t == nil || other == nil {
		return false
	}
	if t.Kind != other.Kind {
		return false
	}
	switch t.Kind {
	case TyInt, TyUint, TyFloat:
		return t.Bits == other.Bits
	case TyBool, TyString, TyUnit:
		return true
	case TyStruct, TyClass, TyEnum, TyInterface, TyVar:
		return t.Name == other.Name
	case TyList, TyOptional, TyChannel:
		return t.Elem.Equal(other.Elem)
	case TyMap:
		return t.Key.Equal(other.Key) && t.Val.Equal(other.Val)
	case TyFunc:
		if len(t.Params) != len(other.Params) {
			return false
		}
		for i := range t.Params {
			if !t.Params[i].Equal(other.Params[i]) {
				return false
			}
		}
		return t.Return.Equal(other.Return)
	default:
		return false
	}
}

// numericWidens checks if 'from' can implicitly widen to 'to'.
// Same-sign integers widen to larger bit widths. Floats widen to larger bit widths.
// No cross-kind coercion (int→float requires explicit cast).
func numericWidens(from, to *Type) bool {
	if from.Kind == to.Kind {
		switch from.Kind {
		case TyInt, TyUint, TyFloat:
			return from.Bits < to.Bits
		}
	}
	return false
}

// coerceNumeric returns the wider type if one side can widen to the other, else nil.
func coerceNumeric(left, right *Type) *Type {
	if numericWidens(left, right) {
		return right
	}
	if numericWidens(right, left) {
		return left
	}
	return nil
}

// IsNumeric returns true for int, uint, and float types.
func (t *Type) IsNumeric() bool {
	return t.Kind == TyInt || t.Kind == TyUint || t.Kind == TyFloat
}

// IsInteger returns true for int and uint types.
func (t *Type) IsInteger() bool {
	return t.Kind == TyInt || t.Kind == TyUint
}

// --- Scope / Environment ---

// Scope maps variable names to types.
type Scope struct {
	parent   *Scope
	vars     map[string]*Type
	mutables map[string]bool // tracks which variables are mutable (let mut)
}

func NewScope(parent *Scope) *Scope {
	return &Scope{parent: parent, vars: make(map[string]*Type), mutables: make(map[string]bool)}
}

func (s *Scope) Define(name string, typ *Type) {
	s.vars[name] = typ
}

func (s *Scope) DefineMut(name string, typ *Type) {
	s.vars[name] = typ
	s.mutables[name] = true
}

func (s *Scope) IsMutable(name string) bool {
	if _, ok := s.mutables[name]; ok {
		return true
	}
	if s.parent != nil {
		return s.parent.IsMutable(name)
	}
	return false
}

func (s *Scope) Lookup(name string) *Type {
	if t, ok := s.vars[name]; ok {
		return t
	}
	if s.parent != nil {
		return s.parent.Lookup(name)
	}
	return nil
}

// --- Type Registry ---

// TypeInfo holds registered type information.
type TypeInfo struct {
	Type    *Type
	Fields  map[string]*Type // struct/class fields
	Methods map[string]*Type // method signatures (as TyFunc)
}

// Registry holds all known types in the program.
type Registry struct {
	types map[string]*TypeInfo
}

func NewRegistry() *Registry {
	return &Registry{types: make(map[string]*TypeInfo)}
}

func (r *Registry) Register(name string, info *TypeInfo) {
	r.types[name] = info
}

func (r *Registry) Lookup(name string) *TypeInfo {
	return r.types[name]
}

// --- Checker ---

// CheckError is a type checking error.
type CheckError struct {
	Message string
	Span    ast.Span
}

func (e *CheckError) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", e.Span.Start.File, e.Span.Start.Line, e.Span.Start.Column, e.Message)
}

// Checker performs type checking on a Grok AST.
type Checker struct {
	registry      *Registry
	errors        []error
	scope         *Scope
	currentReturn *Type // expected return type of current function (nil = void/unit)
	loopDepth     int   // tracks nesting depth inside loops (for break/continue validation)
}

// New creates a new type checker.
func New() *Checker {
	c := &Checker{
		registry: NewRegistry(),
		scope:    NewScope(nil),
	}
	// Register builtin functions
	// println(...) — variadic, accepts any types, returns unit
	c.scope.Define("println", &Type{Kind: TyFunc, Params: nil, Return: TypeUnit, Name: "println"})
	c.scope.Define("print", &Type{Kind: TyFunc, Params: nil, Return: TypeUnit, Name: "print"})
	return c
}

// Errors returns all accumulated type errors.
func (c *Checker) Errors() []error {
	return c.errors
}

func (c *Checker) error(span ast.Span, msg string, args ...any) {
	c.errors = append(c.errors, &CheckError{
		Message: fmt.Sprintf(msg, args...),
		Span:    span,
	})
}

func (c *Checker) pushScope() {
	c.scope = NewScope(c.scope)
}

func (c *Checker) popScope() {
	c.scope = c.scope.parent
}

// resolveTypeExpr converts an AST TypeExpr to a checker Type.
func (c *Checker) resolveTypeExpr(te *ast.TypeExpr) *Type {
	if te == nil {
		return TypeUnknown
	}
	switch te.Kind {
	case ast.TypeNamed:
		nt := te.Data.(ast.NamedType)
		return c.resolveNamedType(nt.Name, nt.Args)
	case ast.TypeOptional:
		ot := te.Data.(ast.OptionalType)
		inner := c.resolveTypeExpr(&ot.Inner)
		return OptionalType(inner)
	case ast.TypeSequence:
		st := te.Data.(ast.SequenceType)
		elem := c.resolveTypeExpr(&st.Elem)
		return ListType(elem)
	case ast.TypeMap:
		mt := te.Data.(ast.MapType)
		key := c.resolveTypeExpr(&mt.Key)
		val := c.resolveTypeExpr(&mt.Value)
		return MapType(key, val)
	case ast.TypeUnit:
		return TypeUnit
	case ast.TypeFunc:
		ft := te.Data.(ast.FuncType)
		var params []*Type
		for i := range ft.Params {
			params = append(params, c.resolveTypeExpr(&ft.Params[i]))
		}
		ret := c.resolveTypeExpr(&ft.Return)
		return &Type{Kind: TyFunc, Params: params, Return: ret}
	default:
		return TypeUnknown
	}
}

func (c *Checker) resolveNamedType(name string, args []ast.TypeExpr) *Type {
	switch name {
	case "i8":
		return IntType(8)
	case "i16":
		return IntType(16)
	case "i32":
		return IntType(32)
	case "i64":
		return IntType(64)
	case "i128":
		return IntType(128)
	case "i256":
		return IntType(256)
	case "u8":
		return UintType(8)
	case "u16":
		return UintType(16)
	case "u32":
		return UintType(32)
	case "u64":
		return UintType(64)
	case "u128":
		return UintType(128)
	case "u256":
		return UintType(256)
	case "f32":
		return FloatType(32)
	case "f64":
		return FloatType(64)
	case "f128":
		return FloatType(128)
	case "bool":
		return TypeBool
	case "string":
		return TypeString
	case "int":
		return IntType(64) // default int
	case "uint":
		return UintType(64)
	case "float":
		return FloatType(64)
	default:
		// Check registry for user-defined types
		if info := c.registry.Lookup(name); info != nil {
			return info.Type
		}
		// Could be a type variable
		return &Type{Kind: TyVar, Name: name}
	}
}

// assignableTo checks if 'from' type can be used where 'to' type is expected.
// Extends Equal() with interface subtyping: a class/struct with all required
// methods satisfies an interface (structural subtyping).
func (c *Checker) assignableTo(from, to *Type) bool {
	if from.Equal(to) {
		return true
	}
	// Interface subtyping: class/struct → interface
	if to.Kind == TyInterface {
		ifaceInfo := c.registry.Lookup(to.Name)
		if ifaceInfo == nil {
			return false
		}
		var fromInfo *TypeInfo
		if from.Kind == TyClass || from.Kind == TyStruct {
			fromInfo = c.registry.Lookup(from.Name)
		}
		if fromInfo == nil {
			return false
		}
		for methodName, ifaceMethod := range ifaceInfo.Methods {
			classMethod, ok := fromInfo.Methods[methodName]
			if !ok {
				return false
			}
			if !classMethod.Equal(ifaceMethod) {
				return false
			}
		}
		return true
	}
	return false
}

// --- Expression type inference ---

func (c *Checker) checkExpr(expr *ast.Expr) *Type {
	if expr == nil {
		return TypeUnknown
	}
	t := c.inferExpr(expr)
	expr.ResolvedType = t
	return t
}

func (c *Checker) inferExpr(expr *ast.Expr) *Type {
	switch expr.Kind {
	case ast.ExprIntLit:
		return TypeI32 // default integer literal type
	case ast.ExprFloatLit:
		return TypeF64
	case ast.ExprStringLit:
		return TypeString
	case ast.ExprStringInterp:
		interp := expr.Data.(*ast.StringInterpExpr)
		for i, part := range interp.Parts {
			if i%2 != 0 {
				c.checkExpr(&part)
			}
		}
		return TypeString
	case ast.ExprBoolLit:
		return TypeBool
	case ast.ExprNil:
		return TypeUnknown // needs context to resolve
	case ast.ExprIdent:
		id := expr.Data.(*ast.IdentExpr)
		t := c.scope.Lookup(id.Name)
		if t == nil {
			c.error(expr.Span, "undefined variable %q", id.Name)
			return TypeError
		}
		return t
	case ast.ExprUnary:
		return c.checkUnary(expr)
	case ast.ExprBinary:
		return c.checkBinary(expr)
	case ast.ExprCall:
		return c.checkCall(expr)
	case ast.ExprMethodCall:
		return c.checkMethodCall(expr)
	case ast.ExprFieldAccess:
		return c.checkFieldAccess(expr)
	case ast.ExprIndex:
		return c.checkIndex(expr)
	case ast.ExprListLit:
		return c.checkListLit(expr)
	case ast.ExprTupleLit:
		return c.checkTupleLit(expr)
	case ast.ExprMapLit:
		return c.checkMapLit(expr)
	case ast.ExprStructLit:
		return c.checkStructLit(expr)
	case ast.ExprMatch:
		return c.checkMatchExpr(expr)
	default:
		return TypeUnknown
	}
}

func (c *Checker) checkUnary(expr *ast.Expr) *Type {
	u := expr.Data.(*ast.UnaryExpr)
	operand := c.checkExpr(&u.Operand)
	switch u.Op {
	case ast.OpNeg:
		if !operand.IsNumeric() {
			c.error(expr.Span, "cannot negate non-numeric type %s", operand)
			return TypeError
		}
		return operand
	case ast.OpNot:
		if !operand.Equal(TypeBool) {
			c.error(expr.Span, "cannot apply ! to non-bool type %s", operand)
			return TypeError
		}
		return TypeBool
	}
	return TypeUnknown
}

func (c *Checker) checkBinary(expr *ast.Expr) *Type {
	b := expr.Data.(*ast.BinaryExpr)
	left := c.checkExpr(&b.Left)
	right := c.checkExpr(&b.Right)

	switch b.Op {
	case ast.OpAdd, ast.OpSub, ast.OpMul, ast.OpDiv, ast.OpMod:
		if left.IsNumeric() && right.IsNumeric() {
			if left.Equal(right) {
				return left
			}
			if wider := coerceNumeric(left, right); wider != nil {
				return wider
			}
			c.error(expr.Span, "mismatched numeric types: %s and %s", left, right)
			return TypeError
		}
		// String concatenation
		if b.Op == ast.OpAdd && left.Equal(TypeString) && right.Equal(TypeString) {
			return TypeString
		}
		c.error(expr.Span, "cannot apply %v to %s and %s", b.Op, left, right)
		return TypeError

	case ast.OpEq, ast.OpNeq:
		// Any two equal types can be compared for equality
		if !left.Equal(right) && left.Kind != TyUnknown && right.Kind != TyUnknown {
			c.error(expr.Span, "cannot compare %s and %s for equality", left, right)
		}
		return TypeBool

	case ast.OpLt, ast.OpLe, ast.OpGt, ast.OpGe:
		if left.IsNumeric() && right.IsNumeric() {
			if left.Equal(right) || coerceNumeric(left, right) != nil {
				return TypeBool
			}
		}
		if left.Equal(TypeString) && right.Equal(TypeString) {
			return TypeBool
		}
		c.error(expr.Span, "cannot compare %s and %s", left, right)
		return TypeBool

	case ast.OpAnd, ast.OpOr:
		if !left.Equal(TypeBool) || !right.Equal(TypeBool) {
			c.error(expr.Span, "logical operators require bool operands, got %s and %s", left, right)
		}
		return TypeBool

	case ast.OpBitAnd, ast.OpBitOr, ast.OpBitXor, ast.OpShl, ast.OpShr:
		if left.IsInteger() && right.IsInteger() && left.Equal(right) {
			return left
		}
		c.error(expr.Span, "bitwise operators require matching integer types, got %s and %s", left, right)
		return TypeError
	}
	return TypeUnknown
}

func (c *Checker) checkCall(expr *ast.Expr) *Type {
	call := expr.Data.(*ast.CallExpr)
	fnType := c.checkExpr(&call.Func)
	if fnType.Kind == TyError || fnType.Kind == TyUnknown {
		return TypeUnknown
	}
	if fnType.Kind != TyFunc {
		c.error(expr.Span, "cannot call non-function type %s", fnType)
		return TypeError
	}
	if fnType.Params == nil {
		// Variadic/builtin — just check each arg is valid
		for i := range call.Args {
			c.checkExpr(&call.Args[i])
		}
	} else if len(call.Args) != len(fnType.Params) {
		c.error(expr.Span, "expected %d arguments, got %d", len(fnType.Params), len(call.Args))
	} else {
		for i := range call.Args {
			argType := c.checkExpr(&call.Args[i])
			if !c.assignableTo(argType, fnType.Params[i]) && argType.Kind != TyUnknown && fnType.Params[i].Kind != TyVar {
				if !numericWidens(argType, fnType.Params[i]) {
					c.error(call.Args[i].Span, "argument %d: expected %s, got %s", i+1, fnType.Params[i], argType)
				}
			}
		}
	}
	return fnType.Return
}

func (c *Checker) checkMethodCall(expr *ast.Expr) *Type {
	mc := expr.Data.(*ast.MethodCallExpr)
	recvType := c.checkExpr(&mc.Receiver)
	// Look up method on the receiver type
	if recvType.Kind == TyStruct || recvType.Kind == TyClass {
		if info := c.registry.Lookup(recvType.Name); info != nil {
			if methType, ok := info.Methods[mc.Method]; ok && methType.Kind == TyFunc {
				return methType.Return
			}
		}
	}
	// Check args but return unknown
	for i := range mc.Args {
		c.checkExpr(&mc.Args[i])
	}
	return TypeUnknown
}

func (c *Checker) checkFieldAccess(expr *ast.Expr) *Type {
	fa := expr.Data.(*ast.FieldAccessExpr)
	recvType := c.checkExpr(&fa.Receiver)
	if recvType.Kind == TyStruct || recvType.Kind == TyClass {
		if info := c.registry.Lookup(recvType.Name); info != nil {
			if fieldType, ok := info.Fields[fa.Field]; ok {
				return fieldType
			}
			c.error(expr.Span, "type %s has no field %q", recvType.Name, fa.Field)
		}
	}
	return TypeUnknown
}

func (c *Checker) checkIndex(expr *ast.Expr) *Type {
	idx := expr.Data.(*ast.IndexExpr)
	recvType := c.checkExpr(&idx.Receiver)
	indexType := c.checkExpr(&idx.Index)

	switch recvType.Kind {
	case TyList:
		if !indexType.IsInteger() {
			c.error(expr.Span, "list index must be integer, got %s", indexType)
		}
		return recvType.Elem
	case TyMap:
		if !indexType.Equal(recvType.Key) && indexType.Kind != TyUnknown {
			c.error(expr.Span, "map key type mismatch: expected %s, got %s", recvType.Key, indexType)
		}
		return recvType.Val
	case TyString:
		if !indexType.IsInteger() {
			c.error(expr.Span, "string index must be integer, got %s", indexType)
		}
		return TypeString // single char as string
	}
	return TypeUnknown
}

func (c *Checker) checkListLit(expr *ast.Expr) *Type {
	lit := expr.Data.(*ast.ListLitExpr)
	if len(lit.Elems) == 0 {
		return ListType(TypeUnknown)
	}
	elemType := c.checkExpr(&lit.Elems[0])
	for i := 1; i < len(lit.Elems); i++ {
		t := c.checkExpr(&lit.Elems[i])
		if !t.Equal(elemType) && t.Kind != TyUnknown && elemType.Kind != TyUnknown {
			c.error(lit.Elems[i].Span, "list element type mismatch: expected %s, got %s", elemType, t)
		}
	}
	return ListType(elemType)
}

func (c *Checker) checkTupleLit(expr *ast.Expr) *Type {
	lit := expr.Data.(*ast.TupleLitExpr)
	var fields []TypeField
	for i := range lit.Elems {
		t := c.checkExpr(&lit.Elems[i])
		fields = append(fields, TypeField{Type: t})
	}
	return &Type{Kind: TyTuple, Fields: fields}
}

func (c *Checker) checkMapLit(expr *ast.Expr) *Type {
	lit := expr.Data.(*ast.MapLitExpr)
	if len(lit.Entries) == 0 {
		return MapType(TypeUnknown, TypeUnknown)
	}
	keyType := c.checkExpr(&lit.Entries[0].Key)
	valType := c.checkExpr(&lit.Entries[0].Value)
	for i := 1; i < len(lit.Entries); i++ {
		k := c.checkExpr(&lit.Entries[i].Key)
		v := c.checkExpr(&lit.Entries[i].Value)
		if !k.Equal(keyType) && k.Kind != TyUnknown {
			c.error(lit.Entries[i].Key.Span, "map key type mismatch: expected %s, got %s", keyType, k)
		}
		if !v.Equal(valType) && v.Kind != TyUnknown {
			c.error(lit.Entries[i].Value.Span, "map value type mismatch: expected %s, got %s", valType, v)
		}
	}
	return MapType(keyType, valType)
}

func (c *Checker) checkMatchExpr(expr *ast.Expr) *Type {
	m := expr.Data.(*ast.MatchStmt)
	c.checkExpr(&m.Value)
	// Infer type from last expression of first arm
	var resultType *Type
	for _, arm := range m.Arms {
		c.pushScope()
		c.bindPattern(&arm.Pattern, c.inferExpr(&m.Value))
		for i := range arm.Body.Stmts {
			c.checkStmt(&arm.Body.Stmts[i])
		}
		// Use last stmt if it's an expr stmt
		if len(arm.Body.Stmts) > 0 {
			last := &arm.Body.Stmts[len(arm.Body.Stmts)-1]
			if last.Kind == ast.StmtExpr {
				es := last.Data.(*ast.ExprStmt)
				t := c.inferExpr(&es.Expr)
				if resultType == nil {
					resultType = t
				}
			}
		}
		c.popScope()
	}
	if resultType == nil {
		return TypeUnknown
	}
	return resultType
}

// --- Statement checking ---

func (c *Checker) checkStmt(stmt *ast.Stmt) {
	switch stmt.Kind {
	case ast.StmtVarDecl:
		c.checkVarDecl(stmt)
	case ast.StmtAssign:
		c.checkAssign(stmt)
	case ast.StmtReturn:
		c.checkReturn(stmt)
	case ast.StmtExpr:
		es := stmt.Data.(*ast.ExprStmt)
		c.checkExpr(&es.Expr)
	case ast.StmtIf:
		c.checkIf(stmt)
	case ast.StmtFor:
		c.checkFor(stmt)
	case ast.StmtWhile:
		c.checkWhile(stmt)
	case ast.StmtMatch:
		c.checkMatch(stmt)
	case ast.StmtBlock:
		blk := stmt.Data.(*ast.Block)
		c.checkBlock(blk)
	case ast.StmtCascade:
		cs := stmt.Data.(*ast.CascadeStmt)
		c.checkBlock(&cs.Body)
	case ast.StmtBreak:
		if c.loopDepth == 0 {
			c.error(stmt.Span, "break outside of loop")
		}
	case ast.StmtContinue:
		if c.loopDepth == 0 {
			c.error(stmt.Span, "continue outside of loop")
		}
	}
}

func (c *Checker) checkVarDecl(stmt *ast.Stmt) {
	decl := stmt.Data.(*ast.VarDeclStmt)
	var declaredType *Type
	if decl.Type != nil {
		declaredType = c.resolveTypeExpr(decl.Type)
	}
	var inferredType *Type
	if decl.Value != nil {
		inferredType = c.checkExpr(decl.Value)
	}

	var finalType *Type
	if declaredType != nil && inferredType != nil {
		// Both present: check compatibility (allow numeric widening)
		if !inferredType.Equal(declaredType) && inferredType.Kind != TyUnknown {
			if !numericWidens(inferredType, declaredType) {
				c.error(stmt.Span, "type mismatch in variable %q: declared %s, got %s", decl.Name, declaredType, inferredType)
			}
		}
		finalType = declaredType
	} else if declaredType != nil {
		finalType = declaredType
	} else if inferredType != nil {
		finalType = inferredType // type inference!
	} else {
		c.error(stmt.Span, "variable %q has no type annotation and no initializer", decl.Name)
		finalType = TypeError
	}

	if decl.IsMut {
		c.scope.DefineMut(decl.Name, finalType)
	} else {
		c.scope.Define(decl.Name, finalType)
	}
}

func (c *Checker) checkAssign(stmt *ast.Stmt) {
	assign := stmt.Data.(*ast.AssignStmt)
	targetType := c.checkExpr(&assign.Target)
	valueType := c.checkExpr(&assign.Value)
	// Check mutability — only for simple identifier targets
	if assign.Target.Kind == ast.ExprIdent {
		id := assign.Target.Data.(*ast.IdentExpr)
		if !c.scope.IsMutable(id.Name) {
			c.error(stmt.Span, "cannot assign to immutable variable %q (use 'let mut')", id.Name)
		}
	}
	if !targetType.Equal(valueType) && targetType.Kind != TyUnknown && valueType.Kind != TyUnknown {
		if !numericWidens(valueType, targetType) {
			c.error(stmt.Span, "cannot assign %s to %s", valueType, targetType)
		}
	}
}

func (c *Checker) checkReturn(stmt *ast.Stmt) {
	ret := stmt.Data.(*ast.ReturnStmt)
	if ret.Value != nil {
		valType := c.checkExpr(ret.Value)
		if c.currentReturn != nil {
			if !c.currentReturn.Equal(valType) && valType.Kind != TyUnknown && c.currentReturn.Kind != TyUnknown {
				if !numericWidens(valType, c.currentReturn) {
					c.error(stmt.Span, "return type mismatch: expected %s, got %s", c.currentReturn, valType)
				}
			}
		}
	} else {
		if c.currentReturn != nil {
			c.error(stmt.Span, "missing return value, expected %s", c.currentReturn)
		}
	}
}

func (c *Checker) checkIf(stmt *ast.Stmt) {
	ifStmt := stmt.Data.(*ast.IfStmt)
	condType := c.checkExpr(&ifStmt.Condition)
	if !condType.Equal(TypeBool) && condType.Kind != TyUnknown && condType.Kind != TyError {
		c.error(stmt.Span, "if condition must be bool, got %s", condType)
	}
	c.checkBlock(&ifStmt.Then)
	for i := range ifStmt.ElseIfs {
		elifCond := c.checkExpr(&ifStmt.ElseIfs[i].Condition)
		if !elifCond.Equal(TypeBool) && elifCond.Kind != TyUnknown && elifCond.Kind != TyError {
			c.error(ifStmt.ElseIfs[i].Span, "else-if condition must be bool, got %s", elifCond)
		}
		c.checkBlock(&ifStmt.ElseIfs[i].Body)
	}
	if ifStmt.Else != nil {
		c.checkBlock(ifStmt.Else)
	}
}

func (c *Checker) checkFor(stmt *ast.Stmt) {
	forStmt := stmt.Data.(*ast.ForStmt)
	collType := c.checkExpr(&forStmt.Collection)
	c.pushScope()
	defer c.popScope()

	// Infer loop variable type from collection
	var elemType *Type
	switch collType.Kind {
	case TyList:
		elemType = collType.Elem
	case TyString:
		elemType = TypeString // iterate characters as strings
	case TyMap:
		elemType = collType.Key // iterate keys
	default:
		if collType.Kind != TyUnknown && collType.Kind != TyError {
			c.error(stmt.Span, "cannot iterate over %s", collType)
		}
		elemType = TypeUnknown
	}
	c.scope.Define(forStmt.Var, elemType)
	c.loopDepth++
	c.checkBlock(&forStmt.Body)
	c.loopDepth--
}

func (c *Checker) checkWhile(stmt *ast.Stmt) {
	whileStmt := stmt.Data.(*ast.WhileStmt)
	condType := c.checkExpr(&whileStmt.Condition)
	if !condType.Equal(TypeBool) && condType.Kind != TyUnknown && condType.Kind != TyError {
		c.error(stmt.Span, "while condition must be bool, got %s", condType)
	}
	c.loopDepth++
	c.checkBlock(&whileStmt.Body)
	c.loopDepth--
}

func (c *Checker) checkMatch(stmt *ast.Stmt) {
	matchStmt := stmt.Data.(*ast.MatchStmt)
	matchType := c.checkExpr(&matchStmt.Value)
	for i := range matchStmt.Arms {
		c.pushScope()
		c.bindPattern(&matchStmt.Arms[i].Pattern, matchType)
		c.checkBlock(&matchStmt.Arms[i].Body)
		c.popScope()
	}
}

// bindPattern introduces variables from a pattern into the current scope.
func (c *Checker) bindPattern(pat *ast.Pattern, matchType *Type) {
	switch pat.Kind {
	case ast.PatIdent:
		id := pat.Data.(*ast.IdentPattern)
		if id.Name != "_" {
			c.scope.Define(id.Name, matchType)
		}
	case ast.PatVariant:
		vp := pat.Data.(*ast.VariantPattern)
		// Bind sub-patterns as unknown (enum variant field types not tracked yet)
		for i := range vp.Bindings {
			c.bindPattern(&vp.Bindings[i], TypeUnknown)
		}
	case ast.PatTuple:
		tp := pat.Data.(*ast.TuplePattern)
		for i := range tp.Elems {
			var elemType *Type
			if matchType.Kind == TyTuple && i < len(matchType.Fields) {
				elemType = matchType.Fields[i].Type
			} else {
				elemType = TypeUnknown
			}
			c.bindPattern(&tp.Elems[i], elemType)
		}
	case ast.PatLiteral:
		// Literals don't bind variables
	case ast.PatWildcard:
		// Wildcards don't bind variables
	}
}

func (c *Checker) checkBlock(block *ast.Block) {
	c.pushScope()
	defer c.popScope()
	for i := range block.Stmts {
		c.checkStmt(&block.Stmts[i])
	}
}

// --- Top-level checking ---

// CheckFile type-checks an entire AST file.
func (c *Checker) CheckFile(file *ast.File) {
	for i := range file.Blocks {
		c.checkGrokBlock(&file.Blocks[i])
	}
}

func (c *Checker) checkGrokBlock(block *ast.GrokBlock) {
	// Register interfaces first (classes may implement them)
	for i := range block.Interfaces {
		c.registerInterface(&block.Interfaces[i])
	}
	// Register all types (forward declarations)
	for i := range block.Structs {
		c.registerStruct(&block.Structs[i])
	}
	for i := range block.Classes {
		c.registerClass(&block.Classes[i])
	}
	for i := range block.Enums {
		c.registerEnum(&block.Enums[i])
	}

	// Check implements satisfaction
	for i := range block.Classes {
		c.checkImplements(&block.Classes[i])
	}

	// Register import aliases in scope (packages are opaque types)
	for _, imp := range block.Imports {
		c.scope.Define(imp.Alias, &Type{Kind: TyUnknown, Name: imp.Path})
	}

	// Register functions in scope
	for i := range block.Functions {
		c.registerFunc(&block.Functions[i])
	}

	// Check function bodies
	for i := range block.Functions {
		if block.Functions[i].Body != nil {
			c.checkFuncBody(&block.Functions[i])
		}
	}
}

func (c *Checker) registerStruct(s *ast.StructDecl) {
	info := &TypeInfo{
		Type:   &Type{Kind: TyStruct, Name: s.Name},
		Fields: make(map[string]*Type),
	}
	for _, f := range s.Fields {
		info.Fields[f.Name] = c.resolveTypeExpr(&f.Type)
	}
	c.registry.Register(s.Name, info)
}

func (c *Checker) registerClass(cls *ast.ClassDecl) {
	info := &TypeInfo{
		Type:    &Type{Kind: TyClass, Name: cls.Name},
		Fields:  make(map[string]*Type),
		Methods: make(map[string]*Type),
	}
	// Ctor params become struct fields
	for _, p := range cls.CtorParams {
		info.Fields[p.Name] = c.resolveTypeExpr(&p.Type)
	}
	for _, f := range cls.Fields {
		info.Fields[f.Name] = c.resolveTypeExpr(&f.Type)
	}
	for _, m := range cls.Methods {
		info.Methods[m.Name] = c.funcDeclToType(&m)
	}
	c.registry.Register(cls.Name, info)
}

func (c *Checker) registerEnum(e *ast.EnumDecl) {
	info := &TypeInfo{
		Type: &Type{Kind: TyEnum, Name: e.Name},
	}
	c.registry.Register(e.Name, info)
}

func (c *Checker) registerInterface(iface *ast.InterfaceDecl) {
	info := &TypeInfo{
		Type:    &Type{Kind: TyInterface, Name: iface.Name},
		Methods: make(map[string]*Type),
	}
	for _, m := range iface.Methods {
		info.Methods[m.Name] = c.funcDeclToType(&m)
	}
	// Compose parent interfaces: copy their methods
	for _, parent := range iface.Implements {
		parentInfo := c.registry.Lookup(parent)
		if parentInfo == nil {
			c.error(ast.Span{}, "interface %s implements unknown type %s", iface.Name, parent)
			continue
		}
		for name, typ := range parentInfo.Methods {
			if _, exists := info.Methods[name]; !exists {
				info.Methods[name] = typ
			}
		}
	}
	c.registry.Register(iface.Name, info)
}

// checkImplements verifies that a class satisfies all its declared interfaces.
func (c *Checker) checkImplements(cls *ast.ClassDecl) {
	classInfo := c.registry.Lookup(cls.Name)
	if classInfo == nil {
		return
	}
	for _, ifaceName := range cls.Implements {
		ifaceInfo := c.registry.Lookup(ifaceName)
		if ifaceInfo == nil {
			c.error(ast.Span{}, "class %s implements unknown type %s", cls.Name, ifaceName)
			continue
		}
		if ifaceInfo.Type.Kind != TyInterface {
			c.error(ast.Span{}, "class %s implements %s which is not an interface", cls.Name, ifaceName)
			continue
		}
		for methodName, ifaceMethod := range ifaceInfo.Methods {
			classMethod, ok := classInfo.Methods[methodName]
			if !ok {
				c.error(ast.Span{}, "class %s missing method %s required by interface %s", cls.Name, methodName, ifaceName)
				continue
			}
			// Check method signature compatibility (param count + types, return type)
			if len(classMethod.Params) != len(ifaceMethod.Params) {
				c.error(ast.Span{}, "class %s method %s has %d params, interface %s requires %d",
					cls.Name, methodName, len(classMethod.Params), ifaceName, len(ifaceMethod.Params))
				continue
			}
			for i, p := range ifaceMethod.Params {
				if !p.Equal(classMethod.Params[i]) {
					c.error(ast.Span{}, "class %s method %s param %d type mismatch: got %s, want %s",
						cls.Name, methodName, i+1, classMethod.Params[i], p)
				}
			}
			if !ifaceMethod.Return.Equal(classMethod.Return) {
				c.error(ast.Span{}, "class %s method %s return type mismatch: got %s, want %s",
					cls.Name, methodName, classMethod.Return, ifaceMethod.Return)
			}
		}
	}
}

func (c *Checker) registerFunc(fn *ast.FuncDecl) {
	fnType := c.funcDeclToType(fn)
	c.scope.Define(fn.Name, fnType)
}

func (c *Checker) funcDeclToType(fn *ast.FuncDecl) *Type {
	var params []*Type
	for _, p := range fn.Params {
		if p.IsSelf {
			continue
		}
		params = append(params, c.resolveTypeExpr(&p.Type))
	}
	ret := TypeUnit
	if fn.ReturnType != nil {
		ret = c.resolveTypeExpr(fn.ReturnType)
	}
	return &Type{Kind: TyFunc, Params: params, Return: ret}
}

func (c *Checker) checkFuncBody(fn *ast.FuncDecl) {
	c.pushScope()
	defer c.popScope()

	// Set expected return type
	prevReturn := c.currentReturn
	if fn.ReturnType != nil {
		c.currentReturn = c.resolveTypeExpr(fn.ReturnType)
	} else {
		c.currentReturn = nil
	}
	defer func() { c.currentReturn = prevReturn }()

	// Bind parameters
	for _, p := range fn.Params {
		if !p.IsSelf {
			c.scope.Define(p.Name, c.resolveTypeExpr(&p.Type))
		}
	}

	// Check body statements
	for i := range fn.Body.Stmts {
		c.checkStmt(&fn.Body.Stmts[i])
	}
}

func (c *Checker) checkStructLit(expr *ast.Expr) *Type {
	sl := expr.Data.(*ast.StructLitExpr)
	info := c.registry.Lookup(sl.TypeName)
	if info == nil {
		c.error(expr.Span, "undefined type %q", sl.TypeName)
		return TypeError
	}
	if info.Type.Kind != TyStruct && info.Type.Kind != TyClass {
		c.error(expr.Span, "%q is not a struct or class type", sl.TypeName)
		return TypeError
	}
	for _, f := range sl.Fields {
		valType := c.checkExpr(&f.Value)
		// Try both the literal field name and lowercase version (Grok uses lowercase,
		// struct literals may use Go-exported names)
		fieldType, ok := info.Fields[f.Name]
		if !ok {
			lower := strings.ToLower(f.Name[:1]) + f.Name[1:]
			fieldType, ok = info.Fields[lower]
		}
		if ok {
			if !fieldType.Equal(valType) && valType.Kind != TyUnknown && fieldType.Kind != TyUnknown {
				c.error(expr.Span, "field %s: expected %s, got %s", f.Name, fieldType, valType)
			}
		} else {
			c.error(expr.Span, "struct %s has no field %q", sl.TypeName, f.Name)
		}
	}
	return info.Type
}
