// Package lir defines the Low-level Intermediate Representation for the Grok compiler.
// The LIR sits between the AST and backend-specific code generators. All semantic
// complexity is resolved during the AST→LIR lowering pass, so each backend is a
// simple syntax emitter with no semantic logic.
//
// Design principles:
//   - Structured control flow (if/while/for/switch), NOT basic blocks
//   - Flat expressions — no nesting, every subexpression gets a named temporary
//   - Fully monomorphized — no type variables
//   - All sugar resolved — no ?, no match-as-expression, no f-strings, no method syntax on primitives
package lir

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// LType represents a concrete type in the LIR. No type variables, no unresolved types.
type LType struct {
	Kind     LTypeKind
	Name     string     // for Struct, ClassHandle, TaggedUnion
	Elem     *LType     // for Slice, Optional, Channel
	Key      *LType     // for Map
	Fields   []LField   // for Struct, Tuple, TaggedUnion variants
	Params   []*LType   // for FuncPtr
	Return   *LType     // for FuncPtr
	Variants []LVariant // for TaggedUnion
	Bits     int        // for integer/float types (8,16,32,64; -1 for platform int/uint)
}

type LTypeKind int

const (
	LTyI8          LTypeKind = iota // signed integers
	LTyI16                          //
	LTyI32                          //
	LTyI64                          //
	LTyU8                           // unsigned integers
	LTyU16                          //
	LTyU32                          //
	LTyU64                          //
	LTyF32                          // floats
	LTyF64                          //
	LTyBool                         //
	LTyString                       //
	LTyUnit                         // void / no value
	LTyError                        // error interface
	LTyPlatformInt                  // Go's int (Bits=-1)
	LTyPlatformUint                 // Go's uint (Bits=-1)
	LTyStruct                       // value type, stack-allocated
	LTyClassHandle                  // uint32 index into class pool (future C backend)
	LTyTuple                        // anonymous struct with positional fields
	LTySlice                        // dynamic array
	LTyMap                          // hash table
	LTyChannel                      // typed channel
	LTyMutex                        // lock type
	LTyOptional                     // {has_value bool, value T}
	LTyTaggedUnion                  // {tag enum, union of variant structs}
	LTyFuncPtr                      // function pointer (for lowered lambdas)
	LTyErrorResult                  // {value T, err error} — lowered (T, error) tuple
	LTyAny                          // interface{} / any
)

// LField is a named field within a struct, tuple, or variant.
type LField struct {
	Name string
	Type *LType
}

// LVariant is a single variant in a tagged union (lowered enum).
type LVariant struct {
	Name   string
	Tag    int      // integer tag value
	Fields []LField // empty for unit variants
}

// ---------------------------------------------------------------------------
// Values (Operands)
// ---------------------------------------------------------------------------

// LValue is an operand — a reference to a variable, temporary, or literal.
// LIR expressions are flat; they reference only LValues, never nested expressions.
type LValue struct {
	Kind     LValueKind
	Name     string  // for Var, Global
	TempID   int     // for Temp
	IntVal   int64   // for LitInt
	UintVal  uint64  // for LitUint
	FloatVal float64 // for LitFloat
	StrVal   string  // for LitString
	BoolVal  bool    // for LitBool
	Type     *LType  // type of this value (always set)
}

type LValueKind int

const (
	LValVar      LValueKind = iota // named variable (local)
	LValTemp                       // SSA-like temporary (%0, %1, ...)
	LValGlobal                     // module-level variable
	LValLitInt                     // integer literal
	LValLitUint                    // unsigned integer literal
	LValLitFloat                   // float literal
	LValLitString                  // string literal
	LValLitBool                    // boolean literal
	LValLitNull                    // null/nil/zero value
)

// ---------------------------------------------------------------------------
// Expressions (right-hand sides)
// ---------------------------------------------------------------------------

// LExpr is a right-hand side expression, bound to a temporary via LTempDef.
// Expressions reference only LValue operands, never other expressions.
type LExpr struct {
	Kind LExprKind
	Type *LType      // result type
	Data interface{} // kind-specific data (see L*Data types below)
}

type LExprKind int

