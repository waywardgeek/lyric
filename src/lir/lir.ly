lyric lir {

  // ==========================================================================
  // LIR Type System
  // ==========================================================================

  enum LTypeKind {
    TyI8
    TyI16
    TyI32
    TyI64
    TyU8
    TyU16
    TyU32
    TyU64
    TyF32
    TyF64
    TyBool
    TyString
    TyUnit
    TyError
    TyPlatformInt
    TyPlatformUint
    TyStruct
    TyClassHandle
    TyTuple
    TySlice
    TyMap
    TyChannel
    TyGenerator
    TyMutex
    TyOptional
    TyTaggedUnion
    TyFuncPtr
    TyErrorResult
    TyAny
    TyTypeVar
    TyUnion
  }

  struct LField {
    name: string
    typ: LType?
  }

  struct LVariant {
    name: string
    tag: i32
    fields: [LField]
  }

  permanent class LType {
    kind: LTypeKind
    name: string
    elem: LType?
    key: LType?
    fields: [LField]
    params: [LType?]
    ret: LType?
    variants: [LVariant]
    type_args: [LType?]
    bits: i32
    is_exported: bool
    is_permanent: bool
    is_owned: bool
    class_decl: LClassDecl?
  }

  // ==========================================================================
  // LIR Values (Operands)
  // ==========================================================================

  enum LValueKind {
    ValVar
    ValTemp
    ValGlobal
    ValLitInt
    ValLitUint
    ValLitFloat
    ValLitString
    ValLitBool
    ValLitNull
    ValIndexRef
    ValClassFieldRef
  }

  permanent class LValue {
    kind: LValueKind
    name: string
    temp_id: i32
    int_val: i64
    uint_val: u64
    float_val: f64
    str_val: string
    bool_val: bool
    typ: LType?
    collection: LValue?
    index: LValue?
  }

  // ==========================================================================
  // LIR Expressions
  // ==========================================================================

  enum LExprKind {
    ExBinOp
    ExUnOp
    ExCast
    ExStructField
    ExClassGet
    ExIndexGet
    ExSlice
    ExCall
    ExMethodCall
    ExBuiltin
    ExStructLit
    ExClassAlloc
    ExMakeSlice
    ExMakeMap
    ExMakeChannel
    ExWrapOptional
    ExUnwrapOptional
    ExIsNull
    ExVariantConstruct
    ExVariantTag
    ExVariantData
    ExExtractValue
    ExExtractError
    ExMakeResult
    ExEnvGet
    ExFuncRef
    ExFuncLit
    ExFormat
    ExSlabGet
    ExSlabAlloc
  }

  permanent class LExpr {
    kind: LExprKind
    typ: LType?
    // Data is one of the L*Data classes below, stored by kind.
    // Since Lyric has no `any` type for tagged dispatch in bootstrap,
    // we use nullable fields for each data variant.
    bin_op: LBinOpData?
    un_op: LUnOpData?
    cast: LCastData?
    struct_field: LStructFieldData?
    class_get: LClassGetData?
    index_get: LIndexGetData?
    slice_data: LSliceData?
    call: LCallData?
    method_call: LMethodCallData?
    builtin: LBuiltinData?
    struct_lit: LStructLitData?
    class_alloc: LClassAllocData?
    make_channel: LMakeChannelData?
    wrap_opt: LWrapOptionalData?
    unwrap_opt: LUnwrapOptionalData?
    is_null: LIsNullData?
    variant_construct: LVariantConstructData?
    variant_tag: LVariantTagData?
    variant_data: LVariantDataData?
    extract_value: LExtractValueData?
    extract_error: LExtractErrorData?
    make_result: LMakeResultData?
    env_get: LEnvGetData?
    func_ref: LFuncRefData?
    func_lit: LFuncLitData?
    format: LFormatData?
    slab_get: LSlabGetData?
    slab_alloc: LSlabAllocData?
  }

  enum LBinOpKind {
    BinAdd
    BinSub
    BinMul
    BinDiv
    BinMod
    BinEq
    BinNe
    BinLt
    BinLe
    BinGt
    BinGe
    BinAnd
    BinOr
    BinBitAnd
    BinBitOr
    BinBitXor
    BinShl
    BinShr
  }

  enum LUnOpKind {
    UnNeg
    UnNot
    UnBitNot
  }

  struct LBinOpData {
    op: LBinOpKind
    left: LValue?
    right: LValue?
  }

  struct LUnOpData {
    op: LUnOpKind
    operand: LValue?
  }

  struct LCastData {
    target: LType?
    operand: LValue?
  }

  struct LStructFieldData {
    receiver: LValue?
    field: string
  }

  struct LClassGetData {
    handle: LValue?
    class_name: string
    field: string
  }

  struct LIndexGetData {
    collection: LValue?
    index: LValue?
  }

  struct LSliceData {
    collection: LValue?
    low: LValue?
    high: LValue?
  }

  permanent class LCallData {
    func_name: string
    args: [LValue?]
    mut_args: [bool]
    type_args: [LType?]
    is_exported: bool
  }

  permanent class LMethodCallData {
    receiver: LValue?
    method: string
    args: [LValue?]
    mut_args: [bool]
    type_args: [LType?]
    is_exported: bool
    param_types: [LType?]
  }

  struct LBuiltinData {
    name: string
    args: [LValue?]
    file: string
    line: i32
  }

  struct LFieldInit {
    name: string
    value: LValue?
  }

  struct LStructLitData {
    fields: [LFieldInit]
  }

  permanent class LClassAllocData {
    class_name: string
    fields: [LFieldInit]
    type_args: [LType?]
  }

  struct LMakeChannelData {
    elem_type: LType?
    buf_size: LValue?
  }

  struct LWrapOptionalData {
    value: LValue?
  }

  struct LUnwrapOptionalData {
    value: LValue?
  }

  struct LIsNullData {
    value: LValue?
  }

  struct LVariantConstructData {
    enum_name: string
    variant: string
    tag: i32
    fields: [LValue?]
  }

  struct LVariantTagData {
    value: LValue?
  }

  struct LVariantDataData {
    value: LValue?
    enum_name: string
    variant: string
    field: string
  }

  struct LExtractValueData {
    value: LValue?
  }

  struct LExtractErrorData {
    value: LValue?
  }

  struct LMakeResultData {
    value: LValue?
    err: LValue?
  }

  struct LEnvGetData {
    env: LValue?
    field: string
  }

  struct LFuncRefData {
    name: string
    env: LValue?
  }

  struct LFormatPart {
    is_literal: bool
    text: string
    value: LValue?
    format: string
  }

  struct LFormatData {
    parts: [LFormatPart]
  }

  struct LFuncLitData {
    params: [LParam]
    return_type: LType?
    body: [LStmt?]
  }

  // Slab allocator data types (memory management pass)
  struct LSlabGetData {
    class_name: string
    field: string
    handle: LValue?
  }

  struct LSlabAllocData {
    class_name: string
    fields: [LFieldInit]
  }

  struct LSlabSetData {
    class_name: string
    field: string
    handle: LValue?
    value: LValue?
  }

  struct LSlabFreeData {
    class_name: string
    handle: LValue?
  }

  struct LSliceFreeData {
    name: string
    typ: LType?
  }

  // Data for StSliceRcRelease: release RC on each element before slice free
  struct LSliceRcReleaseData {
    slice_name: string   // variable name of the slice
    elem_type: LType?    // element type (class handle or struct with RC fields)
  }

  struct LRefIncrData {
    handle: LValue?
    class_name: string
  }

  struct LRefDecrData {
    handle: LValue?
    class_name: string
  }

  // ==========================================================================
  // LIR Statements
  // ==========================================================================

  enum LStmtKind {
    StTempDef
    StVarDecl
    StAssign
    StStructSet
    StClassSet
    StIndexSet
    StIf
    StWhile
    StFor
    StSwitch
    StBlock
    StReturn
    StBreak
    StContinue
    StExpr
    StDefer
    StSpawn
    StLock
    StSend
    StSelect
    StYield
    StSideEffect
    StMultiAssign
    StTypeSwitch
    StSlabSet
    StSlabFree
    StSliceFree
    StSliceRetain
    StRefIncr
    StRefDecr
    StSliceRcRelease  // Release RC on each element of a slice before freeing
  }

  permanent class LStmt {
    kind: LStmtKind
    // Data variant fields — one is set based on kind.
    temp_def: LTempDef?
    var_decl: LVarDeclData?
    assign: LAssignData?
    struct_set: LStructSetData?
    class_set: LClassSetData?
    index_set: LIndexSetData?
    if_data: LIfData?
    while_data: LWhileData?
    for_data: LForData?
    switch_data: LSwitchData?
    block: LBlockData?
    ret: LReturnData?
    expr_stmt: LExprStmtData?
    defer_data: LDeferData?
    spawn_data: LSpawnData?
    lock_data: LLockData?
    send_data: LSendData?
    select_data: LSelectData?
    yield_data: LYieldData?
    side_effect: LSideEffectData?
    multi_assign: LMultiAssignData?
    type_switch: LTypeSwitchData?
    slab_set: LSlabSetData?
    slab_free: LSlabFreeData?
    slice_free: LSliceFreeData?
    slice_retain: LSliceFreeData?
    ref_incr: LRefIncrData?
    ref_decr: LRefDecrData?
    slice_rc_release: LSliceRcReleaseData?
  }

  struct LTempDef {
    id: i32
    expr: LExpr?
  }

  struct LVarDeclData {
    name: string
    typ: LType?
    init: LValue?
    mutable: bool
    is_ref: bool
  }

  struct LAssignData {
    target: string
    value: LValue?
  }

  struct LStructSetData {
    receiver: LValue?
    field: string
    value: LValue?
  }

  struct LClassSetData {
    handle: LValue?
    class_name: string
    field: string
    value: LValue?
  }

  struct LIndexSetData {
    collection: LValue?
    index: LValue?
    value: LValue?
    field: string  // optional: for slice[i].field = value (struct element field write)
  }

  struct LIfData {
    cond: LValue?
    then_body: [LStmt?]
    else_body: [LStmt?]
  }

  struct LWhileData {
    cond_block: [LStmt?]
    cond_var: LValue?
    body: [LStmt?]
  }

  struct LForData {
    var_name: string
    var_type: LType?
    index_var: string
    collection: LValue?
    body: [LStmt?]
  }

  struct LSwitchCase {
    tag: i32
    binding: string
    body: [LStmt?]
  }

  struct LSwitchData {
    tag: LValue?
    cases: [LSwitchCase]
    enum_name: string
  }

  struct LTypeSwitchCase {
    typ: LType?
    binding: string
    body: [LStmt?]
  }

  struct LTypeSwitchData {
    value: LValue?
    cases: [LTypeSwitchCase]
  }

  struct LReturnData {
    values: [LValue?]
  }

  struct LBlockData {
    stmts: [LStmt?]
  }

  struct LExprStmtData {
    temp_id: i32
  }

  struct LDeferData {
    body: [LStmt?]
  }

  struct LSpawnData {
    body: [LStmt?]
    captures: [string]
  }

  struct LLockData {
    mutex: LValue?
    body: [LStmt?]
  }

  struct LSendData {
    channel: LValue?
    value: LValue?
  }

  enum LSelectKind {
    SelRecv
    SelSend
    SelDefault
  }

  struct LSelectCase {
    kind: LSelectKind
    channel: LValue?
    value: LValue?
    binding: string
    body: [LStmt?]
  }

  struct LSelectData {
    cases: [LSelectCase]
  }

  struct LYieldData {
    value: LValue?
  }

  struct LSideEffectData {
    expr: LExpr?
  }

  struct LMultiAssignData {
    names: [string]
    types: [LType?]
    expr: LExpr?
  }

  // ==========================================================================
  // Top-Level Declarations
  // ==========================================================================

  struct LParam {
    name: string
    typ: LType?
    mutable: bool
  }

  struct LTypeParam {
    name: string
    constraint: string
  }

  struct LImport {
    alias: string
    path: string
  }

  permanent class LStructDecl {
    name: string
    fields: [LField]
    type_params: [LTypeParam]
    is_exported: bool
  }

  permanent class LClassDecl {
    name: string
    fields: [LField]
    type_params: [LTypeParam]
    is_exported: bool
    is_permanent: bool
    is_owned: bool
    implements: [string]
  }

  permanent class LEnumDecl {
    name: string
    variants: [LVariant]
    is_exported: bool
  }

  struct LInterfaceMethod {
    name: string
    receiver_type: string
    params: [LParam]
    return_type: LType?
  }

  permanent class LInterfaceDecl {
    name: string
    type_params: [LTypeParam]
    methods: [LInterfaceMethod]
    is_exported: bool
  }

  struct LRelationalConstraint {
    interface_name: string
    type_args: [string]
  }

  permanent class LFuncDecl {
    name: string
    type_params: [LTypeParam]
    params: [LParam]
    return_type: LType?
    body: [LStmt?]
    is_exported: bool
    is_final: bool
    is_trusted: bool
    receiver: string
    receiver_type_params: [LTypeParam]
    relational_constraints: [LRelationalConstraint]
    class_rename_map: Dict<Sym, string>?
  }

  permanent class LTypeDef {
    name: string
    typ: LType?
    is_exported: bool
  }

  permanent class LProgram {
    package_name: string
    imports: [LImport]
    structs: [LStructDecl]
    classes: [LClassDecl]
    enums: [LEnumDecl]
    interfaces: [LInterfaceDecl]
    functions: [LFuncDecl]
    globals: [LVarDeclData]
    type_defs: [LTypeDef]
    class_renames: Dict<Sym, string>?
    impl_method_renames: Dict<Sym, string>?
    owned_classes: Dict<Sym, bool>?
    slab_mode: bool
    slab_mode_soa: bool
    detect_uaf: bool
    rc_free: bool
    unsafe_mode: bool   // -U flag: skip null checks on `!` unwrap for speed
  }

  // Resolve class_decl, is_permanent, is_owned on all LType nodes in the program.
  // Must be called after lowering (to wire up initial references) and again after
  // monomorphization (to wire up specialized class references).
  func LProgram.resolve_class_types(self) {
    // Build name → LClassDecl index
    let index = Dict<Sym, LClassDecl>()
    let mut i = 0
    while i < len(self.classes) {
      index.set(sym(self.classes[i].name), self.classes[i])
      // Also set is_owned on the class decl from owned_classes dict
      if !isnull(self.owned_classes) {
        let entry = self.owned_classes!.get(sym(self.classes[i].name))
        if !isnull(entry) {
          self.classes[i].is_owned = true
        }
      }
      i = i + 1
    }

    // Walk all types in functions
    i = 0
    while i < len(self.functions) {
      let f = self.functions[i]
      resolve_type(f.return_type, index)
      let mut j = 0
      while j < len(f.params) {
        resolve_type(f.params[j].typ, index)
        j = j + 1
      }
      resolve_stmts(f.body, index)
      i = i + 1
    }

    // Walk struct and class field types
    i = 0
    while i < len(self.structs) {
      let mut j = 0
      while j < len(self.structs[i].fields) {
        resolve_type(self.structs[i].fields[j].typ, index)
        j = j + 1
      }
      i = i + 1
    }
    i = 0
    while i < len(self.classes) {
      let mut j = 0
      while j < len(self.classes[i].fields) {
        resolve_type(self.classes[i].fields[j].typ, index)
        j = j + 1
      }
      i = i + 1
    }

    // Walk globals
    i = 0
    while i < len(self.globals) {
      resolve_type(self.globals[i].typ, index)
      i = i + 1
    }
  }

}

