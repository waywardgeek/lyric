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
	TyUnion                   // union type (T | U)
	TyUnknown                 // not yet resolved
	TyError                   // error sentinel
)

// Type represents a resolved type.
type Type struct {
	Kind           TypeKind
	Name           string  // for named types (struct, class, enum, type vars)
	Bits           int     // for int/uint/float (8, 16, 32, 64, 128, 256)
	Elem           *Type   // for list, optional, channel
	Key            *Type   // for map
	Val            *Type   // for map
	Fields         []TypeField // for tuple
	Params         []*Type     // for func: param types
	Return         *Type       // for func: return type
	TypeParamNames []string    // for func: generic type parameter names
	Variants       []*Type    // for union: member types
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
		if t.Bits == -1 {
			return "int"
		}
		return fmt.Sprintf("i%d", t.Bits)
	case TyUint:
		if t.Bits == -1 {
			return "uint"
		}
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
	case TyTuple:
		s := "("
		for i, f := range t.Fields {
			if i > 0 {
				s += ", "
			}
			s += f.Type.String()
		}
		return s + ")"
	case TyVar:
		return t.Name
	case TyUnion:
		s := ""
		for i, v := range t.Variants {
			if i > 0 {
				s += " | "
			}
			s += v.String()
		}
		return s
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
	case TyUnion:
		if len(t.Variants) != len(other.Variants) {
			return false
		}
		for i := range t.Variants {
			if !t.Variants[i].Equal(other.Variants[i]) {
				return false
			}
		}
		return true
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
			// Platform int (Bits:-1) accepts any same-kind integer
			if to.Bits == -1 && from.Bits > 0 {
				return true
			}
			if from.Bits == -1 && to.Bits > 0 {
				return true
			}
			return from.Bits > 0 && to.Bits > 0 && from.Bits < to.Bits
		}
	}
	return false
}