const (
	// Arithmetic & logic
	LExprBinOp LExprKind = iota
	LExprUnOp
	LExprCast

	// Data access
	LExprStructField
	LExprClassGet
	LExprIndexGet
	LExprSlice

	// Calls
	LExprCall
	LExprBuiltin

	// Construction
	LExprStructLit
	LExprClassAlloc
	LExprMakeSlice
	LExprMakeMap
	LExprMakeChannel

	// Optional / tagged union
	LExprWrapOptional
	LExprUnwrapOptional
	LExprIsNull
	LExprVariantConstruct
	LExprVariantTag
	LExprVariantData

	// Error result
	LExprExtractValue
	LExprExtractError
	LExprMakeResult

	// Environment / closures
	LExprEnvGet
	LExprFuncRef

	// Format
	LExprFormat
)

// --- Expression data types ---

type LBinOpKind int

const (
	LBinAdd LBinOpKind = iota
	LBinSub
	LBinMul
	LBinDiv
	LBinMod
	LBinEq
	LBinNe
	LBinLt
	LBinLe
	LBinGt
	LBinGe
	LBinAnd
	LBinOr
	LBinBitAnd
	LBinBitOr
	LBinBitXor
	LBinShl
	LBinShr
)

type LUnOpKind int

const (
	LUnNeg    LUnOpKind = iota // -x
	LUnNot                     // !x
	LUnBitNot                  // ~x
)

type LBinOpData struct {
	Op    LBinOpKind
	Left  LValue
	Right LValue
}

type LUnOpData struct {
	Op      LUnOpKind
	Operand LValue
}

type LCastData struct {
	Target  *LType
	Operand LValue
}

type LStructFieldData struct {
	Receiver LValue
	Field    string
}

type LClassGetData struct {
	Handle LValue
	Class  string
	Field  string
}

type LIndexGetData struct {
	Collection LValue
	Index      LValue
}

type LSliceData struct {
	Collection LValue
	Low        *LValue // nil = from start
	High       *LValue // nil = to end
}

type LCallData struct {
	Func string
	Args []LValue
}

type LBuiltinData struct {
	Name string // "len", "append", "println", "print", "printf"
	Args []LValue
}

type LStructLitData struct {
	Fields []LFieldInit
}

type LFieldInit struct {
	Name  string
	Value LValue
}

type LClassAllocData struct {
	Class string
}

type LMakeChannelData struct {
	ElemType *LType
	BufSize  *LValue // nil for unbuffered
}

type LWrapOptionalData struct {
	Value LValue
}

type LUnwrapOptionalData struct {
	Value LValue
}

type LIsNullData struct {
	Value LValue
}

type LVariantConstructData struct {
	Enum    string // enum name
	Variant string // variant name
	Tag     int
	Fields  []LValue
}

type LVariantTagData struct {
	Value LValue
}

type LVariantDataData struct {
	Value   LValue
	Variant string
	Field   string
}

type LExtractValueData struct {
	Value LValue
}

type LExtractErrorData struct {
	Value LValue
}

type LMakeResultData struct {
	Value LValue
	Err   LValue
}

type LEnvGetData struct {
	Env   LValue
	Field string
}

type LFuncRefData struct {
	Name string
	Env  *LValue // nil for non-closure functions
}

type LFormatPart struct {
	IsLiteral bool
	Text      string // if IsLiteral
	Value     LValue // if !IsLiteral
	Format    string // format specifier (e.g., "%d", "%s", "%v")
}

type LFormatData struct {
	Parts []LFormatPart
}

// ---------------------------------------------------------------------------
// Statements
// ---------------------------------------------------------------------------

// LStmt is a statement in the LIR. Statements form a structured tree.
type LStmt struct {
	Kind LStmtKind
	Data interface{}
}

type LStmtKind int

const (
	// Variable management
	LStmtTempDef   LStmtKind = iota // %n = expr
	LStmtVarDecl                     // var name: type = value
	LStmtAssign                      // name = value
	LStmtStructSet                   // target.field = value (value-type struct)
	LStmtClassSet                    // ClassName_set_field(handle, value)
	LStmtIndexSet                    // collection[index] = value

	// Control flow (STRUCTURED)
	LStmtIf       // if cond { then } else { else }
	LStmtWhile    // while { condBlock; if !condVar break } body
	LStmtFor      // for var, index in collection { body }
	LStmtSwitch   // switch tag { case 0: ... }
	LStmtBlock    // { stmts... }
	LStmtReturn   // return value(s)
	LStmtBreak    //
	LStmtContinue //

	// Side effects
	LStmtExpr  // evaluate expression, discard result
	LStmtDefer // defer { body }

	// Concurrency
	LStmtSpawn  // spawn { body }
	LStmtLock   // lock(mutex) { body }
	LStmtSend   // channel <- value
	LStmtSelect // select { case... }
)

// --- Statement data types ---

