// Package ast defines the AST node types for the Grok language.
// Both .grok (declaration-only) and .gk (full code) files parse into these types.
package ast

// Pos is a position in source code.
type Pos struct {
	File   string
	Line   int
	Column int
}

// Span is a range in source code.
type Span struct {
	Start Pos
	End   Pos
}

// TypeExpr represents any type expression in Grok.
type TypeExpr struct {
	Kind TypeExprKind
	Data any // one of NamedType, OptionalType, UnionType, SequenceType, MapType, TupleType, FuncType, ChannelType, or nil for Lock/Unit
	Span Span
}

type TypeExprKind int

const (
	TypeNamed    TypeExprKind = iota // T, Stack<T>, map[K]V
	TypeOptional                     // T?
	TypeUnion                        // T | U
	TypeSequence                     // [T]
	TypeMap                          // map[K]V
	TypeTuple                        // (T, U) or (x: T, y: U)
	TypeFunc                         // T -> U
	TypeChannel                      // channel<T>
	TypeLock                         // lock
	TypeUnit                         // unit
)

// NamedType holds the name and optional type arguments for TypeNamed.
type NamedType struct {
	Name string
	Args []TypeExpr
}

// OptionalType wraps the inner type for T?.
type OptionalType struct {
	Inner TypeExpr
}

// UnionType holds variants for T | U.
type UnionType struct {
	Variants []TypeExpr
}

// SequenceType holds the element type for [T].
type SequenceType struct {
	Elem TypeExpr
}

// MapType holds key and value types for map[K]V.
type MapType struct {
	Key   TypeExpr
	Value TypeExpr
}

// TupleField is a single element in a tuple type.
type TupleField struct {
	Name string // empty for positional
	Type TypeExpr
}

// TupleType holds fields for (T, U) or (x: T, y: U).
type TupleType struct {
	Fields []TupleField
}

// FuncType holds parameter and return types for T -> U.
type FuncType struct {
	Params []TypeExpr
	Return TypeExpr
}

// ChannelType holds the element type for channel<T>.
type ChannelType struct {
	Elem TypeExpr
}

// TypeParam is a type parameter declaration like T or T: Comparable.
type TypeParam struct {
	Name       string
	Constraint string // empty if unconstrained
	Span       Span
}

// Param is a function parameter.
type Param struct {
	Name   string
	Type   TypeExpr
	IsMut  bool // true for `mut self`
	IsSelf bool
	Span   Span
}

// WhereClause constrains a type variable: where T: Integer.
type WhereClause struct {
	Variable   string
	Constraint string
	Span       Span
}

// Annotations holds all function/method annotations.
type Annotations struct {
	Why          string
	Concurrent   *bool // nil = not specified
	RequiresLock []string
	ExcludesLock []string
	Raises       []string
	Requires     []string // preconditions
	Ensures      []string // postconditions
	Spawns       bool
	Pure         bool
}

// FuncDecl is a function or method declaration.
type FuncDecl struct {
	Name       string
	IsPublic   bool // true if declared with `pub`
	TypeParams []TypeParam
	Params     []Param
	ReturnType *TypeExpr // nil for missing (error in .grok)
	Where      []WhereClause
	Annotations Annotations
	// Body is nil in .grok files, holds the function body in .gk files.
	Body *Block
	Span Span
}

// Field is a struct or class field.
type Field struct {
	Name      string
	Type      TypeExpr
	GuardedBy string // empty if not guarded
	Why       string
	Span      Span
}

// ClassDecl is a class declaration.
type ClassDecl struct {
	Name       string
	IsPublic   bool // true if declared with `pub`
	TypeParams []TypeParam
	CtorParams []Param
	Implements []string
	Fields     []Field
	Methods    []FuncDecl
	Why        string
	Source     []string
	Span       Span
}

// StructDecl is a struct (named tuple) declaration.
type StructDecl struct {
	Name       string
	IsPublic   bool // true if declared with `pub`
	TypeParams []TypeParam
	Fields     []Field
	Why        string
	Span       Span
}

// EnumVariant is a single variant in an enum.
type EnumVariant struct {
	Name   string
	Fields []TupleField
	Span   Span
}

// EnumDecl is an enum (sum type) declaration.
type EnumDecl struct {
	Name       string
	IsPublic   bool // true if declared with `pub`
	TypeParams []TypeParam
	Variants   []EnumVariant
	Why        string
	Span       Span
}

// InterfaceDecl is an interface declaration.
type InterfaceDecl struct {
	Name       string
	IsPublic   bool // true if declared with `pub`
	TypeParams []TypeParam
	Implements []string // composed interfaces
	Methods    []FuncDecl
	Why        string
	Span       Span
}

// RelationKind distinguishes owns from refs.
type RelationKind int

const (
	Owns RelationKind = iota
	Refs
)

// RelationSide is one side of a relation.
type RelationSide struct {
	TypeName string
	Label    string // empty if unlabeled
}

// RelationDecl is a relation declaration.
type RelationDecl struct {
	Hint   string // DoublyLinked, ArrayList, Hashed, or empty
	Parent RelationSide
	Kind   RelationKind
	Child  RelationSide
	IsMany bool // [Child] vs Child
	Span   Span
}

// DocBlock is a named narrative section.
type DocBlock struct {
	Section string
	Content string
	Span    Span
}

// InvariantDecl is an invariant claim.
type InvariantDecl struct {
	Claim      string
	VerifiedAt string // commit hash, or empty
	Span       Span
}

// ImportDecl is an import statement.
type ImportDecl struct {
	Alias string
	Path  string
	Span  Span
}

// TypeAliasDecl is a type alias: type Name = OtherType
type TypeAliasDecl struct {
	Name     string
	IsPublic bool // true if declared with `pub`
	Type     TypeExpr
	Span Span
}

// GrokBlock is the top-level grok { ... } block.
type GrokBlock struct {
	Name        string
	Why         string
	Imports     []ImportDecl
	Docs        []DocBlock
	Invariants  []InvariantDecl
	Structs     []StructDecl
	Enums       []EnumDecl
	Interfaces  []InterfaceDecl
	Classes     []ClassDecl
	Functions   []FuncDecl
	Relations   []RelationDecl
	TypeAliases []TypeAliasDecl
	Source      []string
	Fake        string
	Span        Span
}

// Comment represents a line comment in the source.
type Comment struct {
	Text string // full text including "// " prefix
	Pos  Pos    // position of the "//"
}

// File is the top-level AST node.
type File struct {
	Filename string
	Blocks   []GrokBlock
	Comments []Comment // all comments in the file, ordered by position
	Span     Span
}

// DocComment returns the contiguous block of comments immediately preceding
// the given line, or nil if there are none.
func (f *File) DocComment(line int) []Comment {
	// Find comments ending at line-1
	var result []Comment
	for i := len(f.Comments) - 1; i >= 0; i-- {
		c := f.Comments[i]
		if c.Pos.Line == line-1-len(result) {
			result = append([]Comment{c}, result...)
		} else if c.Pos.Line < line-1-len(result) {
			break
		}
	}
	return result
}