// coerceNumeric returns the wider type if one side can widen to the other, else nil.
func coerceNumeric(left, right *Type) *Type {
	// Platform int (Bits:-1) always wins in coercion
	if left.Kind == right.Kind {
		if left.Bits == -1 {
			return left
		}
		if right.Bits == -1 {
			return right
		}
	}
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

// VariantInfo holds field information for an enum variant.
type VariantInfo struct {
	EnumName string
	Fields   []TypeField // variant fields (named or positional)
}

// TypeInfo holds registered type information.
type TypeInfo struct {
	Type     *Type
	Fields   map[string]*Type    // struct/class fields
	Methods  map[string]*Type    // method signatures (as TyFunc)
	Variants map[string]*VariantInfo // enum variants (variant name → info)
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
	// len(collection) -> int — works on lists, strings, maps (Go's len returns int)
	c.scope.Define("len", &Type{Kind: TyFunc, Params: nil, Return: &Type{Kind: TyInt, Bits: -1}, Name: "len"})
	// append(list, elem) -> list — adds element to list, returns new list
	c.scope.Define("append", &Type{Kind: TyFunc, Params: nil, Return: TypeUnknown, Name: "append"})
	// isnull(optional) -> bool — checks if an optional value is nil
	c.scope.Define("isnull", &Type{Kind: TyFunc, Params: nil, Return: TypeBool, Name: "isnull"})
	// Register builtin types
	// error — Go's error interface, used in (T, error) return patterns
	c.registry.Register("error", &TypeInfo{
		Type: &Type{Kind: TyInterface, Name: "error"},
	})
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
	case ast.TypeTuple:
		tt := te.Data.(ast.TupleType)
		var fields []TypeField
		for _, f := range tt.Fields {
			ft := c.resolveTypeExpr(&f.Type)
			fields = append(fields, TypeField{Name: f.Name, Type: ft})
		}
		return &Type{Kind: TyTuple, Fields: fields}
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
	case ast.TypeUnion:
		ut := te.Data.(ast.UnionType)
		var variants []*Type
		for i := range ut.Variants {
			variants = append(variants, c.resolveTypeExpr(&ut.Variants[i]))
		}
		return &Type{Kind: TyUnion, Variants: variants}
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
		return IntType(-1) // Go platform-sized int (for stdlib interop casts)
	case "uint":
		return UintType(-1) // Go platform-sized uint (for stdlib interop casts)
	case "float":
		return FloatType(64)
	default:
		// Check registry for user-defined types
		if info := c.registry.Lookup(name); info != nil {
			return info.Type
		}
		// Check scope for type variables (generics)
		if t := c.scope.Lookup(name); t != nil && t.Kind == TyVar {
			return t
		}
		// Unknown type — could be an error, but return TyVar for .grok compatibility
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
	// Type variables are compatible with anything (constraints checked by Go)
	if from.Kind == TyVar || to.Kind == TyVar {
		return true
	}
	// nil (TyUnknown) is assignable to optional and interface types
	if from.Kind == TyUnknown && (to.Kind == TyOptional || to.Kind == TyInterface) {
		return true
	}
	// Numeric widening (e.g., int → i32, i32 → i64)
	if numericWidens(from, to) {
		return true
	}
	// T is assignable to T? (optional wrapping)
	if to.Kind == TyOptional && to.Elem != nil && c.assignableTo(from, to.Elem) {
		return true
	}
	// Composite types: recurse into element types
	if from.Kind == to.Kind {
		switch from.Kind {
		case TyList:
			if from.Elem == nil || from.Elem.Kind == TyUnknown {
				return true // empty list [] is assignable to any typed list
			}
			if to.Elem != nil {
				return c.assignableTo(from.Elem, to.Elem)
			}
		case TyMap:
			if from.Key != nil && to.Key != nil && from.Val != nil && to.Val != nil {
				return c.assignableTo(from.Key, to.Key) && c.assignableTo(from.Val, to.Val)
			}
		case TyOptional:
			if from.Elem != nil && to.Elem != nil {
				return c.assignableTo(from.Elem, to.Elem)
			}
		case TyTuple:
			if len(from.Fields) == len(to.Fields) {
				for i := range from.Fields {
					if !c.assignableTo(from.Fields[i].Type, to.Fields[i].Type) {
						return false
					}
				}
				return true
			}
		}
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
	// Any member type is assignable to a union containing it
	if to.Kind == TyUnion {
		for _, v := range to.Variants {
			if c.assignableTo(from, v) {
				return true
			}
		}
	}
	// Union is assignable to a type if all variants are assignable to it
	if from.Kind == TyUnion {
		for _, v := range from.Variants {
			if !c.assignableTo(v, to) {
				return false
			}
		}
		return true
	}
	return false
}

// substituteType replaces type variables in a type according to a substitution map.
func substituteType(t *Type, subst map[string]*Type) *Type {
	if t == nil {
		return nil
	}
	switch t.Kind {
	case TyVar:
		if concrete, ok := subst[t.Name]; ok {
			return concrete
		}
		return t
	case TyList:
		return ListType(substituteType(t.Elem, subst))
	case TyMap:
		return MapType(substituteType(t.Key, subst), substituteType(t.Val, subst))
	case TyOptional:
		return OptionalType(substituteType(t.Elem, subst))
	case TyTuple:
		fields := make([]TypeField, len(t.Fields))
		for i, f := range t.Fields {
			fields[i] = TypeField{Name: f.Name, Type: substituteType(f.Type, subst)}
		}
		return &Type{Kind: TyTuple, Fields: fields}
	case TyFunc:
		params := make([]*Type, len(t.Params))
		for i, p := range t.Params {
			params[i] = substituteType(p, subst)
		}
		return &Type{Kind: TyFunc, Params: params, Return: substituteType(t.Return, subst), Name: t.Name}
	case TyUnion:
		variants := make([]*Type, len(t.Variants))
		for i, v := range t.Variants {
			variants[i] = substituteType(v, subst)
		}
		return &Type{Kind: TyUnion, Variants: variants}
	default:
		return t
	}
}

// inferTypeArgs infers generic type arguments from actual argument types.
// Walks each parameter type, and when a TyVar is found, binds it to the
// corresponding argument type (first match wins). Returns nil if inference fails.
func (c *Checker) inferTypeArgs(fnType *Type, argTypes []*Type) map[string]*Type {
	subst := make(map[string]*Type)
	paramTypes := fnType.Params
	if paramTypes == nil {
		return nil
	}
	n := len(paramTypes)
	if n > len(argTypes) {
		n = len(argTypes)
	}
	for i := 0; i < n; i++ {
		matchTypeVars(paramTypes[i], argTypes[i], subst)
	}
	// Check that all type params were resolved
	for _, name := range fnType.TypeParamNames {
		if _, ok := subst[name]; !ok {
			c.error(ast.Span{}, "cannot infer type argument %s; provide explicit type arguments", name)
			return nil
		}
	}
	return subst
}

// matchTypeVars recursively walks a parameter type and an argument type in parallel,
// binding TyVar names to concrete types when found.
func matchTypeVars(param, arg *Type, subst map[string]*Type) {
	if param == nil || arg == nil {
		return
	}
	switch param.Kind {
	case TyVar:
		if _, ok := subst[param.Name]; !ok {
			subst[param.Name] = arg
		}
	case TyList:
		if arg.Kind == TyList {
			matchTypeVars(param.Elem, arg.Elem, subst)
		}
	case TyMap:
		if arg.Kind == TyMap {
			matchTypeVars(param.Key, arg.Key, subst)
			matchTypeVars(param.Val, arg.Val, subst)
		}
	case TyOptional:
		if arg.Kind == TyOptional {
			matchTypeVars(param.Elem, arg.Elem, subst)
		}
	case TyTuple:
		if arg.Kind == TyTuple && len(param.Fields) == len(arg.Fields) {
			for i := range param.Fields {
				matchTypeVars(param.Fields[i].Type, arg.Fields[i].Type, subst)
			}
		}
	case TyFunc:
		if arg.Kind == TyFunc && len(param.Params) == len(arg.Params) {
			for i := range param.Params {
				matchTypeVars(param.Params[i], arg.Params[i], subst)
			}
			matchTypeVars(param.Return, arg.Return, subst)
		}
	case TyUnion:
		if arg.Kind == TyUnion && len(param.Variants) == len(arg.Variants) {
			for i := range param.Variants {
				matchTypeVars(param.Variants[i], arg.Variants[i], subst)
			}
		}
	}
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
	case ast.ExprSlice:
		return c.checkSlice(expr)
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
	case ast.ExprCast:
		return c.checkCast(expr)
	case ast.ExprUnwrap:
		return c.checkUnwrap(expr)
	case ast.ExprLambda:
		return c.checkLambda(expr)
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

	// Type variables: allow all operations, defer constraint checking to Go
	if left.Kind == TyVar || right.Kind == TyVar {
		switch b.Op {
		case ast.OpAnd, ast.OpOr, ast.OpEq, ast.OpNeq, ast.OpLt, ast.OpLe, ast.OpGt, ast.OpGe:
			return TypeBool
		default:
			if left.Kind == TyVar {
				return left
			}
			return right
		}
	}

	switch b.Op {
	case ast.OpAdd, ast.OpSub, ast.OpMul, ast.OpDiv, ast.OpMod:
		if left.IsNumeric() && right.IsNumeric() {
			if left.Equal(right) {
				return left
			}
			if wider := coerceNumeric(left, right); wider != nil {
				// Propagate wider type to literal operands so transpiler emits correct type
				// Only for literals — variables keep their original type for proper casting
				if b.Left.Kind == ast.ExprIntLit || b.Left.Kind == ast.ExprFloatLit {
					b.Left.ResolvedType = wider
				}
				if b.Right.Kind == ast.ExprIntLit || b.Right.Kind == ast.ExprFloatLit {
					b.Right.ResolvedType = wider
				}
				return wider
			}
			c.error(expr.Span, "mismatched numeric types: %s and %s", left, right)
			return TypeError
		}
		// String concatenation (allow unknown on either side if the other is string)
		if b.Op == ast.OpAdd && (left.Equal(TypeString) && (right.Equal(TypeString) || right.Kind == TyUnknown) ||
			right.Equal(TypeString) && left.Kind == TyUnknown) {
			return TypeString
		}
		c.error(expr.Span, "cannot apply %v to %s and %s", b.Op, left, right)
		return TypeError

	case ast.OpEq, ast.OpNeq:
		// Any two compatible types can be compared for equality
		if !left.Equal(right) && left.Kind != TyUnknown && right.Kind != TyUnknown {
			if numericWidens(left, right) || numericWidens(right, left) {
				// Propagate platform int to literal operands
				if left.Bits == -1 && (b.Right.Kind == ast.ExprIntLit || b.Right.Kind == ast.ExprFloatLit) {
					b.Right.ResolvedType = left
				}
				if right.Bits == -1 && (b.Left.Kind == ast.ExprIntLit || b.Left.Kind == ast.ExprFloatLit) {
					b.Left.ResolvedType = right
				}
			} else {
				c.error(expr.Span, "cannot compare %s and %s for equality", left, right)
			}
		}
		return TypeBool

	case ast.OpLt, ast.OpLe, ast.OpGt, ast.OpGe:
		if left.IsNumeric() && right.IsNumeric() {
			if left.Equal(right) {
				return TypeBool
			}
			if wider := coerceNumeric(left, right); wider != nil {
				// Propagate to literals only
				if b.Left.Kind == ast.ExprIntLit || b.Left.Kind == ast.ExprFloatLit {
					b.Left.ResolvedType = wider
				}
				if b.Right.Kind == ast.ExprIntLit || b.Right.Kind == ast.ExprFloatLit {
					b.Right.ResolvedType = wider
				}
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

	// Track whether args were already type-checked (during inference)
	argsChecked := false

	// Handle generic type arguments: build substitution map
	var subst map[string]*Type
	if len(call.TypeArgs) > 0 {
		if len(call.TypeArgs) != len(fnType.TypeParamNames) {
			c.error(expr.Span, "expected %d type arguments, got %d", len(fnType.TypeParamNames), len(call.TypeArgs))
		} else {
			subst = make(map[string]*Type)
			for i, name := range fnType.TypeParamNames {
				subst[name] = c.resolveTypeExpr(&call.TypeArgs[i])
			}
		}
	} else if len(fnType.TypeParamNames) > 0 && len(call.Args) > 0 {
		// Type inference: infer type arguments from actual argument types
		argTypes := make([]*Type, len(call.Args))
		for i := range call.Args {
			argTypes[i] = c.checkExpr(&call.Args[i])
		}
		subst = c.inferTypeArgs(fnType, argTypes)
		if subst != nil {
			// Store inferred type args on the AST for the transpiler
			inferred := make([]any, len(fnType.TypeParamNames))
			for i, name := range fnType.TypeParamNames {
				inferred[i] = subst[name]
			}
			call.InferredTypeArgs = inferred
		}
		argsChecked = true
	}

	// Apply substitution to param and return types
	paramTypes := fnType.Params
	retType := fnType.Return
	if subst != nil {
		paramTypes = make([]*Type, len(fnType.Params))
		for i, p := range fnType.Params {
			paramTypes[i] = substituteType(p, subst)
		}
		retType = substituteType(fnType.Return, subst)
	}

	if paramTypes == nil {
		// Variadic/builtin — just check each arg is valid
		if !argsChecked {
			for i := range call.Args {
				c.checkExpr(&call.Args[i])
			}
		}
	} else if len(call.Args) != len(paramTypes) {
		c.error(expr.Span, "expected %d arguments, got %d", len(paramTypes), len(call.Args))
	} else if !argsChecked {
		for i := range call.Args {
			argType := c.checkExpr(&call.Args[i])
			if !c.assignableTo(argType, paramTypes[i]) && argType.Kind != TyUnknown {
				c.error(call.Args[i].Span, "argument %d: expected %s, got %s", i+1, paramTypes[i], argType)
			}
			// Propagate expected type to literal args so transpiler emits correct type
			// (e.g. identity<i64>(100) should emit int64(100), not int32(100))
			if call.Args[i].Kind == ast.ExprIntLit || call.Args[i].Kind == ast.ExprFloatLit {
				if paramTypes[i].IsNumeric() {
					call.Args[i].ResolvedType = paramTypes[i]
				}
			}
		}
	}
	return retType
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
	// Built-in methods on primitive types
	if ret := c.checkBuiltinMethod(recvType, mc.Method, mc.Args, expr); ret != nil {
		return ret
	}
	// Check args but return unknown
	for i := range mc.Args {
		c.checkExpr(&mc.Args[i])
	}
	return TypeUnknown
}

// checkBuiltinMethod resolves methods on built-in types (string, list, map).
// Returns nil if the method is not a known built-in.
func (c *Checker) checkBuiltinMethod(recvType *Type, method string, args []ast.Expr, expr *ast.Expr) *Type {
	switch recvType.Kind {
	case TyString:
		return c.checkStringMethod(method, args, expr)
	case TyList:
		return c.checkListMethod(recvType, method, args, expr)
	case TyMap:
		return c.checkMapMethod(recvType, method, args, expr)
	}
	return nil
}

func (c *Checker) checkStringMethod(method string, args []ast.Expr, expr *ast.Expr) *Type {
	switch method {
	case "len":
		c.expectArgs(method, args, 0, expr)
		return &Type{Kind: TyInt, Bits: -1}
	case "contains", "has_prefix", "has_suffix":
		c.expectArgs(method, args, 1, expr)
		if len(args) > 0 {
			c.expectArgType(method, &args[0], 0, TypeString)
		}
		return TypeBool
	case "to_upper", "to_lower", "trim", "trim_left", "trim_right":
		c.expectArgs(method, args, 0, expr)
		return TypeString
	case "replace":
		c.expectArgs(method, args, 2, expr)
		for i := range args {
			if i < 2 {
				c.expectArgType(method, &args[i], i, TypeString)
			}
		}
		return TypeString
	case "split":
		c.expectArgs(method, args, 1, expr)
		if len(args) > 0 {
			c.expectArgType(method, &args[0], 0, TypeString)
		}
		return ListType(TypeString)
	case "index_of":
		c.expectArgs(method, args, 1, expr)
		if len(args) > 0 {
			c.expectArgType(method, &args[0], 0, TypeString)
		}
		return &Type{Kind: TyInt, Bits: -1}
	case "repeat":
		c.expectArgs(method, args, 1, expr)
		// Accept any integer type for count
		if len(args) > 0 {
			t := c.checkExpr(&args[0])
			if !t.IsInteger() && t.Kind != TyUnknown {
				c.error(args[0].Span, "%s: argument 1 must be integer, got %s", method, t)
			}
		}
		return TypeString
	}
	return nil
}

func (c *Checker) checkListMethod(recvType *Type, method string, args []ast.Expr, expr *ast.Expr) *Type {
	elemType := recvType.Elem
	if elemType == nil {
		elemType = TypeUnknown
	}
	switch method {
	case "len":
		c.expectArgs(method, args, 0, expr)
		return &Type{Kind: TyInt, Bits: -1}
	case "push":
		c.expectArgs(method, args, 1, expr)
		if len(args) > 0 {
			c.expectArgType(method, &args[0], 0, elemType)
		}
		return &Type{Kind: TyUnit}
	case "pop":
		c.expectArgs(method, args, 0, expr)
		return elemType
	case "contains":
		c.expectArgs(method, args, 1, expr)
		if len(args) > 0 {
			c.expectArgType(method, &args[0], 0, elemType)
		}
		return TypeBool
	case "reverse":
		c.expectArgs(method, args, 0, expr)
		return recvType
	case "join":
		c.expectArgs(method, args, 1, expr)
		if len(args) > 0 {
			c.expectArgType(method, &args[0], 0, TypeString)
		}
		return TypeString
	}
	return nil
}

func (c *Checker) checkMapMethod(recvType *Type, method string, args []ast.Expr, expr *ast.Expr) *Type {
	switch method {
	case "len":
		c.expectArgs(method, args, 0, expr)
		return &Type{Kind: TyInt, Bits: -1}
	case "contains_key":
		c.expectArgs(method, args, 1, expr)
		if len(args) > 0 && recvType.Key != nil {
			c.expectArgType(method, &args[0], 0, recvType.Key)
		}
		return TypeBool
	case "keys":
		c.expectArgs(method, args, 0, expr)
		if recvType.Key != nil {
			return ListType(recvType.Key)
		}
		return ListType(TypeUnknown)
	case "values":
		c.expectArgs(method, args, 0, expr)
		if recvType.Val != nil {
			return ListType(recvType.Val)
		}
		return ListType(TypeUnknown)
	}
	return nil
}

// Helper: check expected arg count
func (c *Checker) expectArgs(method string, args []ast.Expr, expected int, expr *ast.Expr) {
	if len(args) != expected {
		c.error(expr.Span, "%s expects %d arguments, got %d", method, expected, len(args))
	}
}

// Helper: check arg type
func (c *Checker) expectArgType(method string, arg *ast.Expr, idx int, expected *Type) {
	t := c.checkExpr(arg)
	if !c.assignableTo(t, expected) && t.Kind != TyUnknown {
		c.error(arg.Span, "%s: argument %d: expected %s, got %s", method, idx+1, expected, t)
	}
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

func (c *Checker) checkSlice(expr *ast.Expr) *Type {
	sl := expr.Data.(*ast.SliceExpr)
	recvType := c.checkExpr(&sl.Receiver)
	if sl.Low != nil {
		lowType := c.checkExpr(sl.Low)
		if !lowType.IsInteger() && lowType.Kind != TyUnknown {
			c.error(expr.Span, "slice low bound must be integer, got %s", lowType)
		}
	}
	if sl.High != nil {
		highType := c.checkExpr(sl.High)
		if !highType.IsInteger() && highType.Kind != TyUnknown {
			c.error(expr.Span, "slice high bound must be integer, got %s", highType)
		}
	}
	// Slicing a list returns a list, slicing a string returns a string
	switch recvType.Kind {
	case TyList:
		return recvType
	case TyString:
		return TypeString
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

func (c *Checker) checkCast(expr *ast.Expr) *Type {
	cast := expr.Data.(*ast.CastExpr)
	targetType := c.resolveTypeExpr(&cast.TargetType)
	fromType := c.checkExpr(&cast.Operand)
	// For now, only validate numeric ↔ numeric casts
	if fromType.Kind != TyUnknown && targetType.Kind != TyUnknown {
		fromNumeric := fromType.IsNumeric()
		toNumeric := targetType.IsNumeric()
		if !fromNumeric || !toNumeric {
			c.error(expr.Span, "cannot cast %s to %s (only numeric casts supported)", fromType, targetType)
		}
	}
	return targetType
}

func (c *Checker) checkUnwrap(expr *ast.Expr) *Type {
	unwrap := expr.Data.(*ast.UnwrapExpr)
	operandType := c.checkExpr(&unwrap.Operand)
	if operandType.Kind == TyOptional {
		return operandType.Elem
	}
	if operandType.Kind != TyUnknown {
		c.error(expr.Span, "cannot unwrap non-optional type %s", operandType)
	}
	return TypeUnknown
}

func (c *Checker) checkLambda(expr *ast.Expr) *Type {
	lam := expr.Data.(*ast.LambdaExpr)

	// Build parameter types in a new scope
	c.pushScope()
	paramTypes := make([]*Type, len(lam.Params))
	for i, p := range lam.Params {
		paramTypes[i] = c.resolveTypeExpr(&lam.Params[i].Type)
		c.scope.Define(p.Name, paramTypes[i])
	}

	// Resolve return type
	var retType *Type
	if lam.ReturnType != nil {
		retType = c.resolveTypeExpr(lam.ReturnType)
	} else {
		retType = &Type{Kind: TyUnit}
	}

	// Check body
	savedReturn := c.currentReturn
	c.currentReturn = retType
	if lam.Body != nil {
		for i := range lam.Body.Stmts {
			c.checkStmt(&lam.Body.Stmts[i])
		}
	}
	c.currentReturn = savedReturn
	c.popScope()

	return &Type{Kind: TyFunc, Params: paramTypes, Return: retType}
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

	// Tuple destructuring: let (a, b) = expr
	if len(decl.Names) > 0 {
		if decl.Value != nil {
			valType := c.checkExpr(decl.Value)
			if valType.Kind == TyTuple && len(valType.Fields) == len(decl.Names) {
				for i, name := range decl.Names {
					if decl.IsMut {
						c.scope.DefineMut(name, valType.Fields[i].Type)
					} else {
						c.scope.Define(name, valType.Fields[i].Type)
					}
				}
			} else if valType.Kind != TyUnknown {
				c.error(stmt.Span, "cannot destructure %s into %d variables", valType, len(decl.Names))
				for _, name := range decl.Names {
					c.scope.Define(name, TypeError)
				}
			} else {
				for _, name := range decl.Names {
					c.scope.Define(name, TypeUnknown)
				}
			}
		} else {
			c.error(stmt.Span, "tuple destructuring requires an initializer")
			for _, name := range decl.Names {
				c.scope.Define(name, TypeError)
			}
		}
		return
	}

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
		// Both present: check compatibility (widening, interface subtyping, composites)
		if !c.assignableTo(inferredType, declaredType) && inferredType.Kind != TyUnknown {
			c.error(stmt.Span, "type mismatch in variable %q: declared %s, got %s", decl.Name, declaredType, inferredType)
		}
		// Propagate declared type to the value expression (e.g., int literal 100 becomes i64 not i32)
		if decl.Value != nil {
			decl.Value.ResolvedType = declaredType
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
	if !c.assignableTo(valueType, targetType) && targetType.Kind != TyUnknown && valueType.Kind != TyUnknown {
		c.error(stmt.Span, "cannot assign %s to %s", valueType, targetType)
	}
}

func (c *Checker) checkReturn(stmt *ast.Stmt) {
	ret := stmt.Data.(*ast.ReturnStmt)
	if ret.Value != nil {
		valType := c.checkExpr(ret.Value)
		if c.currentReturn != nil {
			if !c.assignableTo(valType, c.currentReturn) && valType.Kind != TyUnknown && c.currentReturn.Kind != TyUnknown {
				c.error(stmt.Span, "return type mismatch: expected %s, got %s", c.currentReturn, valType)
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
	if forStmt.IndexVar != "" {
		// Platform int type for index
		c.scope.Define(forStmt.IndexVar, &Type{Kind: TyInt, Bits: -1})
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
		// Look up variant field types from the enum's registry entry
		var fieldTypes []TypeField
		if matchType.Kind == TyEnum {
			if enumInfo := c.registry.Lookup(matchType.Name); enumInfo != nil {
				if vi, ok := enumInfo.Variants[vp.Name]; ok {
					fieldTypes = vi.Fields
				}
			}
		}
		for i := range vp.Bindings {
			var bindType *Type
			if i < len(fieldTypes) {
				bindType = fieldTypes[i].Type
			} else {
				bindType = TypeUnknown
			}
			c.bindPattern(&vp.Bindings[i], bindType)
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

	// Check class method bodies
	for i := range block.Classes {
		cls := &block.Classes[i]
		classType := c.registry.Lookup(cls.Name)
		for j := range cls.Methods {
			if cls.Methods[j].Body != nil {
				// Define 'self' in the method's scope
				c.scope = NewScope(c.scope)
				if classType != nil {
					c.scope.Define("self", classType.Type)
				}
				// Register type params for generic classes
				for _, tp := range cls.TypeParams {
					c.scope.Define(tp.Name, &Type{Kind: TyVar, Name: tp.Name})
				}
				c.checkFuncBody(&cls.Methods[j])
				c.scope = c.scope.parent
			}
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
	// Register type params as type variables in a temporary scope for resolving field/method types
	c.pushScope()
	for _, tp := range cls.TypeParams {
		c.scope.Define(tp.Name, &Type{Kind: TyVar, Name: tp.Name})
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
	c.popScope()
	c.registry.Register(cls.Name, info)
	// Define class name in scope as constructor function
	var ctorParams []*Type
	for _, p := range cls.CtorParams {
		ctorParams = append(ctorParams, c.resolveTypeExpr(&p.Type))
	}
	var typeParamNames []string
	for _, tp := range cls.TypeParams {
		typeParamNames = append(typeParamNames, tp.Name)
	}
	c.scope.Define(cls.Name, &Type{
		Kind:           TyFunc,
		Name:           cls.Name,
		Params:         ctorParams,
		Return:         info.Type,
		TypeParamNames: typeParamNames,
	})
}

func (c *Checker) registerEnum(e *ast.EnumDecl) {
	enumType := &Type{Kind: TyEnum, Name: e.Name}
	info := &TypeInfo{
		Type:     enumType,
		Variants: make(map[string]*VariantInfo),
	}
	for _, v := range e.Variants {
		vi := &VariantInfo{EnumName: e.Name}
		var paramTypes []*Type
		for _, f := range v.Fields {
			ft := c.resolveTypeExpr(&f.Type)
			vi.Fields = append(vi.Fields, TypeField{Name: f.Name, Type: ft})
			paramTypes = append(paramTypes, ft)
		}
		info.Variants[v.Name] = vi

		// Register variant as a constructor function (or value for unit variants)
		if len(v.Fields) == 0 {
			// Unit variant: just a value of the enum type
			c.scope.Define(v.Name, enumType)
		} else {
			// Constructor: function from fields → enum type
			c.scope.Define(v.Name, &Type{
				Kind:   TyFunc,
				Name:   v.Name,
				Params: paramTypes,
				Return: enumType,
			})
		}
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
	var typeParamNames []string
	for _, tp := range fn.TypeParams {
		typeParamNames = append(typeParamNames, tp.Name)
	}
	return &Type{Kind: TyFunc, Params: params, Return: ret, TypeParamNames: typeParamNames}
}

func (c *Checker) checkFuncBody(fn *ast.FuncDecl) {
	c.pushScope()
	defer c.popScope()

	// Register type parameters as type variables
	for _, tp := range fn.TypeParams {
		c.scope.Define(tp.Name, &Type{Kind: TyVar, Name: tp.Name})
	}

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
			if !c.assignableTo(valType, fieldType) && valType.Kind != TyUnknown && fieldType.Kind != TyUnknown {
				c.error(expr.Span, "field %s: expected %s, got %s", f.Name, fieldType, valType)
			}
		} else {
			c.error(expr.Span, "struct %s has no field %q", sl.TypeName, f.Name)
		}
	}
	return info.Type
}