// Helper: resolve a single LType's class_decl, is_permanent, is_owned
func resolve_type(typ: LType?, index: Dict<Sym, LClassDecl>) {
  if isnull(typ) { return }
  if typ!.kind is TyClassHandle {
    let entry = index.get(sym(typ!.name))
    if !isnull(entry) {
      typ!.class_decl = entry!.value
      typ!.is_permanent = entry!.value.is_permanent
      typ!.is_owned = entry!.value.is_owned
    }
  }
  // Recurse into nested types
  resolve_type(typ!.elem, index)
  resolve_type(typ!.key, index)
  resolve_type(typ!.ret, index)
  let mut i = 0
  while i < len(typ!.type_args) {
    resolve_type(typ!.type_args[i], index)
    i = i + 1
  }
  i = 0
  while i < len(typ!.params) {
    resolve_type(typ!.params[i], index)
    i = i + 1
  }
}

// Walk statements to resolve all embedded LTypes
func resolve_stmts(stmts: [LStmt?], index: Dict<Sym, LClassDecl>) {
  let mut i = 0
  while i < len(stmts) {
    let s = stmts[i]
    if isnull(s) {
      i = i + 1
      continue
    }
    resolve_stmt(s!, index)
    i = i + 1
  }
}

func resolve_stmt(s: LStmt, index: Dict<Sym, LClassDecl>) {
  match s.kind {
    StVarDecl => {
      if !isnull(s.var_decl) {
        resolve_type(s.var_decl!.typ, index)
        resolve_value(s.var_decl!.init, index)
      }
    }
    StTempDef => {
      if !isnull(s.temp_def) {
        resolve_expr(s.temp_def!.expr, index)
      }
    }
    StAssign => {
      if !isnull(s.assign) {
        resolve_value(s.assign!.value, index)
      }
    }
    StReturn => {
      if !isnull(s.ret) {
        let mut j = 0
        while j < len(s.ret!.values) {
          resolve_value(s.ret!.values[j], index)
          j = j + 1
        }
      }
    }
    StIf => {
      if !isnull(s.if_data) {
        resolve_value(s.if_data!.cond, index)
        resolve_stmts(s.if_data!.then_body, index)
        resolve_stmts(s.if_data!.else_body, index)
      }
    }
    StWhile => {
      if !isnull(s.while_data) {
        resolve_value(s.while_data!.cond_var, index)
        resolve_stmts(s.while_data!.cond_block, index)
        resolve_stmts(s.while_data!.body, index)
      }
    }
    StFor => {
      if !isnull(s.for_data) {
        resolve_type(s.for_data!.var_type, index)
        resolve_value(s.for_data!.collection, index)
        resolve_stmts(s.for_data!.body, index)
      }
    }
    StSwitch => {
      if !isnull(s.switch_data) {
        resolve_value(s.switch_data!.tag, index)
        let mut j = 0
        while j < len(s.switch_data!.cases) {
          resolve_stmts(s.switch_data!.cases[j].body, index)
          j = j + 1
        }
      }
    }
    StBlock => {
      if !isnull(s.block) {
        resolve_stmts(s.block!.stmts, index)
      }
    }
    _ => { }
  }
}

