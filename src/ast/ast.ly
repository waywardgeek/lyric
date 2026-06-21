// ast.ly — AST type definitions for the Lyric bootstrap compiler
// Ported from pkg/ast/ast.go and pkg/ast/expr.go
//
// Design:
//   - Structs for small value types only (Pos, Span, RelationSide, StructLitField)
//   - Classes for all AST nodes (identity, heap-allocated)
//   - ArrayList relations for parent→child ownership
//   - Enums with associated data for tagged unions (ExprKind, StmtKind, etc.)
//   - Sym for all identifiers/names; string only for raw text (literals, file content)

lyric ast {

  // ---- Value types (structs) ----

  struct Pos {
    file: Sym?
    line: i32
    column: i32
  }

  struct Span {
    start: Pos
    end: Pos
  }

  // ---- Type expressions ----

  enum TypeExprKind {
    Named(name: Sym, args: [TypeExpr])
    Optional(inner: TypeExpr)
    Union(variants: [TypeExpr])
    Sequence(elem: TypeExpr)
    Map(key: TypeExpr, value: TypeExpr)
    Tuple(fields: [TupleField])
    Func(params: [TypeExpr], ret: TypeExpr)
    Channel(elem: TypeExpr)
    Generator(elem: TypeExpr)
    Lock
    Unit
  }

  class TypeExpr {
    kind: TypeExprKind
    span: Span
  }

  class TupleField {
    name: Sym?
    type_expr: TypeExpr?
  }

  // ---- Type parameters and constraints ----

  class TypeParam {
    name: Sym?
    constraint: Sym?
    span: Span
  }

  class WhereClause {
    variable: Sym?
    constraint: Sym?
    span: Span
  }
  relation ArrayList WhereClause:wc_arg owns [TypeExpr:wc_arg]

  // ---- Functions ----

  class Param {
    name: Sym?
    type_expr: TypeExpr?
    is_mut: bool
    is_self: bool
    span: Span
  }

  class FuncDecl {
    name: Sym?
    is_public: bool
    is_final: bool
    is_trusted: bool
    receiver_type: Sym?
    return_type: TypeExpr?
    body: Block?
    span: Span
  }
  relation ArrayList FuncDecl:fp owns [TypeParam:fp]
  relation ArrayList FuncDecl:param owns [Param:param]
  relation ArrayList FuncDecl:where owns [WhereClause:where]

  // ---- Fields ----

  class Field {
    name: Sym?
    is_public: bool
    type_expr: TypeExpr?
    default_value: Expr?
    guarded_by: Sym?
    span: Span
  }

  // ---- Declarations ----

  class ClassDecl {
    name: Sym?
    is_public: bool
    is_permanent: bool
    implements: [Sym]
    span: Span
  }
  relation ArrayList ClassDecl:ctp owns [TypeParam:ctp]
  relation ArrayList ClassDecl:cf owns [Field:cf]
  relation ArrayList ClassDecl:cm owns [FuncDecl:cm]
  relation ArrayList ClassDecl:cwc owns [WhereClause:cwc]

  class StructDecl {
    name: Sym?
    is_public: bool
    span: Span
  }
  relation ArrayList StructDecl:stp owns [TypeParam:stp]
  relation ArrayList StructDecl:sf owns [Field:sf]

  class EnumVariant {
    name: Sym?
    span: Span
  }
  relation ArrayList EnumVariant:evf owns [TupleField:evf]

  class EnumDecl {
    name: Sym?
    is_public: bool
    span: Span
  }
  relation ArrayList EnumDecl:etp owns [TypeParam:etp]
  relation ArrayList EnumDecl:ev owns [EnumVariant:ev]

  // ---- Interfaces ----

  class InterfaceFieldDecl {
    type_param: Sym?
    name: Sym?
    type_expr: TypeExpr?
    span: Span
  }

  class DestructorBlock {
    type_param: Sym?
    kind: RelationKind
    body: Block?
    span: Span
  }

  class InterfaceDecl {
    name: Sym?
    is_public: bool
    implements: [Sym]
    span: Span
  }
  relation ArrayList InterfaceDecl:itp owns [TypeParam:itp]
  relation ArrayList InterfaceDecl:im owns [FuncDecl:im]
  relation ArrayList InterfaceDecl:ifd owns [InterfaceFieldDecl:ifd]
  relation ArrayList InterfaceDecl:idb owns [DestructorBlock:idb]

  // ---- Impl blocks ----

  enum ImplMappingKind { Alias FieldBind Inline }

  class ImplMapping {
    type_param: Sym?
    method_name: Sym?
    kind: ImplMappingKind
    target_class: Sym?
    target_member: Sym?
    inline_func: FuncDecl?
    span: Span
  }

  class ImplBlock {
    interface_name: Sym?
    for_type: Sym?
    label: Sym?
    span: Span
  }
  relation ArrayList ImplBlock:ib_arg owns [TypeExpr:ib_arg]
  relation ArrayList ImplBlock:ibm owns [ImplMapping:ibm]

  // ---- Relations ----

  enum RelationKind { Owns Refs }

  struct RelationSide {
    type_name: Sym?
    type_args: [Sym]
    label: Sym?
  }

  class RelationDecl {
    hint: Sym?
    parent: RelationSide
    kind: RelationKind
    child: RelationSide
    is_many: bool
    span: Span
  }

  // ---- Other top-level declarations ----

  class ImportDecl {
    alias: Sym?
    path: string
    span: Span
  }

  class TypeAliasDecl {
    name: Sym?
    is_public: bool
    type_expr: TypeExpr?
    span: Span
  }

  class ConstDecl {
    name: Sym?
    is_public: bool
    type_expr: TypeExpr?
    value: Expr?
    span: Span
  }

  class Comment {
    text: string
    pos: Pos
  }

  // ---- Expressions ----

  enum UnaryOp { Neg Not }

  enum BinaryOp {
    Add Sub Mul Div Mod
    Eq Neq Lt Le Gt Ge
    And Or
    BitAnd BitOr BitXor Shl Shr
  }

  struct StructLitField {
    name: Sym?
    value: Expr?
  }

  enum ExprKind {
    Ident(name: Sym)
    IntLit(value: string, type_hint: Sym?)
    FloatLit(value: string)
    StringLit(value: string)
    StringInterp(parts: [Expr])
    BoolLit(value: bool)
    Nil
    Call(func_expr: Expr, type_args: [TypeExpr], args: [Expr], mut_args: [bool])
    MethodCall(receiver: Expr, method: Sym, type_args: [TypeExpr], args: [Expr], mut_args: [bool])
    FieldAccess(receiver: Expr, field_name: Sym)
    Index(receiver: Expr, index: Expr)
    Slice(receiver: Expr, low: Expr?, high: Expr?)
    Unary(op: UnaryOp, operand: Expr)
    Binary(left: Expr, op: BinaryOp, right: Expr)
    TupleLit(elems: [Expr])
    ListLit(elems: [Expr])
    MapLit(keys: [Expr], values: [Expr])
    StructLit(type_name: Sym, type_args: [TypeExpr], fields: [StructLitField])
    Lambda(params: [Param], return_type: TypeExpr?, body: Block)
    Match(value: Expr, arms: [MatchArm])
    Cast(target_type: TypeExpr, operand: Expr)
    Unwrap(operand: Expr)
    Try(operand: Expr)
    Is(operand: Expr, variant: Sym)
    IfElse(cond: Expr, then_block: Block, else_ifs: [ElseIf], else_block: Block)
  }

  permanent class Expr {
    kind: ExprKind
    span: Span
    resolved_type: TypeExpr?  // Set by checker — used by lowerer to determine expression types
    inferred_type_args: [TypeExpr]  // Set by checker for generic calls with inferred type args
  }

  // ---- Patterns ----

  enum PatternKind {
    Ident(name: Sym)
    Literal(expr: Expr)
    Variant(name: Sym, bindings: [Pattern])
    Wildcard
    Tuple(elems: [Pattern])
  }

  permanent class Pattern {
    kind: PatternKind
    span: Span
  }

  permanent class MatchArm {
    pattern: Pattern?
    patterns: [Pattern]  // additional alternative patterns (pat1 | pat2 | pat3)
    guard: Expr?
    body: Block?
    span: Span
  }

  // ---- Statements ----

  enum StmtKind {
    VarDecl(name: Sym, names: [Sym], type_expr: TypeExpr?, is_mut: bool, is_ref: bool, value: Expr?)
    Assign(target: Expr, value: Expr)
    Return(value: Expr?)
    ExprStmt(expr: Expr)
    If(condition: Expr, then_block: Block, else_ifs: [ElseIf], else_block: Block?)
    For(var_name: Sym, index_var: Sym?, collection: Expr, body: Block)
    While(condition: Expr, body: Block)
    Match(value: Expr, arms: [MatchArm])
    BlockStmt(block: Block)
    Break
    Continue
    Spawn(body: Block)
    Select(cases: [SelectCase])
    Yield(value: Expr?)
    Lock(mutex: Expr, body: Block)
    IfLet(pattern: Pattern, value: Expr, then_block: Block, else_block: Block?)
    LetElse(pattern: Pattern, value: Expr, else_block: Block)
    Ref(expr: Expr)
    Unref(expr: Expr)
  }

  class Stmt {
    kind: StmtKind
    span: Span
  }

  permanent class ElseIf {
    condition: Expr?
    body: Block?
    span: Span
  }

  permanent class SelectCase {
    is_default: bool
    bind_var: Sym?
    expr: Expr?
    body: Block?
    span: Span
  }

  permanent class Block {
    span: Span
  }
  relation ArrayList Block:bs owns [Stmt:bs]

  // ---- Top-level ----

  class LyricBlock {
    name: Sym?
    span: Span
  }
  relation ArrayList LyricBlock:imp owns [ImportDecl:imp]
  relation ArrayList LyricBlock:sd owns [StructDecl:sd]
  relation ArrayList LyricBlock:ed owns [EnumDecl:ed]
  relation ArrayList LyricBlock:id owns [InterfaceDecl:id]
  relation ArrayList LyricBlock:cd owns [ClassDecl:cd]
  relation ArrayList LyricBlock:fd owns [FuncDecl:fd]
  relation ArrayList LyricBlock:rd owns [RelationDecl:rd]
  relation ArrayList LyricBlock:ib owns [ImplBlock:ib]
  relation ArrayList LyricBlock:ta owns [TypeAliasDecl:ta]
  relation ArrayList LyricBlock:con owns [ConstDecl:con]

  permanent class File {
    filename: string
    span: Span
  }
  relation ArrayList File:fb owns [LyricBlock:fb]
  relation ArrayList File:fc owns [Comment:fc]



// --- merge_files / merge_stdlib / find_stdlib_dir ---

// merge_files merges multiple parsed AST files into a single file.
func merge_files(files: [File?]) -> File? {
    if len(files) == 0 {
        let empty = File {
            filename: "",
            span: Span {
                start: Pos { file: null, line: 0, column: 0 },
                end: Pos { file: null, line: 0, column: 0 }
            }
        }
        return empty
    }
    if len(files) == 1 {
        return files[0]
    }
    let merged = File {
        filename: files[0]!.filename,
        span: files[0]!.span
    }
    for i in range(0, len(files)) {
        let f = files[i]
        if f != null {
            let blocks = f!.fb_children()
            for j in range(0, len(blocks)) {
                let b = blocks[j]
                array_append<File, LyricBlock>(merged, b)
            }
            let comments = f!.fc_children()
            for j in range(0, len(comments)) {
                array_append<File, Comment>(merged, comments[j])
            }
        }
    }
    return merged
}

// find_stdlib_dir locates the stdlib directory.
func find_stdlib_dir() -> string {
    if file_exists("stdlib/std.ly") {
        return "stdlib"
    }
    if file_exists("../stdlib/std.ly") {
        return "../stdlib"
    }
    return ""
}

// load_stdlib loads and parses all .ly files from the stdlib directory.
func load_stdlib(dir: string) -> File? {
    let entries = list_dir(dir)
    let mut combined: File? = null
    for i in range(0, len(entries)) {
        let entry = entries[i]
        if str_has_suffix(entry, ".ly") {
            let path = f"{dir}/{entry}"
            let read_result = read_file(path)
            let src = read_result._0
            let result = parse_file(src, path)
            let file = result._0
            let err = result._1
            if file == null {
                eprintln(f"warning: failed to parse stdlib file {path}")
                continue
            }
            if combined == null {
                combined = file
            } else {
                let blocks = file!.fb_children()
                for j in range(0, len(blocks)) {
                    array_append<File, LyricBlock>(combined!, blocks[j])
                }
            }
        }
    }
    return combined
}

// --- primitive type check ---

func is_primitive_type(name: string) -> bool {
    if name == "string" { return true }
    if name == "bool" { return true }
    if name == "any" { return true }
    if name == "error" { return true }
    if name == "i8" { return true }
    if name == "i16" { return true }
    if name == "i32" { return true }
    if name == "i64" { return true }
    if name == "i128" { return true }
    if name == "i256" { return true }
    if name == "u8" { return true }
    if name == "u16" { return true }
    if name == "u32" { return true }
    if name == "u64" { return true }
    if name == "u128" { return true }
    if name == "u256" { return true }
    if name == "f32" { return true }
    if name == "f64" { return true }
    if name == "int" { return true }
    if name == "uint" { return true }
    if name == "byte" { return true }
    if name == "rune" { return true }
    return false
}

// --- collect type names from TypeExpr ---

func ast_collect_type_names(te: TypeExpr?, names: Dict<Sym, bool>) {
    if te == null {
        return
    }
    match te!.kind {
        Named(name, args) => {
            if !is_primitive_type(name.name) {
                names.set(sym(name.name), true)
            }
            for i in range(0, len(args)) {
                ast_collect_type_names(args[i], names)
            }
        }
        Optional(inner) => {
            ast_collect_type_names(inner, names)
        }
        Sequence(elem) => {
            ast_collect_type_names(elem, names)
        }
        Map(key, value) => {
            ast_collect_type_names(key, names)
            ast_collect_type_names(value, names)
        }
        Tuple(fields) => {
            for i in range(0, len(fields)) {
                ast_collect_type_names(fields[i].type_expr, names)
            }
        }
        Func(params, ret) => {
            for i in range(0, len(params)) {
                ast_collect_type_names(params[i], names)
            }
            ast_collect_type_names(ret, names)
        }
        _ => {}
    }
}

// --- collect used type names from user code ---

func ast_collect_used_type_names(file: File?) -> Dict<Sym, bool> {
    let names = Dict<Sym, bool>()
    if file == null {
        return names
    }
    let blocks = file!.fb_children()
    for bi in range(0, len(blocks)) {
        let block = blocks[bi]
        // Classes
        let classes = block.cd_children()
        for ci in range(0, len(classes)) {
            let cls = classes[ci]
            let fields = cls.cf_children()
            for fi in range(0, len(fields)) {
                ast_collect_type_names(fields[fi].type_expr, names)
            }
            let methods = cls.cm_children()
            for mi in range(0, len(methods)) {
                let m = methods[mi]
                let params = m.param_children()
                for pi in range(0, len(params)) {
                    ast_collect_type_names(params[pi].type_expr, names)
                }
                ast_collect_type_names(m.return_type, names)
            }
        }
        // Functions
        let fns = block.fd_children()
        for fi in range(0, len(fns)) {
            let fn_ = fns[fi]
            let params = fn_.param_children()
            for pi in range(0, len(params)) {
                ast_collect_type_names(params[pi].type_expr, names)
            }
            ast_collect_type_names(fn_.return_type, names)
        }
        // Structs
        let structs = block.sd_children()
        for si in range(0, len(structs)) {
            let s = structs[si]
            let fields = s.sf_children()
            for fi in range(0, len(fields)) {
                ast_collect_type_names(fields[fi].type_expr, names)
            }
        }
    }
    return names
}

// --- collect function call names from expressions/statements ---

func ast_collect_call_names_expr(expr: Expr?, names: Dict<Sym, bool>) {
    if expr == null {
        return
    }
    match expr!.kind {
        Call(func_expr, type_args, args, mut_args) => {
            match func_expr.kind {
                Ident(id_name) => {
                    names.set(sym(id_name.name), true)
                }
                _ => {}
            }
            ast_collect_call_names_expr(func_expr, names)
            for i in range(0, len(args)) {
                ast_collect_call_names_expr(args[i], names)
            }
        }
        Binary(left, op, right) => {
            ast_collect_call_names_expr(left, names)
            ast_collect_call_names_expr(right, names)
        }
        Unary(op, operand) => {
            ast_collect_call_names_expr(operand, names)
        }
        FieldAccess(receiver, field_name) => {
            ast_collect_call_names_expr(receiver, names)
        }
        MethodCall(receiver, method, type_args, args, mut_args) => {
            ast_collect_call_names_expr(receiver, names)
            for i in range(0, len(args)) {
                ast_collect_call_names_expr(args[i], names)
            }
        }
        StructLit(type_name, type_args, fields) => {
            names.set(type_name, true)
            for i in range(0, len(fields)) {
                ast_collect_call_names_expr(fields[i].value, names)
            }
        }
        Lambda(params, return_type, body) => {
            ast_collect_call_names_block(body, names)
        }
        Index(receiver, index) => {
            ast_collect_call_names_expr(receiver, names)
            ast_collect_call_names_expr(index, names)
        }
        Slice(receiver, low, high) => {
            ast_collect_call_names_expr(receiver, names)
            ast_collect_call_names_expr(low, names)
            ast_collect_call_names_expr(high, names)
        }
        ListLit(elems) => {
            for i in range(0, len(elems)) {
                ast_collect_call_names_expr(elems[i], names)
            }
        }
        MapLit(keys, values) => {
            // Dict literals desugar to Dict<K, V> — ensure stdlib pulls in Dict.
            names.set(`Dict`, true)
            for i in range(0, len(keys)) {
                ast_collect_call_names_expr(keys[i], names)
                ast_collect_call_names_expr(values[i], names)
            }
        }
        TupleLit(elems) => {
            for i in range(0, len(elems)) {
                ast_collect_call_names_expr(elems[i], names)
            }
        }
        Cast(target_type, operand) => {
            ast_collect_call_names_expr(operand, names)
        }
        Unwrap(operand) => {
            ast_collect_call_names_expr(operand, names)
        }
        Try(operand) => {
            ast_collect_call_names_expr(operand, names)
        }
        Is(operand, variant) => {
            ast_collect_call_names_expr(operand, names)
        }
        IfElse(cond, then_block, else_ifs, else_block) => {
            ast_collect_call_names_expr(cond, names)
            ast_collect_call_names_block(then_block, names)
            ast_collect_call_names_block(else_block, names)
            for i in range(0, len(else_ifs)) {
                ast_collect_call_names_expr(else_ifs[i].condition, names)
                if else_ifs[i].body != null {
                    ast_collect_call_names_block(else_ifs[i].body!, names)
                }
            }
        }
        StringInterp(parts) => {
            for i in range(0, len(parts)) {
                ast_collect_call_names_expr(parts[i], names)
            }
        }
        Match(value, arms) => {
            ast_collect_call_names_expr(value, names)
            for i in range(0, len(arms)) {
                if arms[i].body != null {
                    ast_collect_call_names_block(arms[i].body!, names)
                }
                ast_collect_call_names_expr(arms[i].guard, names)
            }
        }
        _ => {}
    }
}

func ast_collect_call_names_stmt(stmt: Stmt, names: Dict<Sym, bool>) {
    match stmt.kind {
        VarDecl(name, names_list, type_expr, is_mut, is_ref, value) => {
            ast_collect_call_names_expr(value, names)
        }
        Assign(target, value) => {
            ast_collect_call_names_expr(target, names)
            ast_collect_call_names_expr(value, names)
        }
        Return(value) => {
            ast_collect_call_names_expr(value, names)
        }
        ExprStmt(expr) => {
            ast_collect_call_names_expr(expr, names)
        }
        If(condition, then_block, else_ifs, else_block) => {
            ast_collect_call_names_expr(condition, names)
            ast_collect_call_names_block(then_block, names)
            if else_block != null {
                ast_collect_call_names_block(else_block!, names)
            }
            for i in range(0, len(else_ifs)) {
                ast_collect_call_names_expr(else_ifs[i].condition, names)
                if else_ifs[i].body != null {
                    ast_collect_call_names_block(else_ifs[i].body!, names)
                }
            }
        }
        For(var_name, index_var, collection, body) => {
            ast_collect_call_names_expr(collection, names)
            ast_collect_call_names_block(body, names)
        }
        While(condition, body) => {
            ast_collect_call_names_expr(condition, names)
            ast_collect_call_names_block(body, names)
        }
        Match(value, arms) => {
            ast_collect_call_names_expr(value, names)
            for i in range(0, len(arms)) {
                if arms[i].body != null {
                    ast_collect_call_names_block(arms[i].body!, names)
                }
                ast_collect_call_names_expr(arms[i].guard, names)
            }
        }
        BlockStmt(block) => {
            ast_collect_call_names_block(block, names)
        }
        Spawn(body) => {
            ast_collect_call_names_block(body, names)
        }
        Yield(value) => {
            ast_collect_call_names_expr(value, names)
        }
        IfLet(pattern, value, then_block, else_block) => {
            ast_collect_call_names_expr(value, names)
            ast_collect_call_names_block(then_block, names)
            if else_block != null {
                ast_collect_call_names_block(else_block!, names)
            }
        }
        LetElse(pattern, value, else_block) => {
            ast_collect_call_names_expr(value, names)
            ast_collect_call_names_block(else_block, names)
        }
        Lock(mutex, body) => {
            ast_collect_call_names_expr(mutex, names)
            ast_collect_call_names_block(body, names)
        }
        Select(cases) => {
            for i in range(0, len(cases)) {
                ast_collect_call_names_expr(cases[i].expr, names)
                if cases[i].body != null {
                    ast_collect_call_names_block(cases[i].body!, names)
                }
            }
        }
        _ => {}
    }
}

func ast_collect_call_names_block(block: Block, names: Dict<Sym, bool>) {
    let stmts = block.bs_children()
    for i in range(0, len(stmts)) {
        ast_collect_call_names_stmt(stmts[i], names)
    }
}

// --- collect variable references (for constant merging) ---

func ast_collect_var_refs_in_expr(expr: Expr?, names: Dict<Sym, bool>) {
    if expr == null {
        return
    }
    match expr!.kind {
        Ident(id_name) => {
            names.set(sym(id_name.name), true)
        }
        Call(func_expr, type_args, args, mut_args) => {
            ast_collect_var_refs_in_expr(func_expr, names)
            for i in range(0, len(args)) {
                ast_collect_var_refs_in_expr(args[i], names)
            }
        }
        Binary(left, op, right) => {
            ast_collect_var_refs_in_expr(left, names)
            ast_collect_var_refs_in_expr(right, names)
        }
        Unary(op, operand) => {
            ast_collect_var_refs_in_expr(operand, names)
        }
        FieldAccess(receiver, field_name) => {
            ast_collect_var_refs_in_expr(receiver, names)
        }
        MethodCall(receiver, method, type_args, args, mut_args) => {
            ast_collect_var_refs_in_expr(receiver, names)
            for i in range(0, len(args)) {
                ast_collect_var_refs_in_expr(args[i], names)
            }
        }
        Index(receiver, index) => {
            ast_collect_var_refs_in_expr(receiver, names)
            ast_collect_var_refs_in_expr(index, names)
        }
        StructLit(type_name, type_args, fields) => {
            for i in range(0, len(fields)) {
                ast_collect_var_refs_in_expr(fields[i].value, names)
            }
        }
        Lambda(params, return_type, body) => {
            ast_collect_var_refs_in_block(body, names)
        }
        Slice(receiver, low, high) => {
            ast_collect_var_refs_in_expr(receiver, names)
            ast_collect_var_refs_in_expr(low, names)
            ast_collect_var_refs_in_expr(high, names)
        }
        ListLit(elems) => {
            for i in range(0, len(elems)) {
                ast_collect_var_refs_in_expr(elems[i], names)
            }
        }
        MapLit(keys, values) => {
            for i in range(0, len(keys)) {
                ast_collect_var_refs_in_expr(keys[i], names)
                ast_collect_var_refs_in_expr(values[i], names)
            }
        }
        TupleLit(elems) => {
            for i in range(0, len(elems)) {
                ast_collect_var_refs_in_expr(elems[i], names)
            }
        }
        Cast(target_type, operand) => {
            ast_collect_var_refs_in_expr(operand, names)
        }
        Unwrap(operand) => {
            ast_collect_var_refs_in_expr(operand, names)
        }
        Try(operand) => {
            ast_collect_var_refs_in_expr(operand, names)
        }
        Is(operand, variant) => {
            ast_collect_var_refs_in_expr(operand, names)
        }
        IfElse(cond, then_block, else_ifs, else_block) => {
            ast_collect_var_refs_in_expr(cond, names)
            ast_collect_var_refs_in_block(then_block, names)
            ast_collect_var_refs_in_block(else_block, names)
            for i in range(0, len(else_ifs)) {
                ast_collect_var_refs_in_expr(else_ifs[i].condition, names)
                if else_ifs[i].body != null {
                    ast_collect_var_refs_in_block(else_ifs[i].body!, names)
                }
            }
        }
        StringInterp(parts) => {
            for i in range(0, len(parts)) {
                ast_collect_var_refs_in_expr(parts[i], names)
            }
        }
        Match(value, arms) => {
            ast_collect_var_refs_in_expr(value, names)
            for i in range(0, len(arms)) {
                if arms[i].body != null {
                    ast_collect_var_refs_in_block(arms[i].body!, names)
                }
                ast_collect_var_refs_in_expr(arms[i].guard, names)
            }
        }
        _ => {}
    }
}

func ast_collect_var_refs_in_stmt(stmt: Stmt, names: Dict<Sym, bool>) {
    match stmt.kind {
        VarDecl(name, names_list, type_expr, is_mut, is_ref, value) => {
            if value != null {
                ast_collect_var_refs_in_expr(value, names)
            }
        }
        Assign(target, value) => {
            ast_collect_var_refs_in_expr(target, names)
            ast_collect_var_refs_in_expr(value, names)
        }
        Return(value) => {
            if value != null {
                ast_collect_var_refs_in_expr(value, names)
            }
        }
        ExprStmt(expr) => {
            ast_collect_var_refs_in_expr(expr, names)
        }
        If(condition, then_block, else_ifs, else_block) => {
            ast_collect_var_refs_in_expr(condition, names)
            ast_collect_var_refs_in_block(then_block, names)
            if else_block != null {
                ast_collect_var_refs_in_block(else_block!, names)
            }
            for i in range(0, len(else_ifs)) {
                ast_collect_var_refs_in_expr(else_ifs[i].condition, names)
                if else_ifs[i].body != null {
                    ast_collect_var_refs_in_block(else_ifs[i].body!, names)
                }
            }
        }
        For(var_name, index_var, collection, body) => {
            ast_collect_var_refs_in_expr(collection, names)
            ast_collect_var_refs_in_block(body, names)
        }
        While(condition, body) => {
            ast_collect_var_refs_in_expr(condition, names)
            ast_collect_var_refs_in_block(body, names)
        }
        Match(value, arms) => {
            ast_collect_var_refs_in_expr(value, names)
            for i in range(0, len(arms)) {
                if arms[i].body != null {
                    ast_collect_var_refs_in_block(arms[i].body!, names)
                }
                ast_collect_var_refs_in_expr(arms[i].guard, names)
            }
        }
        BlockStmt(block) => {
            ast_collect_var_refs_in_block(block, names)
        }
        Spawn(body) => {
            ast_collect_var_refs_in_block(body, names)
        }
        Yield(value) => {
            ast_collect_var_refs_in_expr(value, names)
        }
        IfLet(pattern, value, then_block, else_block) => {
            ast_collect_var_refs_in_expr(value, names)
            ast_collect_var_refs_in_block(then_block, names)
            if else_block != null {
                ast_collect_var_refs_in_block(else_block!, names)
            }
        }
        LetElse(pattern, value, else_block) => {
            ast_collect_var_refs_in_expr(value, names)
            ast_collect_var_refs_in_block(else_block, names)
        }
        Lock(mutex, body) => {
            ast_collect_var_refs_in_expr(mutex, names)
            ast_collect_var_refs_in_block(body, names)
        }
        Select(cases) => {
            for i in range(0, len(cases)) {
                ast_collect_var_refs_in_expr(cases[i].expr, names)
                if cases[i].body != null {
                    ast_collect_var_refs_in_block(cases[i].body!, names)
                }
            }
        }
        _ => {}
    }
}

func ast_collect_var_refs_in_block(block: Block, names: Dict<Sym, bool>) {
    let stmts = block.bs_children()
    for i in range(0, len(stmts)) {
        ast_collect_var_refs_in_stmt(stmts[i], names)
    }
}

// --- collect used function names from user code ---

func ast_collect_used_func_names(file: File?) -> Dict<Sym, bool> {
    let names = Dict<Sym, bool>()
    if file == null {
        return names
    }
    let blocks = file!.fb_children()
    for bi in range(0, len(blocks)) {
        let block = blocks[bi]
        let fns = block.fd_children()
        for fi in range(0, len(fns)) {
            if fns[fi].body != null {
                ast_collect_call_names_block(fns[fi].body!, names)
            }
        }
        let classes = block.cd_children()
        for ci in range(0, len(classes)) {
            let methods = classes[ci].cm_children()
            for mi in range(0, len(methods)) {
                if methods[mi].body != null {
                    ast_collect_call_names_block(methods[mi].body!, names)
                }
            }
        }
    }
    return names
}

// --- func_references_types ---

func ast_func_references_types(fn_: FuncDecl, used_types: Dict<Sym, bool>) -> bool {
    if fn_.return_type != null {
        let ret_names = Dict<Sym, bool>()
        ast_collect_type_names(fn_.return_type, ret_names)
        let keys = ret_names.keys()
        for i in range(0, len(keys)) {
            let entry = used_types.get(keys[i])
            if entry != null {
                return true
            }
        }
    }
    return false
}

// --- merge_stdlib ---

func merge_stdlib(file: File?, std_file: File?) {
    if file == null { return }
    if std_file == null { return }

    // Collect relation hints (interface names used in relations)
    let used_ifaces = Dict<Sym, bool>()
    let blocks = file!.fb_children()
    for bi in range(0, len(blocks)) {
        let rels = blocks[bi].rd_children()
        for ri in range(0, len(rels)) {
            if rels[ri].hint != null {
                used_ifaces.set(sym(rels[ri].hint!.name), true)
            }
        }
    }

    // Collect used type names and function call names
    let used_types = ast_collect_used_type_names(file)
    let used_func_names = ast_collect_used_func_names(file)

    // Build stdlib lookups
    let std_iface_map = Dict<Sym, InterfaceDecl>()
    let std_class_map = Dict<Sym, ClassDecl>()
    let std_func_map = Dict<Sym, FuncDecl>()

    let std_blocks = std_file!.fb_children()
    for bi in range(0, len(std_blocks)) {
        let sb = std_blocks[bi]
        let ifaces = sb.id_children()
        for i in range(0, len(ifaces)) {
            if ifaces[i].name != null {
                std_iface_map.set(sym(ifaces[i].name!.name), ifaces[i])
            }
        }
        let classes = sb.cd_children()
        for i in range(0, len(classes)) {
            if classes[i].name != null {
                std_class_map.set(sym(classes[i].name!.name), classes[i])
            }
        }
        let fns = sb.fd_children()
        for i in range(0, len(fns)) {
            if fns[i].name != null {
                std_func_map.set(sym(fns[i].name!.name), fns[i])
            }
        }
    }

    // (Transitive embed pull-in removed — embed keyword deleted.)


    // Collect referenced interface declarations, skipping those already defined in user file
    let mut std_ifaces: [InterfaceDecl] = []
    // Build set of user-defined interface names to avoid duplicates
    let user_iface_names = Dict<Sym, bool>()
    for bi in range(0, len(blocks)) {
        let user_ifaces = blocks[bi].id_children()
        for i in range(0, len(user_ifaces)) {
            if user_ifaces[i].name != null {
                user_iface_names.set(sym(user_ifaces[i].name!.name), true)
            }
        }
    }
    let all_iface_keys = used_ifaces.keys()
    for i in range(0, len(all_iface_keys)) {
        // Skip if user already defines this interface
        let user_has = user_iface_names.get(all_iface_keys[i])
        if user_has != null { continue }
        let entry = std_iface_map.get(all_iface_keys[i])
        if entry != null {
            std_ifaces = append(std_ifaces, entry!.value)
        }
    }

    // Collect referenced classes
    let mut std_classes: [ClassDecl] = []
    let type_keys = used_types.keys()
    for i in range(0, len(type_keys)) {
        let entry = std_class_map.get(type_keys[i])
        if entry != null {
            std_classes = append(std_classes, entry!.value)
        }
    }

    // Also merge classes called as constructors (e.g. Dict<Sym, i32>())
    // — class names appear in usedFuncNames since constructors look like function calls
    let func_name_keys = used_func_names.keys()
    for i in range(0, len(func_name_keys)) {
        let fname = func_name_keys[i]
        let cls_entry = std_class_map.get(fname)
        if cls_entry != null {
            let already = used_types.get(fname)
            if already == null {
                used_types.set(fname, true)
                std_classes = append(std_classes, cls_entry!.value)
            }
        }
    }

    // Collect user-defined function names so stdlib doesn't shadow them.
    // Users can call shadowed stdlib functions via std.funcName().
    let user_defined_funcs = Dict<Sym, bool>()
    for bi in range(0, len(blocks)) {
        let user_fns = blocks[bi].fd_children()
        for i in range(0, len(user_fns)) {
            if user_fns[i].name != null {
                user_defined_funcs.set(sym(user_fns[i].name!.name), true)
            }
        }
    }

    // Collect functions that return/use referenced stdlib classes or are called
    let mut std_funcs: [FuncDecl] = []
    let merged_funcs = Dict<Sym, bool>()
    let func_keys = std_func_map.keys()
    for i in range(0, len(func_keys)) {
        let fname = func_keys[i]
        // Skip stdlib functions that would shadow user-defined functions
        let user_has_func = user_defined_funcs.get(fname)
        if user_has_func != null { continue }
        let fentry = std_func_map.get(fname)
        if fentry == null { continue }
        let fn_ = fentry!.value
        let called = used_func_names.get(fname)
        let refs_types = ast_func_references_types(fn_, used_types)
        if called != null || refs_types {
            std_funcs = append(std_funcs, fn_)
            merged_funcs.set(fname, true)
            // If function returns a stdlib class, merge that class too
            if fn_.return_type != null {
                let ret_names = Dict<Sym, bool>()
                ast_collect_type_names(fn_.return_type, ret_names)
                let rkeys = ret_names.keys()
                for ri in range(0, len(rkeys)) {
                    let cls_entry = std_class_map.get(rkeys[ri])
                    if cls_entry != null {
                        let already_type = used_types.get(rkeys[ri])
                        if already_type == null {
                            used_types.set(rkeys[ri], true)
                            std_classes = append(std_classes, cls_entry!.value)
                        }
                    }
                }
            }
        }
    }

    // Collect primitive extension methods (e.g. i32.get_hash, u64.get_hash) —
    // needed when generic code (Dict, etc.) calls methods on primitive type args.
    // Can't use std_func_map for these because multiple functions share the same
    // bare name (get_hash) and only the last one survives in the map.
    for bi in range(0, len(std_blocks)) {
        let sb = std_blocks[bi]
        let fns = sb.fd_children()
        for i in range(0, len(fns)) {
            if fns[i].name == null { continue }
            if fns[i].receiver_type == null { continue }
            if !is_primitive_type(fns[i].receiver_type!.name) { continue }
            let qname = fns[i].receiver_type!.name + "." + fns[i].name!.name
            let already = merged_funcs.get(sym(qname))
            if already != null { continue }
            std_funcs = append(std_funcs, fns[i])
            merged_funcs.set(sym(qname), true)
        }
    }

    // Collect stdlib relations whose participant types are being merged
    let merged_classes = Dict<Sym, bool>()
    for i in range(0, len(std_classes)) {
        if std_classes[i].name != null {
            merged_classes.set(sym(std_classes[i].name!.name), true)
        }
    }

    // Build set of existing relations to avoid duplicates
    let existing_rels = Dict<Sym, bool>()
    for bi in range(0, len(blocks)) {
        let rels = blocks[bi].rd_children()
        for ri in range(0, len(rels)) {
            let r = rels[ri]
            let mut key = ""
            if r.hint != null {
                key = r.hint!.name
            }
            key = f"{key}:{r.parent.type_name!.name}:{r.child.type_name!.name}"
            existing_rels.set(sym(key), true)
        }
    }

    let mut std_relations: [RelationDecl] = []
    for bi in range(0, len(std_blocks)) {
        let rels = std_blocks[bi].rd_children()
        for ri in range(0, len(rels)) {
            let r = rels[ri]
            let parent_name = r.parent.type_name!.name
            let child_name = r.child.type_name!.name

            let mut key = ""
            if r.hint != null {
                key = r.hint!.name
            }
            key = f"{key}:{parent_name}:{child_name}"
            let exists = existing_rels.get(sym(key))
            if exists != null { continue }

            let parent_merged = merged_classes.get(sym(parent_name))
            let child_merged = merged_classes.get(sym(child_name))
            if parent_merged != null || child_merged != null {
                std_relations = append(std_relations, r)
                // Ensure the interface hint is merged
                if r.hint != null {
                    let hint_name = r.hint!.name
                    let hint_used = used_ifaces.get(sym(hint_name))
                    if hint_used == null {
                        used_ifaces.set(sym(hint_name), true)
                        let hint_entry = std_iface_map.get(sym(hint_name))
                        if hint_entry != null {
                            std_ifaces = append(std_ifaces, hint_entry!.value)
                        }

                    }
                }
                // Ensure both participant types are merged
                if parent_merged == null {
                    let cls_entry = std_class_map.get(sym(parent_name))
                    if cls_entry != null {
                        merged_classes.set(sym(parent_name), true)
                        std_classes = append(std_classes, cls_entry!.value)
                    }
                }
                if child_merged == null {
                    let cls_entry = std_class_map.get(sym(child_name))
                    if cls_entry != null {
                        merged_classes.set(sym(child_name), true)
                        std_classes = append(std_classes, cls_entry!.value)
                    }
                }
            }
        }
    }

    // Transitively pull in functions called by already-merged functions
    let mut changed = true
    while changed {
        changed = false
        let all_func_keys = std_func_map.keys()
        for i in range(0, len(all_func_keys)) {
            let fname = all_func_keys[i]
            let already = merged_funcs.get(fname)
            if already != null { continue }
            let mut found = false
            for j in range(0, len(std_funcs)) {
                if std_funcs[j].body != null {
                    let calls = Dict<Sym, bool>()
                    ast_collect_call_names_block(std_funcs[j].body!, calls)
                    let call_entry = calls.get(fname)
                    if call_entry != null {
                        found = true
                        break
                    }
                }
            }
            if found {
                let fentry = std_func_map.get(fname)
                if fentry != null {
                    let fn_ = fentry!.value
                    merged_funcs.set(fname, true)
                    std_funcs = append(std_funcs, fn_)
                    changed = true
                    if fn_.return_type != null {
                        let ret_names = Dict<Sym, bool>()
                        ast_collect_type_names(fn_.return_type, ret_names)
                        let rkeys = ret_names.keys()
                        for ri in range(0, len(rkeys)) {
                            let cls_entry = std_class_map.get(rkeys[ri])
                            if cls_entry != null {
                                let cls_merged = merged_classes.get(rkeys[ri])
                                if cls_merged == null {
                                    merged_classes.set(rkeys[ri], true)
                                    std_classes = append(std_classes, cls_entry!.value)
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    // Also merge external methods (func T.method) whose receiver type is a merged class
    for bi in range(0, len(std_blocks)) {
        let fns = std_blocks[bi].fd_children()
        for i in range(0, len(fns)) {
            if fns[i].receiver_type != null {
                let recv_name = fns[i].receiver_type!.name
                let recv_merged = merged_classes.get(sym(recv_name))
                if recv_merged != null {
                    let fn_name = fns[i].name!.name
                    let already = merged_funcs.get(sym(fn_name))
                    if already == null {
                        merged_funcs.set(sym(fn_name), true)
                        std_funcs = append(std_funcs, fns[i])
                    }
                }
            }
        }
    }

    // Merge interfaces referenced by where clauses on merged classes and functions
    for i in range(0, len(std_classes)) {
        let wheres = std_classes[i].cwc_children()
        for wi in range(0, len(wheres)) {
            if wheres[wi].constraint != null {
                let cname = wheres[wi].constraint!.name
                let already = used_ifaces.get(sym(cname))
                if already == null {
                    used_ifaces.set(sym(cname), true)
                    let iface_entry = std_iface_map.get(sym(cname))
                    if iface_entry != null {
                        std_ifaces = append(std_ifaces, iface_entry!.value)
                    }
                }
            }
        }
    }
    for i in range(0, len(std_funcs)) {
        let wheres = std_funcs[i].where_children()
        for wi in range(0, len(wheres)) {
            if wheres[wi].constraint != null {
                let cname = wheres[wi].constraint!.name
                let already = used_ifaces.get(sym(cname))
                if already == null {
                    used_ifaces.set(sym(cname), true)
                    let iface_entry = std_iface_map.get(sym(cname))
                    if iface_entry != null {
                        std_ifaces = append(std_ifaces, iface_entry!.value)
                    }
                }
            }
        }
    }

    // Collect stdlib constants referenced by merged functions
    let std_const_map = Dict<Sym, ConstDecl>()
    for bi in range(0, len(std_blocks)) {
        let consts = std_blocks[bi].con_children()
        for i in range(0, len(consts)) {
            if consts[i].name != null {
                std_const_map.set(sym(consts[i].name!.name), consts[i])
            }
        }
    }
    let merged_var_refs = Dict<Sym, bool>()
    for i in range(0, len(std_funcs)) {
        if std_funcs[i].body != null {
            ast_collect_var_refs_in_block(std_funcs[i].body!, merged_var_refs)
        }
    }
    let mut std_constants: [ConstDecl] = []
    let const_keys = std_const_map.keys()
    for i in range(0, len(const_keys)) {
        let ref_entry = merged_var_refs.get(const_keys[i])
        if ref_entry != null {
            let c_entry = std_const_map.get(const_keys[i])
            if c_entry != null {
                std_constants = append(std_constants, c_entry!.value)
            }
        }
    }

    // Nothing to merge
    let total = (len(std_ifaces) + len(std_classes) + len(std_funcs) + len(std_relations) + len(std_constants))
    if total == 0 {
        return
    }

    // Merge into the first block only
    if len(blocks) > 0 {
        let block0 = blocks[0]
        for i in range(0, len(std_ifaces)) {
            array_append<LyricBlock, InterfaceDecl>(block0, std_ifaces[i])
        }
        for i in range(0, len(std_classes)) {
            array_append<LyricBlock, ClassDecl>(block0, std_classes[i])
        }
        for i in range(0, len(std_funcs)) {
            array_append<LyricBlock, FuncDecl>(block0, std_funcs[i])
        }
        for i in range(0, len(std_relations)) {
            array_append<LyricBlock, RelationDecl>(block0, std_relations[i])
        }
        for i in range(0, len(std_constants)) {
            array_append<LyricBlock, ConstDecl>(block0, std_constants[i])
        }
    }
}



}
