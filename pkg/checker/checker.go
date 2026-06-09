// Package checker implements type checking and inference for Forge.
package checker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/waywardgeek/forge/pkg/ast"
	"github.com/waywardgeek/forge/pkg/parser"
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
	TyGenerator               // gen T
	TyStruct                  // named struct type
	TyClass                   // named class type
	TyEnum                    // named enum type
	TyInterface               // named interface type
	TyVar                     // type variable (for generics)
	TyUnion                   // union type (T | U)
	TyModule                  // imported Forge module (qualified access to exports)
	TyAny                     // any (empty interface)
	TyNil                     // nil literal — assignable to optional and interface types
	TyUnknown                 // not yet resolved — MUST NOT survive past checker
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
	TypeParamNames       []string    // for func: generic type parameter names
	TypeParamConstraints []string    // for func: constraints (parallel to TypeParamNames)
	Variants       []*Type    // for union: member types
	TypeArgs       []*Type    // for class/struct: concrete type arguments (e.g., DictEntry<InterfaceDecl>)
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
	case TyGenerator:
		return fmt.Sprintf("gen %s", t.Elem)
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
	case TyModule:
		return "<module " + t.Name + ">"
	case TyAny:
		return "any"
	case TyNil:
		return "nil"
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
	TypeAny     = &Type{Kind: TyAny}
	TypeNil     = &Type{Kind: TyNil}
	TypeUnknown = &Type{Kind: TyUnknown}
	TypeError   = &Type{Kind: TyError}
	TypeI32     = &Type{Kind: TyInt, Bits: 32}
	TypeI64     = &Type{Kind: TyInt, Bits: 64}
	TypeU8      = &Type{Kind: TyUint, Bits: 8}
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
	case TyList, TyOptional, TyChannel, TyGenerator:
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
	Type           *Type
	Fields         map[string]*Type        // struct/class fields
	FieldOrder     []string                // field names in declaration order (for positional struct literals)
	Methods        map[string]*Type        // method signatures (as TyFunc)
	Variants       map[string]*VariantInfo // enum variants (variant name → info)
	GuardedFields  map[string]string       // field name → lock name (guarded_by annotations)
	TypeParamNames []string                // generic type parameter names (for substitution)
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

// ModuleExports holds the public symbols exported by a Forge module.
type ModuleExports struct {
	Types     map[string]*TypeInfo // exported type names → type info
	Functions map[string]*Type     // exported function names → function types
}

// Checker performs type checking on a Forge AST.
type Checker struct {
	registry       *Registry
	errors         []error
	scope          *Scope
	currentReturn  *Type // expected return type of current function (nil = void/unit)
	loopDepth      int   // tracks nesting depth inside loops (for break/continue validation)
	modules        map[string]*ModuleExports // file path → exports (cache)
	checking       map[string]bool           // files currently being checked (cycle detection)
	currentFile    string                    // file path of the file being checked
	heldLocks      map[string]bool           // lock names currently held (for guarded_by enforcement)
	typeVarMethods map[string]map[string]*Type // type var name → method name → method type (from relational constraints)
	ifaceDecls     map[string]*ast.InterfaceDecl // interface name → AST declaration (for relational constraint resolution)
}

// New creates a new type checker.
func New() *Checker {
	c := &Checker{
		registry: NewRegistry(),
		scope:    NewScope(nil),
		modules:   make(map[string]*ModuleExports),
		checking:  make(map[string]bool),
		heldLocks: make(map[string]bool),
	}
	// Register builtin functions
	c.registerBuiltins()
	// Register builtin types
	// error — interface satisfied by any class with message(self) -> string
	c.registry.Register("error", &TypeInfo{
		Type: &Type{Kind: TyInterface, Name: "error"},
		Methods: map[string]*Type{
			"message": {Kind: TyFunc, Params: nil, Return: TypeString, Name: "message"},
		},
	})
	return c
}

// Errors returns all accumulated type errors.
func (c *Checker) Errors() []error {
	return c.errors
}

// registerBuiltins defines all built-in functions in the current scope.
func (c *Checker) registerBuiltins() {
	// Console I/O
	c.scope.Define("println", &Type{Kind: TyFunc, Params: nil, Return: TypeUnit, Name: "println"})
	c.scope.Define("print", &Type{Kind: TyFunc, Params: nil, Return: TypeUnit, Name: "print"})
	c.scope.Define("eprint", &Type{Kind: TyFunc, Params: nil, Return: TypeUnit, Name: "eprint"})
	c.scope.Define("eprintln", &Type{Kind: TyFunc, Params: nil, Return: TypeUnit, Name: "eprintln"})

	// Collections
	c.scope.Define("len", &Type{Kind: TyFunc, Params: nil, Return: &Type{Kind: TyInt, Bits: -1}, Name: "len"})
	c.scope.Define("append", &Type{Kind: TyFunc, Params: nil, Return: TypeError, Name: "append"})
	c.scope.Define("isnull", &Type{Kind: TyFunc, Params: nil, Return: TypeBool, Name: "isnull"})
	c.scope.Define("make_channel", &Type{Kind: TyFunc, Params: nil, Return: TypeError, Name: "make_channel"})

	// Hashing
	c.scope.Define("hash_string", &Type{Kind: TyFunc, Params: []*Type{TypeString}, Return: &Type{Kind: TyUint, Bits: 64}, Name: "hash_string"})

	// File I/O — read_file(path) -> (string, bool), write_file(path, data) -> bool
	c.scope.Define("read_file", &Type{Kind: TyFunc, Params: []*Type{TypeString},
		Return: &Type{Kind: TyTuple, Fields: []TypeField{{Type: TypeString}, {Type: TypeBool}}}, Name: "read_file"})
	c.scope.Define("write_file", &Type{Kind: TyFunc, Params: []*Type{TypeString, TypeString},
		Return: TypeBool, Name: "write_file"})

	// OS
	c.scope.Define("os_args", &Type{Kind: TyFunc, Params: nil,
		Return: ListType(TypeString), Name: "os_args"})
	c.scope.Define("os_exit", &Type{Kind: TyFunc, Params: []*Type{TypeI32},
		Return: TypeUnit, Name: "os_exit"})
	c.scope.Define("os_getwd", &Type{Kind: TyFunc, Params: nil,
		Return: TypeString, Name: "os_getwd"})

	// Process execution — exec_command(program, args) -> (string, bool)
	c.scope.Define("exec_command", &Type{Kind: TyFunc, Params: []*Type{TypeString, ListType(TypeString)},
		Return: &Type{Kind: TyTuple, Fields: []TypeField{{Type: TypeString}, {Type: TypeBool}}}, Name: "exec_command"})

	// Path manipulation
	c.scope.Define("path_join", &Type{Kind: TyFunc, Params: []*Type{ListType(TypeString)},
		Return: TypeString, Name: "path_join"})
	c.scope.Define("path_dir", &Type{Kind: TyFunc, Params: []*Type{TypeString},
		Return: TypeString, Name: "path_dir"})
	c.scope.Define("path_base", &Type{Kind: TyFunc, Params: []*Type{TypeString},
		Return: TypeString, Name: "path_base"})
	c.scope.Define("path_ext", &Type{Kind: TyFunc, Params: []*Type{TypeString},
		Return: TypeString, Name: "path_ext"})

	// String conversion
	c.scope.Define("itoa", &Type{Kind: TyFunc, Params: []*Type{&Type{Kind: TyInt, Bits: -1}},
		Return: TypeString, Name: "itoa"})
	c.scope.Define("atoi", &Type{Kind: TyFunc, Params: []*Type{TypeString},
		Return: &Type{Kind: TyTuple, Fields: []TypeField{{Type: &Type{Kind: TyInt, Bits: 64}}, {Type: TypeBool}}}, Name: "atoi"})
	c.scope.Define("parse_float", &Type{Kind: TyFunc, Params: []*Type{TypeString},
		Return: &Type{Kind: TyTuple, Fields: []TypeField{{Type: &Type{Kind: TyFloat, Bits: 64}}, {Type: TypeBool}}}, Name: "parse_float"})
	c.scope.Define("char_to_string", &Type{Kind: TyFunc, Params: []*Type{TypeU8},
		Return: TypeString, Name: "char_to_string"})

	// Testing
	c.scope.Define("assert", &Type{Kind: TyFunc, Params: []*Type{TypeBool, TypeString},
		Return: TypeUnit, Name: "assert"})
	c.scope.Define("assert_eq", &Type{Kind: TyFunc, Params: nil, // variadic: (any, any, string)
		Return: TypeUnit, Name: "assert_eq"})

	// Register known Go stdlib modules so module-qualified calls get proper types
	c.registerGoStdlibModules()
}

// registerGoStdlibModules registers type information for known Go standard library
// modules. This allows the checker to resolve return types for calls like
// strings.ToUpper("x") instead of falling back to TypeError.
func (c *Checker) registerGoStdlibModules() {
	register := func(modName string, methods map[string]*Type) {
		modInfo := &TypeInfo{
			Type:    &Type{Kind: TyModule, Name: modName},
			Fields:  make(map[string]*Type),
			Methods: make(map[string]*Type),
		}
		for name, t := range methods {
			modInfo.Methods[name] = t
		}
		c.registry.Register(modName, modInfo)
	}

	// strings module
	register("strings", map[string]*Type{
		"ToUpper":    {Kind: TyFunc, Params: []*Type{TypeString}, Return: TypeString},
		"ToLower":    {Kind: TyFunc, Params: []*Type{TypeString}, Return: TypeString},
		"Contains":   {Kind: TyFunc, Params: []*Type{TypeString, TypeString}, Return: TypeBool},
		"HasPrefix":  {Kind: TyFunc, Params: []*Type{TypeString, TypeString}, Return: TypeBool},
		"HasSuffix":  {Kind: TyFunc, Params: []*Type{TypeString, TypeString}, Return: TypeBool},
		"TrimSpace":  {Kind: TyFunc, Params: []*Type{TypeString}, Return: TypeString},
		"Split":      {Kind: TyFunc, Params: []*Type{TypeString, TypeString}, Return: ListType(TypeString)},
		"Join":       {Kind: TyFunc, Params: []*Type{ListType(TypeString), TypeString}, Return: TypeString},
		"Replace":    {Kind: TyFunc, Params: []*Type{TypeString, TypeString, TypeString, TypeI32}, Return: TypeString},
		"ReplaceAll": {Kind: TyFunc, Params: []*Type{TypeString, TypeString, TypeString}, Return: TypeString},
	})

	// fmt module
	register("fmt", map[string]*Type{
		"Sprintf":  {Kind: TyFunc, Params: nil, Return: TypeString},
		"Errorf":   {Kind: TyFunc, Params: nil, Return: &Type{Kind: TyError}},
		"Println":  {Kind: TyFunc, Params: nil, Return: TypeUnit},
		"Printf":   {Kind: TyFunc, Params: nil, Return: TypeUnit},
	})

	// errors module
	register("errors", map[string]*Type{
		"New": {Kind: TyFunc, Params: []*Type{TypeString}, Return: &Type{Kind: TyError}},
	})

	// strconv module
	register("strconv", map[string]*Type{
		"Itoa": {Kind: TyFunc, Params: []*Type{&Type{Kind: TyInt, Bits: -1}}, Return: TypeString},
		"Atoi": {Kind: TyFunc, Params: []*Type{TypeString},
			Return: &Type{Kind: TyTuple, Fields: []TypeField{{Type: &Type{Kind: TyInt, Bits: -1}}, {Type: &Type{Kind: TyError}}}}},
	})
}

