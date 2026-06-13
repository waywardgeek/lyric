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

  class LType {
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

  class LValue {
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

  class LExpr {
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

  class LCallData {
    func_name: string
    args: [LValue?]
    mut_args: [bool]
    type_args: [LType?]
    is_exported: bool
  }

  class LMethodCallData {
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

  class LClassAllocData {
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
  }

  class LStmt {
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

  class LStructDecl {
    name: string
    fields: [LField]
    type_params: [LTypeParam]
    is_exported: bool
  }

  class LClassDecl {
    name: string
    fields: [LField]
    type_params: [LTypeParam]
    is_exported: bool
    implements: [string]
  }

  class LEnumDecl {
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

  class LInterfaceDecl {
    name: string
    type_params: [LTypeParam]
    methods: [LInterfaceMethod]
    embeds: [string]
    is_exported: bool
  }

  struct LRelationalConstraint {
    interface_name: string
    type_args: [string]
  }

  class LFuncDecl {
    name: string
    type_params: [LTypeParam]
    params: [LParam]
    return_type: LType?
    body: [LStmt?]
    is_exported: bool
    receiver: string
    receiver_type_params: [LTypeParam]
    relational_constraints: [LRelationalConstraint]
    class_rename_map: Dict<Sym, string>?
  }

  class LTypeDef {
    name: string
    typ: LType?
    is_exported: bool
  }

  class LProgram {
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
    slab_mode: bool
    slab_mode_soa: bool
  }

}