// LTempDef: %n = expr — SSA temporary, assigned exactly once.
type LTempDef struct {
	ID   int
	Expr LExpr
}

// LVarDecl: var name: type [= value]
type LVarDecl struct {
	Name    string
	Type    *LType
	Init    *LValue // nil if uninitialized
	Mutable bool
}

// LAssign: target = value (target is a variable name or field path)
type LAssign struct {
	Target string
	Value  LValue
}

// LStructSet: receiver.field = value
type LStructSet struct {
	Receiver LValue
	Field    string
	Value    LValue
}

// LClassSet: ClassName_set_field(handle, value)
type LClassSet struct {
	Handle LValue
	Class  string
	Field  string
	Value  LValue
}

// LIndexSet: collection[index] = value
type LIndexSet struct {
	Collection LValue
	Index      LValue
	Value      LValue
}

// LIf: if cond { then } [else { else }]
type LIf struct {
	Cond LValue
	Then []LStmt
	Else []LStmt
}

// LWhile: condition is a block of statements ending in a TempDef.
// This solves Open Question #3: re-evaluation of complex conditions.
type LWhile struct {
	CondBlock []LStmt // statements that compute the condition
	CondVar   LValue  // the bool result to test
	Body      []LStmt
}

// LFor: for var [, indexVar] in collection { body }
type LFor struct {
	Var        string
	VarType    *LType
	IndexVar   string // empty if no index
	Collection LValue
	Body       []LStmt
}

// LSwitch: switch tag { case 0: ... case 1: ... }
type LSwitch struct {
	Tag   LValue
	Cases []LSwitchCase
}

// LSwitchCase: a single case in a switch.
type LSwitchCase struct {
	Tag     int    // integer tag value (-1 for default)
	Binding string // variable name bound to variant data (empty if none)
	Body    []LStmt
}

// LReturn: return val1, val2, ...
type LReturn struct {
	Values []LValue
}

// LBlock: { stmts... } — scoped block
type LBlock struct {
	Stmts []LStmt
}

// LExprStmt: evaluate expression, discard result (wraps a TempDef ID)
type LExprStmt struct {
	TempID int
}

// LDefer: defer { body }
type LDefer struct {
	Body []LStmt
}

// LLock: lock(mutex) { body }
type LLock struct {
	Mutex LValue
	Body  []LStmt
}

// LSpawn: spawn { body }
type LSpawn struct {
	Body     []LStmt
	Captures []string // variables captured from enclosing scope
}

// LSend: channel <- value
type LSend struct {
	Channel LValue
	Value   LValue
}

type LSelectKind int

const (
	LSelectRecv    LSelectKind = iota
	LSelectSend
	LSelectDefault
)

// LSelect: select { case... }
type LSelect struct {
	Cases []LSelectCase
}

// LSelectCase: a single case in a select.
type LSelectCase struct {
	Kind    LSelectKind
	Channel LValue
	Value   LValue // for Send: value to send; for Recv: received value
	Binding string // for Recv: variable name
	Body    []LStmt
}

// ---------------------------------------------------------------------------
// Top-Level Declarations
// ---------------------------------------------------------------------------

// LProgram is the root of a lowered program.
type LProgram struct {
	Package   string
	Imports   []LImport
	Structs   []LStructDecl
	Classes   []LClassDecl
	Enums     []LEnumDecl
	Functions []LFuncDecl
	Globals   []LVarDecl
	TypeDefs  []LTypeDef // type aliases
}

// LImport is a single import declaration.
type LImport struct {
	Alias string
	Path  string
}

// LStructDecl: a value-type struct.
type LStructDecl struct {
	Name       string
	Fields     []LField
	IsExported bool
}

// LClassDecl: a heap-allocated class with accessor-based field access.
type LClassDecl struct {
	Name      string
	Fields    []LField
	GuardedBy map[string]string // field → lock name
	HasFinal  bool
	IsExported bool
}

// LEnumDecl: a tagged union.
type LEnumDecl struct {
	Name       string
	Variants   []LVariant
	IsExported bool
}

// LFuncDecl: a function or method.
type LFuncDecl struct {
	Name       string
	Params     []LParam
	ReturnType *LType
	Body       []LStmt
	IsExported bool
	Receiver   string // non-empty for methods: the receiver type name
}

// LParam: a function parameter.
type LParam struct {
	Name string
	Type *LType
}

// LTypeDef: type Name = Type (type alias)
type LTypeDef struct {
	Name       string
	Type       *LType
	IsExported bool
}