// CheckModuleFile parses, checks, and caches a .fg module file.
// Returns the module's exported symbols. Uses cycle detection and caching.
func (c *Checker) CheckModuleFile(importPath string, fromSpan ast.Span) *ModuleExports {
	// Resolve path relative to current file
	absPath := importPath
	if c.currentFile != "" && !filepath.IsAbs(importPath) {
		absPath = filepath.Join(filepath.Dir(c.currentFile), importPath)
	}
	absPath, _ = filepath.Abs(absPath)

	// Check cache
	if exports, ok := c.modules[absPath]; ok {
		return exports
	}

	// Cycle detection
	if c.checking[absPath] {
		c.error(fromSpan, "import cycle detected: %s", importPath)
		return nil
	}
	c.checking[absPath] = true
	defer delete(c.checking, absPath)

	// Read and parse — supports both single files and directories
	var file *ast.File
	info, err := os.Stat(absPath)
	if err != nil {
		c.error(fromSpan, "cannot read module %q: %v", importPath, err)
		return nil
	}
	if info.IsDir() {
		// Directory import: read all .fg files, parse each, merge into one
		entries, err := os.ReadDir(absPath)
		if err != nil {
			c.error(fromSpan, "cannot read module directory %q: %v", importPath, err)
			return nil
		}
		var files []*ast.File
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".fg") {
				continue
			}
			fpath := filepath.Join(absPath, entry.Name())
			src, err := os.ReadFile(fpath)
			if err != nil {
				c.error(fromSpan, "cannot read %q: %v", fpath, err)
				return nil
			}
			f, err := parser.ParseFile(string(src), fpath)
			if err != nil {
				c.error(fromSpan, "parse error in %q: %v", fpath, err)
				return nil
			}
			files = append(files, f)
		}
		if len(files) == 0 {
			c.error(fromSpan, "module directory %q contains no .fg files", importPath)
			return nil
		}
		file = ast.MergeFiles(files)
	} else {
		// Single file import
		src, err := os.ReadFile(absPath)
		if err != nil {
			c.error(fromSpan, "cannot read module %q: %v", importPath, err)
			return nil
		}
		file, err = parser.ParseFile(string(src), absPath)
		if err != nil {
			c.error(fromSpan, "parse error in module %q: %v", importPath, err)
			return nil
		}
	}

	// Save and restore checker state for the module
	savedRegistry := c.registry
	savedScope := c.scope
	savedFile := c.currentFile
	c.registry = NewRegistry()
	c.scope = NewScope(nil)
	c.currentFile = absPath
	// Re-register builtins in new scope
	c.registerBuiltins()
	c.registry.Register("error", &TypeInfo{
		Type: &Type{Kind: TyInterface, Name: "error"},
		Methods: map[string]*Type{
			"message": {Kind: TyFunc, Params: nil, Return: TypeString, Name: "message"},
		},
	})
	c.CheckFile(file)

	// Extract exports: pub types and pub functions
	exports := &ModuleExports{
		Types:     make(map[string]*TypeInfo),
		Functions: make(map[string]*Type),
	}
	for _, block := range file.Blocks {
		for _, s := range block.Structs {
			if s.IsPublic {
				if info := c.registry.Lookup(s.Name); info != nil {
					exports.Types[s.Name] = info
				}
			}
		}
		for _, cl := range block.Classes {
			if cl.IsPublic {
				if info := c.registry.Lookup(cl.Name); info != nil {
					exports.Types[cl.Name] = info
				}
			}
		}
		for _, e := range block.Enums {
			if e.IsPublic {
				if info := c.registry.Lookup(e.Name); info != nil {
					exports.Types[e.Name] = info
				}
			}
		}
		for _, iface := range block.Interfaces {
			if iface.IsPublic {
				if info := c.registry.Lookup(iface.Name); info != nil {
					exports.Types[iface.Name] = info
				}
			}
		}
		for _, f := range block.Functions {
			if f.IsPublic {
				if t := c.scope.Lookup(f.Name); t != nil {
					exports.Functions[f.Name] = t
				}
			}
		}
	}

	// Restore state
	c.registry = savedRegistry
	c.scope = savedScope
	c.currentFile = savedFile

	// Cache
	c.modules[absPath] = exports
	return exports
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
		return TypeError // missing type expression is a bug in the AST
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
	case ast.TypeLock:
		return &Type{Kind: TyStruct, Name: "Mutex"}
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
	case ast.TypeChannel:
		ct := te.Data.(ast.ChannelType)
		elem := c.resolveTypeExpr(&ct.Elem)
		return &Type{Kind: TyChannel, Elem: elem}
	case ast.TypeGenerator:
		gt := te.Data.(ast.GeneratorType)
		elem := c.resolveTypeExpr(&gt.Elem)
		return &Type{Kind: TyGenerator, Elem: elem}
	default:
		c.error(te.Span, "unsupported type expression kind %v", te.Kind)
		return TypeError
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
	case "any":
		return TypeAny
	default:
		// Check registry for user-defined types
		if info := c.registry.Lookup(name); info != nil {
			// Track type arguments on generic class/struct instances
			if len(args) > 0 {
				var typeArgs []*Type
				for i := range args {
					typeArgs = append(typeArgs, c.resolveTypeExpr(&args[i]))
				}
				return &Type{Kind: info.Type.Kind, Name: info.Type.Name, TypeArgs: typeArgs}
			}
			return info.Type
		}
		// Check scope for type variables (generics)
		if t := c.scope.Lookup(name); t != nil && t.Kind == TyVar {
			return t
		}
		// Unknown type — could be an error, but return TyVar for .forge compatibility
		return &Type{Kind: TyVar, Name: name}
	}
}