func resolve_expr(expr: LExpr?, index: Dict<Sym, LClassDecl>) {
  if isnull(expr) { return }
  resolve_type(expr!.typ, index)
  match expr!.kind {
    ExCall => {
      if !isnull(expr!.call) {
        let mut j = 0
        while j < len(expr!.call!.args) {
          resolve_value(expr!.call!.args[j], index)
          j = j + 1
        }
        j = 0
        while j < len(expr!.call!.type_args) {
          resolve_type(expr!.call!.type_args[j], index)
          j = j + 1
        }
      }
    }
    ExMethodCall => {
      if !isnull(expr!.method_call) {
        resolve_value(expr!.method_call!.receiver, index)
        let mut j = 0
        while j < len(expr!.method_call!.args) {
          resolve_value(expr!.method_call!.args[j], index)
          j = j + 1
        }
        j = 0
        while j < len(expr!.method_call!.type_args) {
          resolve_type(expr!.method_call!.type_args[j], index)
          j = j + 1
        }
        j = 0
        while j < len(expr!.method_call!.param_types) {
          resolve_type(expr!.method_call!.param_types[j], index)
          j = j + 1
        }
      }
    }
    ExClassAlloc => {
      if !isnull(expr!.class_alloc) {
        let mut j = 0
        while j < len(expr!.class_alloc!.fields) {
          resolve_value(expr!.class_alloc!.fields[j].value, index)
          j = j + 1
        }
      }
    }
    _ => { }
  }
}

func resolve_value(val: LValue?, index: Dict<Sym, LClassDecl>) {
  if isnull(val) { return }
  resolve_type(val!.typ, index)
}
