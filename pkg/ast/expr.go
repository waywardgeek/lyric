package ast

// --- Expressions ---

// ExprKind discriminates expression types.
type ExprKind int

const (
	ExprIdent     ExprKind = iota // variable/function reference
	ExprIntLit                    // integer literal
	ExprFloatLit                  // float literal
	ExprStringLit                 // string literal
	ExprStringInterp              // f"hello {name}" — interpolated string
	ExprBoolLit                   // true/false
	ExprNil                       // nil
	ExprCall                      // f(x, y)
	ExprMethodCall                // obj.method(x, y)
	ExprFieldAccess               // obj.field
	ExprIndex                     // xs[i]
	ExprUnary                     // -x, !x
	ExprBinary                    // x + y, x && y
	ExprTupleLit                  // (x, y)
	ExprListLit                   // [1, 2, 3]
	ExprMapLit                    // map[K]V{k1: v1, k2: v2}
	ExprLambda                    // (x: T) -> x + 1
	ExprMatch                     // match value { ... } as expression
	ExprStructLit                 // Point{X: 3.0, Y: 4.0}
	ExprCast                      // <i64>x — type cast
	ExprUnwrap                    // x! — unwrap optional, panic if nil
	ExprSlice                     // xs[start:end] — slice expression
)

// Expr is any expression node.
type Expr struct {
	Kind         ExprKind
	Data         any  // one of the *Lit, *CallExpr, etc. below
	Span         Span
	ResolvedType any  // set by checker: *checker.Type (avoids import cycle via any)
}

type IdentExpr struct {
	Name string
}

type IntLitExpr struct {
	Value string // keep as string to support i256
}

type FloatLitExpr struct {
	Value string
}

type StringLitExpr struct {
	Value string
}

// StringInterpExpr represents f"hello {name}, you are {age}"
// Parts alternates: string, expr, string, expr, string
// Parts always starts and ends with a string (may be empty).
type StringInterpExpr struct {
	Parts []Expr // alternating ExprStringLit and other expressions
}

type BoolLitExpr struct {
	Value bool
}

type CallExpr struct {
	Func              Expr
	TypeArgs          []TypeExpr // explicit type arguments, e.g. f<int>(x)
	InferredTypeArgs  []any      // set by checker: []*checker.Type (avoids import cycle via any)
	Args              []Expr
}

type MethodCallExpr struct {
	Receiver Expr
	Method   string
	TypeArgs []TypeExpr
	Args     []Expr
}

type FieldAccessExpr struct {
	Receiver Expr
	Field    string
}

type IndexExpr struct {
	Receiver Expr
	Index    Expr
}

type SliceExpr struct {
	Receiver Expr
	Low      *Expr // nil = from start
	High     *Expr // nil = to end
}

type UnaryOp int

const (
	OpNeg UnaryOp = iota // -
	OpNot                // !
)

type UnaryExpr struct {
	Op      UnaryOp
	Operand Expr
}

type BinaryOp int

const (
	OpAdd BinaryOp = iota // +
	OpSub                 // -
	OpMul                 // *
	OpDiv                 // /
	OpMod                 // %
	OpEq                  // ==
	OpNeq                 // !=
	OpLt                  // <
	OpLe                  // <=
	OpGt                  // >
	OpGe                  // >=
	OpAnd                 // &&
	OpOr                  // ||
	OpBitAnd              // &
	OpBitOr               // |
	OpBitXor              // ^
	OpShl                 // <<
	OpShr                 // >>
)

type BinaryExpr struct {
	Left  Expr
	Op    BinaryOp
	Right Expr
}

type TupleLitExpr struct {
	Elems []Expr
}

type ListLitExpr struct {
	Elems []Expr
}

type MapEntry struct {
	Key   Expr
	Value Expr
}

type MapLitExpr struct {
	Entries []MapEntry
}

type StructLitField struct {
	Name  string
	Value Expr
}

type StructLitExpr struct {
	TypeName string
	Fields   []StructLitField
}

type LambdaExpr struct {
	Params     []Param
	ReturnType *TypeExpr
	Body       *Block // single expression or block
}

// CastExpr represents <TargetType>expr — explicit type conversion.
type CastExpr struct {
	TargetType TypeExpr
	Operand    Expr
}

// UnwrapExpr represents expr! — unwrap an optional value, panic if nil.
type UnwrapExpr struct {
	Operand Expr
}

// --- Statements ---

// StmtKind discriminates statement types.
type StmtKind int

const (
	StmtVarDecl  StmtKind = iota // let x: T = expr  or  let mut x: T = expr
	StmtAssign                   // x = expr
	StmtReturn                   // return expr
	StmtExpr                     // bare expression (function call, etc.)
	StmtIf                       // if/else if/else
	StmtFor                      // for item in collection
	StmtWhile                    // while condition
	StmtMatch                    // match value { ... }
	StmtBlock                    // { ... }
	StmtCascade                  // cascade { ... } (like Go defer)
	StmtBreak                    // break
	StmtContinue                 // continue
)

// Stmt is any statement node.
type Stmt struct {
	Kind StmtKind
	Data any // one of the *Stmt types below
	Span Span
}

// Block is a sequence of statements (function body, if branch, etc.).
type Block struct {
	Stmts []Stmt
	Span  Span
}

type VarDeclStmt struct {
	Name  string
	Names []string  // for tuple destructuring: let (a, b) = expr
	Type  *TypeExpr // nil if inferred
	IsMut bool
	Value *Expr // nil if uninitialized
}

type AssignStmt struct {
	Target Expr // ident, field access, or index
	Value  Expr
}

type ReturnStmt struct {
	Value *Expr // nil for bare return
}

type ExprStmt struct {
	Expr Expr
}

type IfStmt struct {
	Condition Expr
	Then      Block
	ElseIfs   []ElseIf
	Else      *Block // nil if no else
}

type ElseIf struct {
	Condition Expr
	Body      Block
	Span      Span
}

type ForStmt struct {
	Var        string
	IndexVar   string // optional: for i, x in xs — empty if not used
	Collection Expr
	Body       Block
}

type WhileStmt struct {
	Condition Expr
	Body      Block
}

type MatchStmt struct {
	Value Expr
	Arms  []MatchArm
}

type MatchArm struct {
	Pattern Pattern
	Guard   *Expr // optional: `if <expr>` guard clause
	Body    Block
	Span    Span
}

// --- Patterns ---

type PatternKind int

const (
	PatIdent    PatternKind = iota // x (binding)
	PatLiteral                     // 42, "hello", true
	PatVariant                     // Some(x), None
	PatWildcard                    // _
	PatTuple                       // (x, y)
)

type Pattern struct {
	Kind         PatternKind
	Data         any
	Span         Span
	ResolvedType any // set by checker for union type patterns
}

type IdentPattern struct {
	Name string
}

type LiteralPattern struct {
	Expr Expr
}

type VariantPattern struct {
	Name     string
	Bindings []Pattern
}

type TuplePattern struct {
	Elems []Pattern
}

type CascadeStmt struct {
	Body Block
}