// buildTypeArgSubst creates a type substitution map from a type's TypeArgs
// and the class/struct's TypeParamNames. Returns nil if no substitution needed.
func (c *Checker) buildTypeArgSubst(recvType *Type) map[string]*Type {
	if len(recvType.TypeArgs) == 0 {
		return nil
	}
	info := c.registry.Lookup(recvType.Name)
	if info == nil || len(info.TypeParamNames) == 0 {
		return nil
	}
	subst := make(map[string]*Type)
	for i, name := range info.TypeParamNames {
		if i < len(recvType.TypeArgs) {
			subst[name] = recvType.TypeArgs[i]
		}
	}
	return subst
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
	// any accepts any type; any is assignable to any
	if from.Kind == TyAny || to.Kind == TyAny {
		return true
	}
	// nil is assignable to optional, interface, class, and struct types
	if from.Kind == TyNil {
		return true
	}
	// TyError (error sentinel / error type) is assignable to error interface and suppresses cascading
	if from.Kind == TyError || to.Kind == TyError {
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
			if from.Elem == nil || from.Elem.Kind == TyUnit {
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
	case TyChannel:
		return &Type{Kind: TyChannel, Elem: substituteType(t.Elem, subst)}
	case TyGenerator:
		return &Type{Kind: TyGenerator, Elem: substituteType(t.Elem, subst)}
	case TyClass, TyStruct:
		// Track type arguments on generic class/struct instances
		if len(t.TypeArgs) > 0 {
			newArgs := make([]*Type, len(t.TypeArgs))
			for i, a := range t.TypeArgs {
				newArgs[i] = substituteType(a, subst)
			}
			return &Type{Kind: t.Kind, Name: t.Name, TypeArgs: newArgs}
		}
		return t
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
	case TyChannel:
		if arg.Kind == TyChannel {
			matchTypeVars(param.Elem, arg.Elem, subst)
		}
	case TyGenerator:
		if arg.Kind == TyGenerator {
			matchTypeVars(param.Elem, arg.Elem, subst)
		}
	}
}

// --- Expression type inference ---

func (c *Checker) checkExpr(expr *ast.Expr) *Type {
	if expr == nil {
		return TypeError
	}
	t := c.inferExpr(expr)
	expr.ResolvedType = t

	return t
}

func (c *Checker) inferExpr(expr *ast.Expr) *Type {
	switch expr.Kind {
	case ast.ExprIntLit:
		lit := expr.Data.(*ast.IntLitExpr)
		if lit.TypeHint == "u8" {
			return TypeU8
		}
		return TypeI32 // default integer literal type
	case ast.ExprFloatLit:
		return TypeF64
	case ast.ExprStringLit:
		return TypeString
	case ast.ExprStringInterp:
		interp := expr.Data.(*ast.StringInterpExpr)
		for i := range interp.Parts {
			if i%2 != 0 {
				c.checkExpr(&interp.Parts[i])
			} else {
				// Even parts are string literals — annotate them
				interp.Parts[i].ResolvedType = TypeString
			}
		}
		return TypeString
	case ast.ExprBoolLit:
		return TypeBool
	case ast.ExprNil:
		// nil literal — callers with context (checkVarDecl, checkCall) override ResolvedType.
		return TypeNil
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
	case ast.ExprTry:
		return c.checkTry(expr)
	case ast.ExprLambda:
		return c.checkLambda(expr)
	default:
		c.error(expr.Span, "unsupported expression kind %v", expr.Kind)
		return TypeError
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
	c.error(expr.Span, "unsupported unary operator %v", u.Op)
	return TypeError
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
		if b.Op == ast.OpAdd && (left.Equal(TypeString) && (right.Equal(TypeString) || right.Kind == TyError) ||
			right.Equal(TypeString) && left.Kind == TyError) {
			return TypeString
		}
		c.error(expr.Span, "cannot apply %v to %s and %s", b.Op, left, right)
		return TypeError

	case ast.OpEq, ast.OpNeq:
		// Optional/class/interface compared with nil is always valid
		if left.Kind == TyNil && right.Kind != TyNil {
			b.Left.ResolvedType = right
			return TypeBool
		}
		if right.Kind == TyNil && left.Kind != TyNil {
			b.Right.ResolvedType = left
			return TypeBool
		}
		// Any two compatible types can be compared for equality
		if !left.Equal(right) && left.Kind != TyError && right.Kind != TyError {
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
	c.error(expr.Span, "unsupported binary operator %v", b.Op)
	return TypeError
}

func (c *Checker) checkCall(expr *ast.Expr) *Type {
	call := expr.Data.(*ast.CallExpr)
	fnType := c.checkExpr(&call.Func)

	// Special builtin: append(slice, elem) -> slice (return type = first arg type)
	if fnType.Kind == TyFunc && fnType.Name == "append" {
		if len(call.Args) < 1 {
			c.error(expr.Span, "append requires at least 1 argument")
			return TypeError
		}
		sliceType := c.checkExpr(&call.Args[0])
		for i := 1; i < len(call.Args); i++ {
			c.checkExpr(&call.Args[i])
		}
		return sliceType
	}

	// Special builtin: make_channel<T>(capacity?) -> channel<T>
	if fnType.Kind == TyFunc && fnType.Name == "make_channel" {
		if len(call.TypeArgs) != 1 {
			c.error(expr.Span, "make_channel requires exactly 1 type argument")
			return TypeError
		}
		elemType := c.resolveTypeExpr(&call.TypeArgs[0])
		// Check optional capacity arg
		for i := range call.Args {
			c.checkExpr(&call.Args[i])
		}
		if len(call.Args) > 1 {
			c.error(expr.Span, "make_channel accepts 0 or 1 arguments (capacity)")
		}
		return &Type{Kind: TyChannel, Elem: elemType}
	}

	if fnType.Kind == TyError {
		// Still check args for side effects and sub-expression typing
		for i := range call.Args {
			c.checkExpr(&call.Args[i])
		}
		return TypeError
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
		// Validate constraints
		for i, name := range fnType.TypeParamNames {
			if i < len(fnType.TypeParamConstraints) && fnType.TypeParamConstraints[i] != "" {
				if concreteType, ok := subst[name]; ok && concreteType != nil {
					if !c.satisfiesConstraint(concreteType, fnType.TypeParamConstraints[i]) {
						c.error(expr.Span, "type %s does not satisfy constraint %s", concreteType, fnType.TypeParamConstraints[i])
					}
				}
			}
		}
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
			if !c.assignableTo(argType, paramTypes[i]) && argType.Kind != TyError {
				c.error(call.Args[i].Span, "argument %d: expected %s, got %s", i+1, paramTypes[i], argType)
			}
			// Propagate expected type to literal args so transpiler emits correct type
			// (e.g. identity<i64>(100) should emit int64(100), not int32(100))
			if call.Args[i].Kind == ast.ExprIntLit || call.Args[i].Kind == ast.ExprFloatLit {
				if paramTypes[i].IsNumeric() {
					call.Args[i].ResolvedType = paramTypes[i]
				}
			}
			// Propagate expected type to nil args (needed for Go generics — bare nil
			// is invalid for type parameters)
			if call.Args[i].Kind == ast.ExprNil {
				call.Args[i].ResolvedType = paramTypes[i]
			}
		}
	}
	return retType
}

func (c *Checker) checkMethodCall(expr *ast.Expr) *Type {
	mc := expr.Data.(*ast.MethodCallExpr)
	recvType := c.checkExpr(&mc.Receiver)
	// Module method call: mod.func(args)
	if recvType.Kind == TyModule {
		if info := c.registry.Lookup(recvType.Name); info != nil {
			if methType, ok := info.Methods[mc.Method]; ok && methType.Kind == TyFunc {
				// Check argument types against function params
				if methType.Params != nil && len(mc.Args) != len(methType.Params) {
					c.error(expr.Span, "%s.%s expects %d arguments, got %d", recvType.Name, mc.Method, len(methType.Params), len(mc.Args))
				}
				for i := range mc.Args {
					argType := c.checkExpr(&mc.Args[i])
					if methType.Params != nil && i < len(methType.Params) {
						if !c.assignableTo(argType, methType.Params[i]) && argType.Kind != TyError {
							c.error(mc.Args[i].Span, "%s.%s: argument %d: expected %s, got %s", recvType.Name, mc.Method, i+1, methType.Params[i], argType)
						}
					}
				}
				return methType.Return
			}
			c.error(expr.Span, "module %q has no exported function %q", recvType.Name, mc.Method)
			return TypeError
		}
		// Unregistered module — check args, return TypeError (no way to resolve)
		for i := range mc.Args {
			c.checkExpr(&mc.Args[i])
		}
		return TypeError
	}
	// Look up method on the receiver type
	if recvType.Kind == TyStruct || recvType.Kind == TyClass || recvType.Kind == TyInterface {
		if info := c.registry.Lookup(recvType.Name); info != nil {
			if methType, ok := info.Methods[mc.Method]; ok && methType.Kind == TyFunc {
				// Check argument types
				for i := range mc.Args {
					c.checkExpr(&mc.Args[i])
				}
				retType := methType.Return
				// Substitute type args for generic class instances
				if subst := c.buildTypeArgSubst(recvType); subst != nil {
					retType = substituteType(retType, subst)
				}
				return retType
			}
		}
	}
	// Built-in methods on primitive types
	if ret := c.checkBuiltinMethod(recvType, mc.Method, mc.Args, expr); ret != nil {
		return ret
	}
	// Type variable methods from relational constraints (where Graph<G, N, E>)
	if recvType.Kind == TyVar && c.typeVarMethods != nil {
		if methods, ok := c.typeVarMethods[recvType.Name]; ok {
			if methType, ok := methods[mc.Method]; ok && methType.Kind == TyFunc {
				// Check argument count
				if methType.Params != nil && len(mc.Args) != len(methType.Params) {
					c.error(expr.Span, "%s.%s expects %d arguments, got %d", recvType.Name, mc.Method, len(methType.Params), len(mc.Args))
				}
				for i := range mc.Args {
					c.checkExpr(&mc.Args[i])
					// Propagate expected type to nil args for Go generics
					if mc.Args[i].Kind == ast.ExprNil && methType.Params != nil && i < len(methType.Params) {
						mc.Args[i].ResolvedType = methType.Params[i]
					}
				}
				return methType.Return
			}
		}
	}
	// Universal methods available on all types
	if mc.Method == "to_string" {
		for i := range mc.Args {
			c.checkExpr(&mc.Args[i])
		}
		return TypeString
	}
	// Unknown method — check args but report error (suppress if receiver already errored)
	for i := range mc.Args {
		c.checkExpr(&mc.Args[i])
	}
	if recvType.Kind != TyError {
		c.error(expr.Span, "unknown method %q on type %s", mc.Method, recvType)
	}
	return TypeError
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
	case TyChannel:
		return c.checkChannelMethod(recvType, method, args, expr)
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
			if !t.IsInteger() && t.Kind != TyError {
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
		elemType = TypeError
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
		return ListType(TypeError)
	case "values":
		c.expectArgs(method, args, 0, expr)
		if recvType.Val != nil {
			return ListType(recvType.Val)
		}
		return ListType(TypeError)
	}
	return nil
}

func (c *Checker) checkChannelMethod(recvType *Type, method string, args []ast.Expr, expr *ast.Expr) *Type {
	switch method {
	case "send":
		c.expectArgs(method, args, 1, expr)
		if len(args) > 0 && recvType.Elem != nil {
			c.expectArgType(method, &args[0], 0, recvType.Elem)
		}
		return TypeUnit
	case "receive":
		c.expectArgs(method, args, 0, expr)
		if recvType.Elem != nil {
			return recvType.Elem
		}
		return TypeError
	case "close":
		c.expectArgs(method, args, 0, expr)
		return TypeUnit
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
	if !c.assignableTo(t, expected) && t.Kind != TyError {
		c.error(arg.Span, "%s: argument %d: expected %s, got %s", method, idx+1, expected, t)
	}
}

// extractLockName returns the name of the lock expression for guarded_by tracking.
// Handles simple identifiers (mu) and field access (self.mu).
func (c *Checker) extractLockName(expr *ast.Expr) string {
	switch expr.Kind {
	case ast.ExprIdent:
		return expr.Data.(*ast.IdentExpr).Name
	case ast.ExprFieldAccess:
		fa := expr.Data.(*ast.FieldAccessExpr)
		return fa.Field
	}
	return ""
}

func (c *Checker) checkFieldAccess(expr *ast.Expr) *Type {
	fa := expr.Data.(*ast.FieldAccessExpr)
	recvType := c.checkExpr(&fa.Receiver)
	if recvType.Kind == TyStruct || recvType.Kind == TyClass {
		if info := c.registry.Lookup(recvType.Name); info != nil {
			if fieldType, ok := info.Fields[fa.Field]; ok {
				// guarded_by enforcement: check field is accessed under the correct lock
				if lockName, guarded := info.GuardedFields[fa.Field]; guarded {
					if !c.heldLocks[lockName] {
						c.error(expr.Span, "field %q is guarded by %q and must be accessed within lock(%s) { ... }", fa.Field, lockName, lockName)
					}
				}
				// Substitute type args for generic class instances
				if subst := c.buildTypeArgSubst(recvType); subst != nil {
					return substituteType(fieldType, subst)
				}
				return fieldType
			}
			c.error(expr.Span, "type %s has no field %q", recvType.Name, fa.Field)
			return TypeError
		}
	}
	if recvType.Kind == TyTuple {
		// Tuple field access: _0, _1, etc.
		if strings.HasPrefix(fa.Field, "_") {
			idxStr := fa.Field[1:]
			idx := 0
			valid := len(idxStr) > 0
			for _, ch := range idxStr {
				if ch < '0' || ch > '9' {
					valid = false
					break
				}
				idx = idx*10 + int(ch-'0')
			}
			if valid && idx < len(recvType.Fields) {
				return recvType.Fields[idx].Type
			}
		}
		c.error(expr.Span, "cannot access field %q on tuple type %s", fa.Field, recvType)
		return TypeError
	}
	if recvType.Kind == TyModule {
		if info := c.registry.Lookup(recvType.Name); info != nil {
			// Check types first (for struct/enum/class access)
			if fieldType, ok := info.Fields[fa.Field]; ok {
				return fieldType
			}
			// Check functions
			if methType, ok := info.Methods[fa.Field]; ok {
				return methType
			}
			c.error(expr.Span, "module %q has no exported symbol %q", recvType.Name, fa.Field)
			return TypeError
		}
	}
	if recvType.Kind != TyError {
		c.error(expr.Span, "cannot access field %q on type %s", fa.Field, recvType)
	}
	return TypeError
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
		if !indexType.Equal(recvType.Key) && indexType.Kind != TyError {
			c.error(expr.Span, "map key type mismatch: expected %s, got %s", recvType.Key, indexType)
		}
		return recvType.Val
	case TyString:
		if !indexType.IsInteger() {
			c.error(expr.Span, "string index must be integer, got %s", indexType)
		}
		return TypeU8 // string indexing returns a byte
	}
	if recvType.Kind != TyError {
		c.error(expr.Span, "cannot index type %s", recvType)
	}
	return TypeError
}

func (c *Checker) checkSlice(expr *ast.Expr) *Type {
	sl := expr.Data.(*ast.SliceExpr)
	recvType := c.checkExpr(&sl.Receiver)
	if sl.Low != nil {
		lowType := c.checkExpr(sl.Low)
		if !lowType.IsInteger() && lowType.Kind != TyError {
			c.error(expr.Span, "slice low bound must be integer, got %s", lowType)
		}
	}
	if sl.High != nil {
		highType := c.checkExpr(sl.High)
		if !highType.IsInteger() && highType.Kind != TyError {
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
	if recvType.Kind != TyError {
		c.error(expr.Span, "cannot slice type %s", recvType)
	}
	return TypeError
}

func (c *Checker) checkListLit(expr *ast.Expr) *Type {
	lit := expr.Data.(*ast.ListLitExpr)
	if len(lit.Elems) == 0 {
		// Empty list — type comes from context (checkVarDecl/checkStructLit override ResolvedType).
		// Return List(Unit) as placeholder; callers with declared types will override.
		return ListType(TypeUnit)
	}
	elemType := c.checkExpr(&lit.Elems[0])
	for i := 1; i < len(lit.Elems); i++ {
		t := c.checkExpr(&lit.Elems[i])
		if !t.Equal(elemType) && t.Kind != TyError && elemType.Kind != TyError {
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
		// Empty map — type comes from context. Placeholder until caller overrides.
		return MapType(TypeUnit, TypeUnit)
	}
	keyType := c.checkExpr(&lit.Entries[0].Key)
	valType := c.checkExpr(&lit.Entries[0].Value)
	for i := 1; i < len(lit.Entries); i++ {
		k := c.checkExpr(&lit.Entries[i].Key)
		v := c.checkExpr(&lit.Entries[i].Value)
		if !k.Equal(keyType) && k.Kind != TyError {
			c.error(lit.Entries[i].Key.Span, "map key type mismatch: expected %s, got %s", keyType, k)
		}
		if !v.Equal(valType) && v.Kind != TyError {
			c.error(lit.Entries[i].Value.Span, "map value type mismatch: expected %s, got %s", valType, v)
		}
	}
	return MapType(keyType, valType)
}

func (c *Checker) checkMatchExpr(expr *ast.Expr) *Type {
	m := expr.Data.(*ast.MatchStmt)
	c.checkExpr(&m.Value)
	matchType := c.inferExpr(&m.Value)
	isUnion := matchType.Kind == TyUnion
	// Infer type from last expression of first arm
	var resultType *Type
	for i := range m.Arms {
		c.pushScope()
		if isUnion {
			c.bindUnionPattern(&m.Arms[i].Pattern, matchType)
		} else {
			c.bindPattern(&m.Arms[i].Pattern, matchType)
		}
		// Check guard clause if present
		if m.Arms[i].Guard != nil {
			c.checkExpr(m.Arms[i].Guard)
		}
		for j := range m.Arms[i].Body.Stmts {
			c.checkStmt(&m.Arms[i].Body.Stmts[j])
		}
		// Use last stmt if it's an expr stmt
		if len(m.Arms[i].Body.Stmts) > 0 {
			last := &m.Arms[i].Body.Stmts[len(m.Arms[i].Body.Stmts)-1]
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
		return TypeUnit
	}
	return resultType
}

func (c *Checker) checkCast(expr *ast.Expr) *Type {
	cast := expr.Data.(*ast.CastExpr)
	targetType := c.resolveTypeExpr(&cast.TargetType)
	fromType := c.checkExpr(&cast.Operand)
	// For now, only validate numeric ↔ numeric casts
	if fromType.Kind != TyError && targetType.Kind != TyError {
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
	if operandType.Kind != TyError {
		c.error(expr.Span, "cannot unwrap non-optional type %s", operandType)
	}
	return TypeError
}

// isErrorType returns true if the type represents Go's error interface.
func isErrorType(t *Type) bool {
	if t.Kind == TyError {
		return true
	}
	if t.Kind == TyInterface && t.Name == "error" {
		return true
	}
	return false
}

func (c *Checker) checkTry(expr *ast.Expr) *Type {
	try := expr.Data.(*ast.TryExpr)
	operandType := c.checkExpr(&try.Operand)

	// Operand must be (T, error) tuple
	if operandType.Kind != TyTuple || len(operandType.Fields) < 2 {
		if operandType.Kind != TyError {
			c.error(expr.Span, "? operator requires (T, error) return type, got %s", operandType)
		}
		return TypeError
	}

	lastField := operandType.Fields[len(operandType.Fields)-1]
	if !isErrorType(lastField.Type) {
		c.error(expr.Span, "? operator requires last tuple element to be error, got %s", lastField.Type)
		return TypeError
	}

	// Enclosing function must also return (..., error)
	if c.currentReturn == nil {
		c.error(expr.Span, "? operator can only be used in functions that return error")
		return TypeError
	}
	if c.currentReturn.Kind == TyTuple {
		lastRet := c.currentReturn.Fields[len(c.currentReturn.Fields)-1]
		if !isErrorType(lastRet.Type) {
			c.error(expr.Span, "? operator can only be used in functions that return (..., error)")
		}
	} else if !isErrorType(c.currentReturn) {
		c.error(expr.Span, "? operator can only be used in functions that return error")
	}

	// Result type: if single non-error field, return it directly; otherwise tuple of non-error fields
	nonErrorFields := operandType.Fields[:len(operandType.Fields)-1]
	if len(nonErrorFields) == 1 {
		return nonErrorFields[0].Type
	}
	// Return tuple of non-error field types
	return &Type{Kind: TyTuple, Fields: nonErrorFields}
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
	case ast.StmtSpawn:
		ss := stmt.Data.(*ast.SpawnStmt)
		c.checkBlock(&ss.Body)
	case ast.StmtSelect:
		sel := stmt.Data.(*ast.SelectStmt)
		for i := range sel.Cases {
			sc := &sel.Cases[i]
			if sc.Expr != nil {
				t := c.checkExpr(sc.Expr)
				if sc.BindVar != "" {
					// Bind the receive result
					c.scope = NewScope(c.scope)
					c.scope.Define(sc.BindVar, t)
					c.checkBlock(&sc.Body)
					c.scope = c.scope.parent
					continue
				}
			}
			c.checkBlock(&sc.Body)
		}
	case ast.StmtLock:
		ls := stmt.Data.(*ast.LockStmt)
		c.checkExpr(&ls.Mutex)
		// Track the held lock for guarded_by enforcement
		lockName := c.extractLockName(&ls.Mutex)
		if lockName != "" {
			c.heldLocks[lockName] = true
		}
		c.checkBlock(&ls.Body)
		if lockName != "" {
			delete(c.heldLocks, lockName)
		}
	case ast.StmtBreak:
		if c.loopDepth == 0 {
			c.error(stmt.Span, "break outside of loop")
		}
	case ast.StmtContinue:
		if c.loopDepth == 0 {
			c.error(stmt.Span, "continue outside of loop")
		}
	case ast.StmtYield:
		ys := stmt.Data.(*ast.YieldStmt)
		if ys.Value != nil {
			c.checkExpr(ys.Value)
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
			} else if valType.Kind != TyError {
				c.error(stmt.Span, "cannot destructure %s into %d variables", valType, len(decl.Names))
				for _, name := range decl.Names {
					c.scope.Define(name, TypeError)
				}
			} else {
				for _, name := range decl.Names {
					c.scope.Define(name, TypeError)
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
		if !c.assignableTo(inferredType, declaredType) && inferredType.Kind != TyError {
			c.error(stmt.Span, "type mismatch in variable %q: declared %s, got %s", decl.Name, declaredType, inferredType)
		}
		// Propagate declared type to the value expression (e.g., int literal 100 becomes i64 not i32)
		// Don't propagate union types — the literal keeps its concrete type for correct Go emission.
		if decl.Value != nil && declaredType.Kind != TyUnion {
			decl.Value.ResolvedType = declaredType
		}
		finalType = declaredType
	} else if declaredType != nil {
		finalType = declaredType
	} else if inferredType != nil {
		// null/nil without type annotation has no type context
		if decl.Value != nil && decl.Value.Kind == ast.ExprNil {
			c.error(stmt.Span, "cannot infer type of null without annotation; use 'let x: T? = null'")
		}
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
	if !c.assignableTo(valueType, targetType) && targetType.Kind != TyError && valueType.Kind != TyError {
		c.error(stmt.Span, "cannot assign %s to %s", valueType, targetType)
	}
}

func (c *Checker) checkReturn(stmt *ast.Stmt) {
	ret := stmt.Data.(*ast.ReturnStmt)
	if ret.Value != nil {
		valType := c.checkExpr(ret.Value)
		if c.currentReturn != nil {
			if !c.assignableTo(valType, c.currentReturn) && valType.Kind != TyError && c.currentReturn.Kind != TyError {
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
	if !condType.Equal(TypeBool) && condType.Kind != TyError {
		c.error(stmt.Span, "if condition must be bool, got %s", condType)
	}
	c.checkBlock(&ifStmt.Then)
	for i := range ifStmt.ElseIfs {
		elifCond := c.checkExpr(&ifStmt.ElseIfs[i].Condition)
		if !elifCond.Equal(TypeBool) && elifCond.Kind != TyError {
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
	case TyGenerator:
		elemType = collType.Elem
	case TyString:
		elemType = TypeString // iterate characters as strings
	case TyMap:
		elemType = collType.Key // iterate keys
	default:
		if collType.Kind != TyError {
			c.error(stmt.Span, "cannot iterate over %s", collType)
		}
		elemType = TypeError
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
	if !condType.Equal(TypeBool) && condType.Kind != TyError {
		c.error(stmt.Span, "while condition must be bool, got %s", condType)
	}
	c.loopDepth++
	c.checkBlock(&whileStmt.Body)
	c.loopDepth--
}

func (c *Checker) checkMatch(stmt *ast.Stmt) {
	matchStmt := stmt.Data.(*ast.MatchStmt)
	matchType := c.checkExpr(&matchStmt.Value)
	isUnion := matchType.Kind == TyUnion
	for i := range matchStmt.Arms {
		c.pushScope()
		if isUnion {
			c.bindUnionPattern(&matchStmt.Arms[i].Pattern, matchType)
		} else {
			c.bindPattern(&matchStmt.Arms[i].Pattern, matchType)
		}
		if matchStmt.Arms[i].Guard != nil {
			c.checkExpr(matchStmt.Arms[i].Guard)
		}
		c.checkBlock(&matchStmt.Arms[i].Body)
		c.popScope()
	}

	// Exhaustiveness check for enum match
	if matchType.Kind == TyEnum {
		enumInfo := c.registry.Lookup(matchType.Name)
		if enumInfo != nil && len(enumInfo.Variants) > 0 {
			hasWildcard := false
			covered := make(map[string]bool)
			for _, arm := range matchStmt.Arms {
				switch arm.Pattern.Kind {
				case ast.PatWildcard:
					hasWildcard = true
				case ast.PatIdent:
					id := arm.Pattern.Data.(*ast.IdentPattern)
					if id.Name == "_" {
						hasWildcard = true
					} else {
						covered[id.Name] = true
					}
				case ast.PatVariant:
					vp := arm.Pattern.Data.(*ast.VariantPattern)
					covered[vp.Name] = true
				}
			}
			if !hasWildcard {
				var missing []string
				for name := range enumInfo.Variants {
					if !covered[name] {
						missing = append(missing, name)
					}
				}
				if len(missing) > 0 {
					c.error(stmt.Span, "non-exhaustive match on enum %s: missing variant(s) %v", matchType.Name, missing)
				}
			}
		}
	}

	// Exhaustiveness check for union match
	if isUnion && len(matchType.Variants) > 0 {
		hasWildcard := false
		covered := make(map[string]bool)
		for _, arm := range matchStmt.Arms {
			switch arm.Pattern.Kind {
			case ast.PatWildcard:
				hasWildcard = true
			case ast.PatIdent:
				id := arm.Pattern.Data.(*ast.IdentPattern)
				if id.Name == "_" {
					hasWildcard = true
				} else {
					covered[id.Name] = true
				}
			}
		}
		if !hasWildcard {
			var missing []string
			for _, v := range matchType.Variants {
				name := v.String()
				if !covered[name] {
					missing = append(missing, name)
				}
			}
			if len(missing) > 0 {
				c.error(stmt.Span, "non-exhaustive match on union %s: missing type(s) %v", matchType, missing)
			}
		}
	}
}

// satisfiesConstraint checks whether a concrete type satisfies a named constraint.
func (c *Checker) satisfiesConstraint(t *Type, constraint string) bool {
	switch constraint {
	case "Comparable":
		// Comparable types: all numeric, string, bool
		return t.IsNumeric() || t.Kind == TyString || t.Kind == TyBool
	case "Equatable":
		// Equatable: comparable in Go (==)
		return t.IsNumeric() || t.Kind == TyString || t.Kind == TyBool
	case "Hashable":
		// Same as Equatable for now
		return t.IsNumeric() || t.Kind == TyString || t.Kind == TyBool
	case "":
		return true // unconstrained
	default:
		// Check if constraint is a user-defined interface
		if info := c.registry.Lookup(constraint); info != nil && info.Type.Kind == TyInterface {
			return c.assignableTo(t, info.Type)
		}
		// Unknown constraint — pass through (let Go handle it)
		return true
	}
}

// bindUnionPattern handles match arms for union types. PatIdent names are
// resolved as type references (e.g. "string", "i32") and the implicit _m
// variable is bound with the narrowed type.
func (c *Checker) bindUnionPattern(pat *ast.Pattern, matchType *Type) {
	switch pat.Kind {
	case ast.PatIdent:
		id := pat.Data.(*ast.IdentPattern)
		if id.Name == "_" {
			return
		}
		// Try to resolve as a type name
		resolved := c.resolveNamedType(id.Name, nil)
		if resolved.Kind == TyError {
			c.error(pat.Span, "unknown type '%s' in union match", id.Name)
			return
		}
		// Verify this type is a member of the union
		found := false
		for _, v := range matchType.Variants {
			if v.Equal(resolved) {
				found = true
				break
			}
		}
		if !found {
			c.error(pat.Span, "type '%s' is not a member of union %s", id.Name, matchType)
		}
		// Store the resolved type on the pattern for the transpiler
		pat.ResolvedType = resolved
	case ast.PatWildcard:
		// nothing to bind
	default:
		c.bindPattern(pat, matchType)
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
				c.error(pat.Span, "too many bindings for variant %s (has %d fields)", vp.Name, len(fieldTypes))
				bindType = TypeError
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
				c.error(pat.Span, "too many elements in tuple pattern (expected %d)", len(matchType.Fields))
				elemType = TypeError
			}
			c.bindPattern(&tp.Elems[i], elemType)
		}
	case ast.PatLiteral:
		// Check the literal expression so it gets ResolvedType set
		lp := pat.Data.(*ast.LiteralPattern)
		c.checkExpr(&lp.Expr)
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

// CheckFiles type-checks multiple files with cross-file method resolution.
// Unlike calling CheckFile per-file, this runs Phase 0 and Phase 1 across
// ALL files before checking ANY bodies (Phase 2), so methods defined in
// file B are visible when checking bodies in file A.
func (c *Checker) CheckFiles(files []*ast.File) {
	// Phase 0: Pre-register all type NAMES across all files
	for _, file := range files {
		for i := range file.Blocks {
			block := &file.Blocks[i]
			for _, s := range block.Structs {
				if c.registry.Lookup(s.Name) == nil {
					c.registry.Register(s.Name, &TypeInfo{
						Type:   &Type{Kind: TyStruct, Name: s.Name},
						Fields: make(map[string]*Type),
					})
				}
			}
			for _, cls := range block.Classes {
				if c.registry.Lookup(cls.Name) == nil {
					c.registry.Register(cls.Name, &TypeInfo{
						Type:    &Type{Kind: TyClass, Name: cls.Name},
						Fields:  make(map[string]*Type),
						Methods: make(map[string]*Type),
					})
				}
			}
			for _, e := range block.Enums {
				if c.registry.Lookup(e.Name) == nil {
					c.registry.Register(e.Name, &TypeInfo{
						Type: &Type{Kind: TyEnum, Name: e.Name},
					})
				}
			}
			for _, iface := range block.Interfaces {
				if c.registry.Lookup(iface.Name) == nil {
					c.registry.Register(iface.Name, &TypeInfo{
						Type:    &Type{Kind: TyInterface, Name: iface.Name},
						Fields:  make(map[string]*Type),
						Methods: make(map[string]*Type),
					})
				}
			}
		}
	}
	// Phase 1: Register all types, functions, constants across ALL files
	for _, file := range files {
		for i := range file.Blocks {
			c.registerForgeBlock(&file.Blocks[i])
		}
	}
	// Phase 2: Check function and method bodies (all names now in scope)
	for _, file := range files {
		for i := range file.Blocks {
			c.checkForgeBlockBodies(&file.Blocks[i])
		}
	}
	// Phase 3: Validate
	for _, file := range files {
		c.validateAllTypesResolved(file)
	}
}

// CheckFile type-checks an entire AST file.
func (c *Checker) CheckFile(file *ast.File) {
	if file.Filename != "" && c.currentFile == "" {
		c.currentFile = file.Filename
	}
	// Phase 0: Pre-register all type names across all blocks so cross-block
	// references in function/method signatures resolve correctly during Phase 1.
	// Without this, a test block declaring `func f() -> Token` fails to resolve
	// Token if it's defined in a later block.
	for i := range file.Blocks {
		block := &file.Blocks[i]
		for _, s := range block.Structs {
			if c.registry.Lookup(s.Name) == nil {
				c.registry.Register(s.Name, &TypeInfo{
					Type:   &Type{Kind: TyStruct, Name: s.Name},
					Fields: make(map[string]*Type),
				})
			}
		}
		for _, cls := range block.Classes {
			if c.registry.Lookup(cls.Name) == nil {
				c.registry.Register(cls.Name, &TypeInfo{
					Type:    &Type{Kind: TyClass, Name: cls.Name},
					Fields:  make(map[string]*Type),
					Methods: make(map[string]*Type),
				})
			}
		}
		for _, e := range block.Enums {
			if c.registry.Lookup(e.Name) == nil {
				c.registry.Register(e.Name, &TypeInfo{
					Type: &Type{Kind: TyEnum, Name: e.Name},
				})
			}
		}
		for _, iface := range block.Interfaces {
			if c.registry.Lookup(iface.Name) == nil {
				c.registry.Register(iface.Name, &TypeInfo{
					Type:    &Type{Kind: TyInterface, Name: iface.Name},
					Fields:  make(map[string]*Type),
					Methods: make(map[string]*Type),
				})
			}
		}
	}
	// Phase 1: Register all types, functions, and constants across all blocks
	// so cross-block references resolve correctly in multi-file compilation.
	for i := range file.Blocks {
		c.registerForgeBlock(&file.Blocks[i])
	}
	// Phase 2: Check function and method bodies (all names now in scope).
	for i := range file.Blocks {
		c.checkForgeBlockBodies(&file.Blocks[i])
	}

	// Phase 3: Validate all function signatures are fully resolved.
	c.validateAllTypesResolved(file)

	// Phase 4: Validate all expressions have ResolvedType set.
	// Only run when there are no checker errors — error paths may leave
	// sub-expressions unannotated intentionally.
	if len(c.errors) == 0 {
		c.validateAllExprsResolved(file)
	}
}

// validateAllExprsResolved walks every Expr node in the AST after checking
// and panics if any has a nil ResolvedType. This catches checker gaps
// systematically rather than chasing panics one at a time in the lowerer.
func (c *Checker) validateAllExprsResolved(file *ast.File) {
	for _, block := range file.Blocks {
		// Check struct/class field defaults
		for i := range block.Structs {
			for j := range block.Structs[i].Fields {
				if block.Structs[i].Fields[j].Default != nil {
					c.walkExpr(block.Structs[i].Fields[j].Default, block.Structs[i].Name)
				}
			}
		}
		for i := range block.Classes {
			for j := range block.Classes[i].Fields {
				if block.Classes[i].Fields[j].Default != nil {
					c.walkExpr(block.Classes[i].Fields[j].Default, block.Classes[i].Name)
				}
			}
		}
		// Check constant values
		for i := range block.Constants {
			c.walkExpr(&block.Constants[i].Value, "const:"+block.Constants[i].Name)
		}
		// Check function bodies
		for i := range block.Functions {
			fn := &block.Functions[i]
			if len(fn.TypeParams) > 0 {
				continue // generic functions aren't checked until instantiation
			}
			name := fn.Name
			if fn.ReceiverType != "" {
				name = fn.ReceiverType + "." + fn.Name
			}
			if fn.Body != nil {
				c.walkBlock(fn.Body, name)
			}
		}
	}
}

func (c *Checker) walkBlock(block *ast.Block, ctx string) {
	for i := range block.Stmts {
		c.walkStmt(&block.Stmts[i], ctx)
	}
}

func (c *Checker) walkStmt(stmt *ast.Stmt, ctx string) {
	switch stmt.Kind {
	case ast.StmtVarDecl:
		d := stmt.Data.(*ast.VarDeclStmt)
		if d.Value != nil {
			c.walkExpr(d.Value, ctx)
		}
	case ast.StmtAssign:
		d := stmt.Data.(*ast.AssignStmt)
		c.walkExpr(&d.Target, ctx)
		c.walkExpr(&d.Value, ctx)
	case ast.StmtReturn:
		d := stmt.Data.(*ast.ReturnStmt)
		if d.Value != nil {
			c.walkExpr(d.Value, ctx)
		}
	case ast.StmtExpr:
		d := stmt.Data.(*ast.ExprStmt)
		c.walkExpr(&d.Expr, ctx)
	case ast.StmtIf:
		d := stmt.Data.(*ast.IfStmt)
		c.walkExpr(&d.Condition, ctx)
		c.walkBlock(&d.Then, ctx)
		for i := range d.ElseIfs {
			c.walkExpr(&d.ElseIfs[i].Condition, ctx)
			c.walkBlock(&d.ElseIfs[i].Body, ctx)
		}
		if d.Else != nil {
			c.walkBlock(d.Else, ctx)
		}
	case ast.StmtFor:
		d := stmt.Data.(*ast.ForStmt)
		c.walkExpr(&d.Collection, ctx)
		c.walkBlock(&d.Body, ctx)
	case ast.StmtWhile:
		d := stmt.Data.(*ast.WhileStmt)
		c.walkExpr(&d.Condition, ctx)
		c.walkBlock(&d.Body, ctx)
	case ast.StmtMatch:
		d := stmt.Data.(*ast.MatchStmt)
		c.walkExpr(&d.Value, ctx)
		for i := range d.Arms {
			c.walkPattern(&d.Arms[i].Pattern, ctx)
			for j := range d.Arms[i].Patterns {
				c.walkPattern(&d.Arms[i].Patterns[j], ctx)
			}
			if d.Arms[i].Guard != nil {
				c.walkExpr(d.Arms[i].Guard, ctx)
			}
			c.walkBlock(&d.Arms[i].Body, ctx)
		}
	case ast.StmtBlock:
		d := stmt.Data.(*ast.Block)
		c.walkBlock(d, ctx)
	case ast.StmtCascade:
		d := stmt.Data.(*ast.CascadeStmt)
		c.walkBlock(&d.Body, ctx)
	case ast.StmtSpawn:
		d := stmt.Data.(*ast.SpawnStmt)
		c.walkBlock(&d.Body, ctx)
	case ast.StmtSelect:
		d := stmt.Data.(*ast.SelectStmt)
		for i := range d.Cases {
			if d.Cases[i].Expr != nil {
				c.walkExpr(d.Cases[i].Expr, ctx)
			}
			c.walkBlock(&d.Cases[i].Body, ctx)
		}
	case ast.StmtYield:
		d := stmt.Data.(*ast.YieldStmt)
		if d.Value != nil {
			c.walkExpr(d.Value, ctx)
		}
	case ast.StmtLock:
		d := stmt.Data.(*ast.LockStmt)
		c.walkExpr(&d.Mutex, ctx)
		c.walkBlock(&d.Body, ctx)
	case ast.StmtBreak, ast.StmtContinue:
		// no expressions
	}
}

func (c *Checker) walkPattern(pat *ast.Pattern, ctx string) {
	switch pat.Kind {
	case ast.PatLiteral:
		d := pat.Data.(*ast.LiteralPattern)
		c.walkExpr(&d.Expr, ctx)
	case ast.PatVariant:
		d := pat.Data.(*ast.VariantPattern)
		for i := range d.Bindings {
			c.walkPattern(&d.Bindings[i], ctx)
		}
	case ast.PatTuple:
		d := pat.Data.(*ast.TuplePattern)
		for i := range d.Elems {
			c.walkPattern(&d.Elems[i], ctx)
		}
	case ast.PatIdent, ast.PatWildcard:
		// no sub-expressions
	}
}

func (c *Checker) walkExpr(expr *ast.Expr, ctx string) {
	if expr.ResolvedType == nil {
		panic(fmt.Sprintf("checker: validateAllExprsResolved: nil ResolvedType in %s at %s:%d:%d (kind=%d)",
			ctx, expr.Span.Start.File, expr.Span.Start.Line, expr.Span.Start.Column, expr.Kind))
	}
	// Recurse into sub-expressions
	switch expr.Kind {
	case ast.ExprIdent, ast.ExprIntLit, ast.ExprFloatLit, ast.ExprStringLit, ast.ExprBoolLit, ast.ExprNil:
		// leaf nodes
	case ast.ExprStringInterp:
		d := expr.Data.(*ast.StringInterpExpr)
		for i := range d.Parts {
			c.walkExpr(&d.Parts[i], ctx)
		}
	case ast.ExprCall:
		d := expr.Data.(*ast.CallExpr)
		c.walkExpr(&d.Func, ctx)
		for i := range d.Args {
			c.walkExpr(&d.Args[i], ctx)
		}
	case ast.ExprMethodCall:
		d := expr.Data.(*ast.MethodCallExpr)
		c.walkExpr(&d.Receiver, ctx)
		for i := range d.Args {
			c.walkExpr(&d.Args[i], ctx)
		}
	case ast.ExprFieldAccess:
		d := expr.Data.(*ast.FieldAccessExpr)
		c.walkExpr(&d.Receiver, ctx)
	case ast.ExprIndex:
		d := expr.Data.(*ast.IndexExpr)
		c.walkExpr(&d.Receiver, ctx)
		c.walkExpr(&d.Index, ctx)
	case ast.ExprUnary:
		d := expr.Data.(*ast.UnaryExpr)
		c.walkExpr(&d.Operand, ctx)
	case ast.ExprBinary:
		d := expr.Data.(*ast.BinaryExpr)
		c.walkExpr(&d.Left, ctx)
		c.walkExpr(&d.Right, ctx)
	case ast.ExprTupleLit:
		d := expr.Data.(*ast.TupleLitExpr)
		for i := range d.Elems {
			c.walkExpr(&d.Elems[i], ctx)
		}
	case ast.ExprListLit:
		d := expr.Data.(*ast.ListLitExpr)
		for i := range d.Elems {
			c.walkExpr(&d.Elems[i], ctx)
		}
	case ast.ExprMapLit:
		d := expr.Data.(*ast.MapLitExpr)
		for i := range d.Entries {
			c.walkExpr(&d.Entries[i].Key, ctx)
			c.walkExpr(&d.Entries[i].Value, ctx)
		}
	case ast.ExprStructLit:
		d := expr.Data.(*ast.StructLitExpr)
		for i := range d.Fields {
			c.walkExpr(&d.Fields[i].Value, ctx)
		}
	case ast.ExprLambda:
		d := expr.Data.(*ast.LambdaExpr)
		if d.Body != nil {
			c.walkBlock(d.Body, ctx+".<lambda>")
		}
	case ast.ExprCast:
		d := expr.Data.(*ast.CastExpr)
		c.walkExpr(&d.Operand, ctx)
	case ast.ExprUnwrap:
		d := expr.Data.(*ast.UnwrapExpr)
		c.walkExpr(&d.Operand, ctx)
	case ast.ExprSlice:
		d := expr.Data.(*ast.SliceExpr)
		c.walkExpr(&d.Receiver, ctx)
		if d.Low != nil {
			c.walkExpr(d.Low, ctx)
		}
		if d.High != nil {
			c.walkExpr(d.High, ctx)
		}
	case ast.ExprTry:
		d := expr.Data.(*ast.TryExpr)
		c.walkExpr(&d.Operand, ctx)
	case ast.ExprMatch:
		d := expr.Data.(*ast.MatchStmt) // match expressions reuse MatchStmt
		c.walkExpr(&d.Value, ctx)
		for i := range d.Arms {
			c.walkPattern(&d.Arms[i].Pattern, ctx)
			for j := range d.Arms[i].Patterns {
				c.walkPattern(&d.Arms[i].Patterns[j], ctx)
			}
			if d.Arms[i].Guard != nil {
				c.walkExpr(d.Arms[i].Guard, ctx)
			}
			c.walkBlock(&d.Arms[i].Body, ctx)
		}
	}
}

// validateAllTypesResolved checks that no function parameter or return type
// is TyUnknown after type checking. TyUnknown must not survive past the checker.
func (c *Checker) validateAllTypesResolved(file *ast.File) {
	for _, block := range file.Blocks {
		for _, fn := range block.Functions {
			// Skip generic functions — their type vars are resolved at instantiation
			if len(fn.TypeParams) > 0 {
				continue
			}
			fnType := c.scope.Lookup(fn.Name)
			if fnType == nil {
				// External methods are registered under receiver.method
				if fn.ReceiverType != "" {
					fnType = c.scope.Lookup(fn.ReceiverType + "." + fn.Name)
				}
				if fnType == nil {
					continue
				}
			}
			if fnType.Kind == TyFunc {
				for i, p := range fnType.Params {
					if p.Kind == TyUnknown {
						paramName := ""
						if i < len(fn.Params) {
							paramName = fn.Params[i].Name
						}
						panic(fmt.Sprintf("checker: function %s: parameter %q has TyUnknown type", fn.Name, paramName))
					}
				}
				if fnType.Return != nil && fnType.Return.Kind == TyUnknown {
					panic(fmt.Sprintf("checker: function %s: return type is TyUnknown", fn.Name))
				}
			}
		}
	}
}

// registerForgeBlock registers all types, functions, constants, and imports
// from a block into scope. Called across all blocks before checking bodies,
// so cross-block references resolve correctly in multi-file compilation.
func (c *Checker) registerForgeBlock(block *ast.ForgeBlock) {
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

	// Register impl block methods on concrete classes so destructor bodies
	// can resolve method calls like self.children() on concrete types.
	for _, impl := range block.ImplBlocks {
		c.registerImplMethods(&impl)
	}

	// Register import aliases in scope (packages are opaque types)
	for _, imp := range block.Imports {
		if strings.HasSuffix(imp.Path, ".fg") {
			// Forge module import — parse and check the imported file
			exports := c.CheckModuleFile(imp.Path, imp.Span)
			if exports != nil {
				// Register as a module type with exports attached
				modType := &Type{Kind: TyModule, Name: imp.Alias}
				modType.Fields = make([]TypeField, 0)
				// Store exports in registry under the alias for lookup
				modInfo := &TypeInfo{
					Type:    modType,
					Fields:  make(map[string]*Type),
					Methods: make(map[string]*Type),
				}
				for name, ti := range exports.Types {
					modInfo.Fields[name] = ti.Type
				}
				for name, ft := range exports.Functions {
					modInfo.Methods[name] = ft
				}
				c.registry.Register(imp.Alias, modInfo)
				c.scope.Define(imp.Alias, modType)
			}
		} else {
			c.scope.Define(imp.Alias, &Type{Kind: TyModule, Name: imp.Path})
		}
	}

	// Register type aliases
	for i := range block.TypeAliases {
		ta := &block.TypeAliases[i]
		resolved := c.resolveTypeExpr(&ta.Type)
		c.registry.Register(ta.Name, &TypeInfo{Type: resolved})
		c.scope.Define(ta.Name, resolved)
	}

	// Register top-level constants
	for i := range block.Constants {
		con := &block.Constants[i]
		var typ *Type
		if con.Type != nil {
			typ = c.resolveTypeExpr(con.Type)
			c.checkExpr(&con.Value) // ensure Value gets ResolvedType
		} else {
			typ = c.checkExpr(&con.Value)
		}
		c.scope.Define(con.Name, typ)
	}

	// Register functions in scope
	for i := range block.Functions {
		c.registerFunc(&block.Functions[i])
	}
}

// checkForgeBlockBodies checks function and method bodies in a block.
// Called after all blocks have been registered, so cross-block references work.
func (c *Checker) checkForgeBlockBodies(block *ast.ForgeBlock) {
	// Check function bodies
	for i := range block.Functions {
		if block.Functions[i].Body != nil {
			fn := &block.Functions[i]
			// For func T.method(self) syntax, define 'self' as the receiver type
			if fn.ReceiverType != "" {
				c.scope = NewScope(c.scope)
				if recvInfo := c.registry.Lookup(fn.ReceiverType); recvInfo != nil {
					c.scope.Define("self", recvInfo.Type)
				}
				c.checkFuncBody(fn)
				c.scope = c.scope.parent
			} else {
				c.checkFuncBody(fn)
			}
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
		Type:          &Type{Kind: TyStruct, Name: s.Name},
		Fields:        make(map[string]*Type),
		GuardedFields: make(map[string]string),
	}
	for _, tp := range s.TypeParams {
		info.TypeParamNames = append(info.TypeParamNames, tp.Name)
	}
	for _, f := range s.Fields {
		info.Fields[f.Name] = c.resolveTypeExpr(&f.Type)
		info.FieldOrder = append(info.FieldOrder, f.Name)
		if f.GuardedBy != "" {
			info.GuardedFields[f.Name] = f.GuardedBy
		}
		if f.Default != nil {
			c.checkExpr(f.Default)
		}
	}
	c.registry.Register(s.Name, info)
}

func (c *Checker) registerClass(cls *ast.ClassDecl) {
	info := &TypeInfo{
		Type:          &Type{Kind: TyClass, Name: cls.Name},
		Fields:        make(map[string]*Type),
		Methods:       make(map[string]*Type),
		GuardedFields: make(map[string]string),
	}
	// Record type param names for generic substitution
	for _, tp := range cls.TypeParams {
		info.TypeParamNames = append(info.TypeParamNames, tp.Name)
	}
	// Register type params as type variables in a temporary scope for resolving field/method types
	c.pushScope()
	for _, tp := range cls.TypeParams {
		c.scope.Define(tp.Name, &Type{Kind: TyVar, Name: tp.Name})
	}
	for _, f := range cls.Fields {
		info.Fields[f.Name] = c.resolveTypeExpr(&f.Type)
		info.FieldOrder = append(info.FieldOrder, f.Name)
		if f.GuardedBy != "" {
			info.GuardedFields[f.Name] = f.GuardedBy
		}
		if f.Default != nil {
			c.checkExpr(f.Default)
		}
	}
	for _, m := range cls.Methods {
		info.Methods[m.Name] = c.funcDeclToType(&m)
	}
	c.popScope()
	c.registry.Register(cls.Name, info)

	// Check for explicit constructor: func ClassName(self, ...)
	var ctorParams []*Type
	var hasExplicitCtor bool
	for _, m := range cls.Methods {
		if m.Name == cls.Name {
			hasExplicitCtor = true
			for _, p := range m.Params {
				if !p.IsSelf {
					ctorParams = append(ctorParams, c.resolveTypeExpr(&p.Type))
				}
			}
			break
		}
	}

	var typeParamNames []string
	var typeParamConstraints []string
	for _, tp := range cls.TypeParams {
		typeParamNames = append(typeParamNames, tp.Name)
		typeParamConstraints = append(typeParamConstraints, tp.Constraint)
	}

	if hasExplicitCtor {
		// Explicit constructor: define class name as a function
		c.scope.Define(cls.Name, &Type{
			Kind:                 TyFunc,
			Name:                 cls.Name,
			Params:               ctorParams,
			Return:               info.Type,
			TypeParamNames:       typeParamNames,
			TypeParamConstraints: typeParamConstraints,
		})
	} else {
		// No explicit constructor: define as type for struct-literal construction
		c.scope.Define(cls.Name, &Type{
			Kind:                 TyFunc,
			Name:                 cls.Name,
			Return:               info.Type,
			TypeParamNames:       typeParamNames,
			TypeParamConstraints: typeParamConstraints,
		})
	}
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

// registerImplMethods processes an impl block and registers the interface's methods
// on the concrete classes. This enables the checker to resolve method calls like
// self.children() in desugared destructor bodies.
func (c *Checker) registerImplMethods(impl *ast.ImplBlock) {
	ifaceInfo := c.registry.Lookup(impl.InterfaceName)
	if ifaceInfo == nil {
		return
	}
	ifaceDecl := c.ifaceDecls[impl.InterfaceName]
	if ifaceDecl == nil {
		return
	}

	// Build type param → concrete type substitution map
	subst := make(map[string]*Type)
	for i, tp := range ifaceDecl.TypeParams {
		if i < len(impl.TypeArgs) {
			subst[tp.Name] = c.resolveTypeExpr(&impl.TypeArgs[i])
		}
	}

	// For each mapping, resolve which concrete class gets the method,
	// substitute type params in the method signature, and register it.
	for _, m := range impl.Mappings {
		// Find the interface method type
		methType, ok := ifaceInfo.Methods[m.MethodName]
		if !ok {
			continue
		}

		// Determine which concrete class this method belongs to
		className := m.TargetClass
		if className == "" {
			// For inline impls, resolve from type param
			if ct, ok := subst[m.TypeParam]; ok {
				className = ct.Name
			}
		}
		if className == "" {
			continue
		}

		classInfo := c.registry.Lookup(className)
		if classInfo == nil || classInfo.Methods == nil {
			continue
		}

		substituted := c.substituteMethodType(methType, subst)

		// Only add if not already defined (don't override explicit methods)
		if _, exists := classInfo.Methods[m.MethodName]; !exists {
			classInfo.Methods[m.MethodName] = substituted
		}

		// For field-bind mappings with label prefixes (e.g., children → fb_children),
		// also register the label-prefixed method name. This is needed because a class
		// can have multiple relations of the same interface type (e.g., ForgeBlock has
		// many ArrayList relations), and the label prefix disambiguates them.
		if m.Kind == ast.ImplFieldBind && m.TargetMember != "" && m.TargetMember != m.MethodName {
			if strings.HasPrefix(m.MethodName, "set_") {
				// Setter: set_children → set_fb_children
				prefixedName := "set_" + m.TargetMember
				if _, exists := classInfo.Methods[prefixedName]; !exists {
					classInfo.Methods[prefixedName] = substituted
				}
			} else {
				// Getter: children → fb_children
				if _, exists := classInfo.Methods[m.TargetMember]; !exists {
					classInfo.Methods[m.TargetMember] = substituted
				}
			}
		}
	}
}

// substituteMethodType replaces type variables in a function type with concrete types.
func (c *Checker) substituteMethodType(t *Type, subst map[string]*Type) *Type {
	if t == nil {
		return nil
	}
	if t.Kind == TyVar {
		if ct, ok := subst[t.Name]; ok {
			return ct
		}
		return t
	}
	if t.Kind == TyFunc {
		result := &Type{
			Kind:   TyFunc,
			Name:   t.Name,
			Return: c.substituteMethodType(t.Return, subst),
		}
		if t.Params != nil {
			result.Params = make([]*Type, len(t.Params))
			for i, p := range t.Params {
				result.Params[i] = c.substituteMethodType(p, subst)
			}
		}
		return result
	}
	if t.Kind == TyList {
		return &Type{Kind: TyList, Elem: c.substituteMethodType(t.Elem, subst)}
	}
	if t.Kind == TyOptional {
		return &Type{Kind: TyOptional, Elem: c.substituteMethodType(t.Elem, subst)}
	}
	return t
}

func (c *Checker) registerInterface(iface *ast.InterfaceDecl) {
	// Store the AST declaration for relational constraint resolution
	if c.ifaceDecls == nil {
		c.ifaceDecls = make(map[string]*ast.InterfaceDecl)
	}
	c.ifaceDecls[iface.Name] = iface

	info := &TypeInfo{
		Type:    &Type{Kind: TyInterface, Name: iface.Name},
		Methods: make(map[string]*Type),
	}

	// Build set of valid type param names for multi-class interface validation
	typeParamNames := make(map[string]bool)
	for _, tp := range iface.TypeParams {
		typeParamNames[tp.Name] = true
	}

	for _, m := range iface.Methods {
		// Validate ReceiverType if present (multi-class interface method)
		if m.ReceiverType != "" && !typeParamNames[m.ReceiverType] {
			c.error(m.Span, "receiver type %s is not a type parameter of interface %s", m.ReceiverType, iface.Name)
			continue
		}
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

	// For external methods (func T.method(self) syntax), also register
	// in the receiver type's Methods map so checkMethodCall can find them.
	if fn.ReceiverType != "" {
		if info := c.registry.Lookup(fn.ReceiverType); info != nil {
			if info.Methods == nil {
				info.Methods = make(map[string]*Type)
			}
			info.Methods[fn.Name] = fnType
		}
	}
}

// grantRelationalMethods populates typeVarMethods from a relational constraint.
// Given `where Graph<G, N, E>`, looks up the Graph interface and maps
// typed methods (func G.nodes, func N.out_edges, etc.) to the actual type var names.
func (c *Checker) grantRelationalMethods(ifaceName string, typeArgs []ast.TypeExpr) {
	iface := c.ifaceDecls[ifaceName]
	if iface == nil {
		return
	}

	// Build interface type param name → where clause type arg name mapping
	// e.g. for Graph<G,N,E> and where Graph<MyG, MyN, MyE>:
	//   iface param "G" → typeArg "MyG", etc.
	paramMap := make(map[string]string) // interface param name → function type var name
	for i, tp := range iface.TypeParams {
		if i < len(typeArgs) {
			if typeArgs[i].Kind == ast.TypeNamed {
				nt := typeArgs[i].Data.(ast.NamedType)
				paramMap[tp.Name] = nt.Name
			}
		}
	}

	if c.typeVarMethods == nil {
		c.typeVarMethods = make(map[string]map[string]*Type)
	}

	for _, m := range iface.Methods {
		if m.ReceiverType == "" {
			continue // free function, not a typed method
		}
		// Map interface type param to function type var
		typeVarName, ok := paramMap[m.ReceiverType]
		if !ok {
			continue
		}
		if c.typeVarMethods[typeVarName] == nil {
			c.typeVarMethods[typeVarName] = make(map[string]*Type)
		}
		// Build method type, substituting interface type params with function type vars
		methType := c.funcDeclToTypeWithSubst(&m, paramMap)
		c.typeVarMethods[typeVarName][m.Name] = methType
	}
}

// funcDeclToTypeWithSubst is like funcDeclToType but substitutes type param names.
func (c *Checker) funcDeclToTypeWithSubst(fn *ast.FuncDecl, subst map[string]string) *Type {
	var params []*Type
	for _, p := range fn.Params {
		if p.IsSelf {
			continue
		}
		t := c.resolveTypeExpr(&p.Type)
		t = c.substTypeVarNames(t, subst)
		params = append(params, t)
	}
	ret := TypeUnit
	if fn.ReturnType != nil {
		ret = c.resolveTypeExpr(fn.ReturnType)
		ret = c.substTypeVarNames(ret, subst)
	}
	return &Type{Kind: TyFunc, Params: params, Return: ret}
}

// substTypeVarNames replaces type variable names according to the substitution map.
func (c *Checker) substTypeVarNames(t *Type, subst map[string]string) *Type {
	if t == nil {
		return t
	}
	if t.Kind == TyVar {
		if newName, ok := subst[t.Name]; ok {
			return &Type{Kind: TyVar, Name: newName}
		}
	}
	// Recurse into compound types
	if t.Kind == TyList && t.Elem != nil {
		newElem := c.substTypeVarNames(t.Elem, subst)
		if newElem != t.Elem {
			return &Type{Kind: TyList, Elem: newElem}
		}
	}
	if t.Kind == TyOptional && t.Elem != nil {
		newElem := c.substTypeVarNames(t.Elem, subst)
		if newElem != t.Elem {
			return &Type{Kind: TyOptional, Elem: newElem}
		}
	}
	return t
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
	var typeParamConstraints []string
	for _, tp := range fn.TypeParams {
		typeParamNames = append(typeParamNames, tp.Name)
		typeParamConstraints = append(typeParamConstraints, tp.Constraint)
	}
	// Merge where clause constraints into type param constraints
	for _, wc := range fn.Where {
		for i, name := range typeParamNames {
			if name == wc.Variable {
				if typeParamConstraints[i] == "" {
					typeParamConstraints[i] = wc.Constraint
				}
				break
			}
		}
	}
	return &Type{Kind: TyFunc, Params: params, Return: ret, TypeParamNames: typeParamNames, TypeParamConstraints: typeParamConstraints}
}

func (c *Checker) checkFuncBody(fn *ast.FuncDecl) {
	c.pushScope()
	defer c.popScope()

	// Register type parameters as type variables
	for _, tp := range fn.TypeParams {
		c.scope.Define(tp.Name, &Type{Kind: TyVar, Name: tp.Name})
	}

	// Populate typeVarMethods from relational where clauses
	prevTypeVarMethods := c.typeVarMethods
	c.typeVarMethods = nil
	for _, wc := range fn.Where {
		if wc.Variable == "" && len(wc.TypeArgs) > 0 {
			// Bare relational constraint: where Graph<G, N, E>
			ifaceInfo := c.registry.Lookup(wc.Constraint)
			if ifaceInfo == nil || ifaceInfo.Type.Kind != TyInterface {
				continue
			}
			// Look up the interface declaration to find typed methods
			// Build a type param name → concrete type arg mapping
			// For now, we just need to know which methods go to which type vars
			c.grantRelationalMethods(wc.Constraint, wc.TypeArgs)
		}
	}
	defer func() { c.typeVarMethods = prevTypeVarMethods }()

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
	// If not found and contains a dot, try module-qualified lookup
	if info == nil && strings.Contains(sl.TypeName, ".") {
		parts := strings.SplitN(sl.TypeName, ".", 2)
		modAlias, typeName := parts[0], parts[1]
		if modInfo := c.registry.Lookup(modAlias); modInfo != nil && modInfo.Type.Kind == TyModule {
			if fieldType, ok := modInfo.Fields[typeName]; ok {
				// Register the type temporarily in the local registry for field checking
				info = &TypeInfo{
					Type:   fieldType,
					Fields: make(map[string]*Type),
				}
				// Look up the full type info from the module's exports
				absPath := ""
				if c.currentFile != "" {
					absPath = filepath.Join(filepath.Dir(c.currentFile), typeName)
				}
				_ = absPath
				// Find the module exports to get full field info
				for _, exports := range c.modules {
					if ti, ok := exports.Types[typeName]; ok {
						info = ti
						break
					}
				}
			}
		}
	}
	if info == nil {
		c.error(expr.Span, "undefined type %q", sl.TypeName)
		return TypeError
	}
	if info.Type.Kind != TyStruct && info.Type.Kind != TyClass {
		c.error(expr.Span, "%q is not a struct or class type", sl.TypeName)
		return TypeError
	}
	// Resolve positional fields to named fields using FieldOrder
	posIdx := 0
	for i, f := range sl.Fields {
		if f.Name == "" {
			if posIdx < len(info.FieldOrder) {
				sl.Fields[i].Name = info.FieldOrder[posIdx]
				posIdx++
			} else {
				c.error(expr.Span, "too many positional fields for struct %s", sl.TypeName)
				break
			}
		} else {
			// Named field encountered; stop positional tracking
			posIdx = len(info.FieldOrder) // prevent further positional
		}
	}
	for i := range sl.Fields {
		// Look up expected field type for enum variant disambiguation
		fieldType, ok := info.Fields[sl.Fields[i].Name]
		if !ok {
			lower := strings.ToLower(sl.Fields[i].Name[:1]) + sl.Fields[i].Name[1:]
			fieldType, ok = info.Fields[lower]
		}

		// If the expected field type is an enum, temporarily shadow variant constructors
		// in scope so that ambiguous variant names (e.g., "Tuple" in both TypeExprKind
		// and PatternKind) resolve to the correct enum.
		scopePushed := false
		if ok && fieldType.Kind == TyEnum && fieldType.Name != "" {
			enumInfo := c.registry.Lookup(fieldType.Name)
			if enumInfo != nil && len(enumInfo.Variants) > 0 {
				c.scope = NewScope(c.scope)
				scopePushed = true
				for vName, vi := range enumInfo.Variants {
					if len(vi.Fields) == 0 {
						c.scope.Define(vName, fieldType)
					} else {
						paramTypes := make([]*Type, len(vi.Fields))
						for j, f := range vi.Fields {
							paramTypes[j] = f.Type
						}
						c.scope.Define(vName, &Type{
							Kind:   TyFunc,
							Name:   vName,
							Params: paramTypes,
							Return: fieldType,
						})
					}
				}
			}
		}

		valType := c.checkExpr(&sl.Fields[i].Value)

		if scopePushed {
			c.scope = c.scope.parent
		}

		// Infer type for context-dependent literals (nil, empty slice/map)
		// when the expected field type is known. This prevents void* in C output.
		if ok && fieldType.Kind != TyError {
			if sl.Fields[i].Value.Kind == ast.ExprNil {
				valType = fieldType
				sl.Fields[i].Value.ResolvedType = fieldType
			} else if sl.Fields[i].Value.Kind == ast.ExprListLit {
				lit := sl.Fields[i].Value.Data.(*ast.ListLitExpr)
				if len(lit.Elems) == 0 {
					valType = fieldType
					sl.Fields[i].Value.ResolvedType = fieldType
				}
			} else if sl.Fields[i].Value.Kind == ast.ExprMapLit {
				lit := sl.Fields[i].Value.Data.(*ast.MapLitExpr)
				if len(lit.Entries) == 0 {
					valType = fieldType
					sl.Fields[i].Value.ResolvedType = fieldType
				}
			}
		}

		if ok {
			if !c.assignableTo(valType, fieldType) && valType.Kind != TyError && fieldType.Kind != TyError {
				c.error(expr.Span, "field %s: expected %s, got %s", sl.Fields[i].Name, fieldType, valType)
			}
		} else {
			c.error(expr.Span, "struct %s has no field %q", sl.TypeName, sl.Fields[i].Name)
		}
	}
	return info.Type
}
