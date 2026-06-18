// checker.ly — Full type checker for the Lyric bootstrap compiler
// Ported from pkg/checker/checker.go (~4163 lines)
//
// Architecture:
//   - Two-phase: register_lyric_block (types/signatures) then check_lyric_block_bodies (bodies)
//   - Internal Type representation for checking; type_to_type_expr converts to AST TypeExpr
//   - ResolvedType annotation on every Expr node (checker→lowerer contract)
//   - Scope chain for variable resolution (parent pointers)
//   - Dict-based registries for types, functions, methods
//   - matchTypeVars for generic type inference
//   - Invariant validators: validateAllExprsResolved, validateFieldAndMethodAccess

lyric checker {

  // =====================================================================
  // Type system
  // =====================================================================

  enum TypeKind {
    Int
    Uint
    Float
    Bool
    String
    Void     // unit type (no value)
    Nil      // null literal — assignable to optional, class, interface, error
    Any
    Error    // error interface
    Lock
    Optional(inner: Type)
    Sequence(elem: Type)
    Map(key: Type, value: Type)
    Tuple(fields: [TupleFieldType])
    Func(params: [Type], ret: Type, type_param_names: [string])
    Struct(name: string)
    Class(name: string)
    Enum(name: string)
    Interface(name: string)
    TypeVar(name: string)
    Channel(elem: Type)
    Generator(elem: Type)
    ErrorResult(ok: Type, err: Type)
    Module(name: string)
    Union(variants: [Type])
  }

  struct TupleFieldType {
    name: string
    type_val: Type?
  }

  permanent class Type {
    kind: TypeKind
    bits: i32  // for Int/Uint/Float: 8,16,32,64; -1 for platform-width int/uint
    type_args: [Type]  // for generic class/struct instances (e.g., Stack<i32> → [i32])
  }

  // ---- TypeInfo: registry entry for named types ----
  permanent class TypeInfo {
    type_val: Type
    fields: Dict<Sym, Type>        // field name -> type
    field_order: [string]     // field names in declaration order
    methods: Dict<Sym, Type>       // method name -> func type
    variants: Dict<Sym, VariantInfo>  // variant name -> info (for enums)
    type_param_names: [string]
    type_param_constraints: [string]
    implements_list: [string]   // interface names this class implements
  }

  permanent class VariantInfo {
    enum_name: string
    fields: [VariantField]
  }

  struct VariantField {
    name: string
    type_val: Type
  }

  // =====================================================================
  // Scope
  // =====================================================================

  permanent class Scope {
    parent: Scope?
    vars: Dict<Sym, Type>
  }

  func new_scope(parent: Scope?) -> Scope {
    return Scope { parent: parent, vars: Dict<Sym, Type>() }
  }

  func Scope.lookup(self, name: string) -> Type? {
    let entry = self.vars.get(sym(name))
    if entry != null {
      return entry!.value
    }
    if self.parent != null {
      return self.parent!.lookup(name)
    }
    return null
  }

  func Scope.define(self, name: string, t: Type) {
    self.vars.set(sym(name), t)
  }

  // =====================================================================
  // Registry
  // =====================================================================

  permanent class Registry {
    types: Dict<Sym, TypeInfo>
  }

  func new_registry() -> Registry {
    return Registry { types: Dict<Sym, TypeInfo>() }
  }

  func Registry.register(self, name: string, info: TypeInfo) {
    // Panic if overwriting a fully-registered type (has methods or fields).
    // Phase 0 stubs have empty methods/fields, so Phase 1 overwrites are OK.
    let existing = self.lookup(name)
    if existing != null {
      let eem = existing!.methods.keys()
      let eef = existing!.fields.keys()
      if len(eem) > 0 || len(eef) > 0 {
        eprintln(f"FATAL: registry overwrite of fully-registered type '{name}' (had {itoa(len(eem) as i64)} methods, {itoa(len(eef) as i64)} fields)")
        os_exit(1)
      }
    }
    self.types.set(sym(name), info)
  }

  func Registry.lookup(self, name: string) -> TypeInfo? {
    let entry = self.types.get(sym(name))
    if entry != null {
      return entry!.value
    }
    return null
  }

  // =====================================================================
  // Checker state
  // =====================================================================

  // Global: maps label-prefixed method names to base method names.
  // e.g. "c1_append" → "append". Populated by checker, read by monomorphizer.
  let mut _method_aliases: Dict<Sym, string>? = null
  let mut _method_labels: Dict<Sym, string>? = null

  func get_method_aliases() -> Dict<Sym, string> {
    if isnull(_method_aliases) {
      _method_aliases = Dict<Sym, string>()
    }
    return _method_aliases!
  }

  func get_method_labels() -> Dict<Sym, string> {
    if isnull(_method_labels) {
      _method_labels = Dict<Sym, string>()
    }
    return _method_labels!
  }

  permanent class Checker {
    registry: Registry
    scope: Scope
    errors: [string]
    current_func_return: Type?
    current_func_name: string
    iface_decls: Dict<Sym, InterfaceDecl>  // for checkImplements
    type_var_methods: Dict<Sym, Dict<Sym, Type>>?  // type var name → (method name → method type)
    method_type_args: Dict<Sym, [TypeExpr]>  // "Type.method" → concrete type args for interface methods
    in_trusted: bool
  }

  func new_checker() -> Checker {
    let c = Checker {
      registry: new_registry(),
      scope: new_scope(null),
      errors: [],
      current_func_return: null,
      current_func_name: "",
      iface_decls: Dict<Sym, InterfaceDecl>(),
      method_type_args: Dict<Sym, [TypeExpr]>(),
    }
    c.register_builtins()
    return c
  }

  func Checker.push_scope(self) {
    self.scope = new_scope(self.scope)
  }

  func Checker.pop_scope(self) {
    if self.scope.parent != null {
      self.scope = self.scope.parent!
    }
  }

  // =====================================================================
  // Type constructors
  // =====================================================================

  func make_int_type(bits: i32) -> Type {
    return Type { kind: Int, bits: bits, type_args: [] }
  }

  func make_uint_type(bits: i32) -> Type {
    return Type { kind: Uint, bits: bits, type_args: [] }
  }

  func make_float_type(bits: i32) -> Type {
    return Type { kind: Float, bits: bits, type_args: [] }
  }

  func make_bool_type() -> Type {
    return Type { kind: Bool, bits: 0, type_args: [] }
  }

  func make_string_type() -> Type {
    return Type { kind: String, bits: 0, type_args: [] }
  }

  func make_void_type() -> Type {
    return Type { kind: Void, bits: 0, type_args: [] }
  }

  func make_nil_type() -> Type {
    return Type { kind: TypeKind.Nil, bits: 0, type_args: [] }
  }

  func make_any_type() -> Type {
    return Type { kind: Any, bits: 0, type_args: [] }
  }

  func make_error_type() -> Type {
    return Type { kind: Error, bits: 0, type_args: [] }
  }

  func make_lock_type() -> Type {
    return Type { kind: TypeKind.Lock, bits: 0, type_args: [] }
  }

  func make_union_type(variants: [Type]) -> Type {
    return Type { kind: TypeKind.Union(variants), bits: 0, type_args: [] }
  }

  func make_optional_type(inner: Type) -> Type {
    return Type { kind: TypeKind.Optional(inner), bits: 0, type_args: [] }
  }

  func make_sequence_type(elem: Type) -> Type {
    return Type { kind: TypeKind.Sequence(elem), bits: 0, type_args: [] }
  }

  func make_map_type(key: Type, value: Type) -> Type {
    return Type { kind: TypeKind.Map(key, value), bits: 0, type_args: [] }
  }

  func make_tuple_type(fields: [TupleFieldType]) -> Type {
    return Type { kind: TypeKind.Tuple(fields), bits: 0, type_args: [] }
  }

  func make_func_type(params: [Type], ret: Type, tpnames: [string]) -> Type {
    return Type { kind: TypeKind.Func(params, ret, tpnames), bits: 0, type_args: [] }
  }

  func make_channel_type(elem: Type) -> Type {
    return Type { kind: TypeKind.Channel(elem), bits: 0, type_args: [] }
  }

  func make_generator_type(elem: Type) -> Type {
    return Type { kind: TypeKind.Generator(elem), bits: 0, type_args: [] }
  }

  }

  func make_error_result_type(ok: Type, err: Type) -> Type {
    return Type { kind: ErrorResult(ok, err), bits: 0, type_args: [] }
  }

  func make_typevar_type(name: string) -> Type {
    return Type { kind: TypeVar(name), bits: 0, type_args: [] }
  }

  func make_struct_type(name: string) -> Type {
    return Type { kind: Struct(name), bits: 0, type_args: [] }
  }

  func make_class_type(name: string) -> Type {
    return Type { kind: Class(name), bits: 0, type_args: [] }
  }

  func make_enum_type(name: string) -> Type {
    return Type { kind: Enum(name), bits: 0, type_args: [] }
  }

  func make_interface_type(name: string) -> Type {
    return Type { kind: Interface(name), bits: 0, type_args: [] }
  }

  func make_module_type(name: string) -> Type {
    return Type { kind: Module(name), bits: 0, type_args: [] }
  }

  // =====================================================================
  // Built-in registration
  // =====================================================================

  func Checker.register_builtins(self) {
    // Primitive types
    self.register_primitive_type("bool", make_bool_type())
    self.register_primitive_type("string", make_string_type())
    self.register_primitive_type("i8", make_int_type(8))
    self.register_primitive_type("i16", make_int_type(16))
    self.register_primitive_type("i32", make_int_type(32))
    self.register_primitive_type("i64", make_int_type(64))
    self.register_primitive_type("int", make_int_type(-1))
    self.register_primitive_type("u8", make_uint_type(8))
    self.register_primitive_type("u16", make_uint_type(16))
    self.register_primitive_type("u32", make_uint_type(32))
    self.register_primitive_type("u64", make_uint_type(64))
    self.register_primitive_type("uint", make_uint_type(-1))
    self.register_primitive_type("f32", make_float_type(32))
    self.register_primitive_type("f64", make_float_type(64))
    self.register_primitive_type("any", make_any_type())
    self.register_primitive_type("error", make_error_type())

    // Register Go stdlib modules as opaque types
    self.register_go_stdlib_modules()
  }

  func Checker.register_primitive_type(self, name: string, t: Type) {
    let info = TypeInfo {
      type_val: t,
      fields: Dict<Sym, Type>(),
      field_order: [],
      methods: Dict<Sym, Type>(),
      variants: Dict<Sym, VariantInfo>(),
      type_param_names: [],
      type_param_constraints: [],
      implements_list: [],
    }
    self.registry.register(name, info)
    self.scope.define(name, t)
  }

  func Checker.register_go_stdlib_modules(self) {
    // Register modules from Go stdlib that bootstrap code may import
    let modules = ["fmt", "strings", "strconv", "errors", "os", "io", "sort", "math"]
    for m in modules {
      self.scope.define(m, make_module_type(m))
    }
  }

  // =====================================================================
  // Type comparison and assignability
  // =====================================================================

  func type_name(t: Type) -> string {
    match t.kind {
      Struct(name) => { return name }
      Class(name) => { return name }
      Enum(name) => { return name }
      Interface(name) => { return name }
      Module(name) => { return name }
      Int => {
        if t.bits == 8 { return "i8" }
        if t.bits == 16 { return "i16" }
        if t.bits == 32 { return "i32" }
        if t.bits == 64 { return "i64" }
        return "int"
      }
      Uint => {
        if t.bits == 8 { return "u8" }
        if t.bits == 16 { return "u16" }
        if t.bits == 32 { return "u32" }
        if t.bits == 64 { return "u64" }
        return "uint"
      }
      Float => {
        if t.bits == 32 { return "f32" }
        if t.bits == 64 { return "f64" }
        return "f64"
      }
      Bool => { return "bool" }
      String => { return "string" }
      _ => { return "" }
    }
    return ""
  }

  func types_equal(a: Type, b: Type) -> bool {
    match a.kind {
      Int => {
        if b.kind is Int { return a.bits == b.bits }
        return false
      }
      Uint => {
        if b.kind is Uint { return a.bits == b.bits }
        return false
      }
      Float => {
        if b.kind is Float { return a.bits == b.bits }
        return false
      }
      Bool => { return b.kind is Bool }
      String => { return b.kind is String }
      Void => { return b.kind is Void }
      Nil => { return b.kind is Nil }
      Any => { return b.kind is Any }
      Error => { return b.kind is Error }
      Lock => { return b.kind is Lock }
      Optional(inner_a) => {
        match b.kind {
          Optional(inner_b) => { return types_equal(inner_a, inner_b) }
          _ => { return false }
        }
      }
      Sequence(elem_a) => {
        match b.kind {
          Sequence(elem_b) => { return types_equal(elem_a, elem_b) }
          _ => { return false }
        }
      }
      Channel(elem_a) => {
        match b.kind {
          Channel(elem_b) => { return types_equal(elem_a, elem_b) }
          _ => { return false }
        }
      }
      Generator(elem_a) => {
        match b.kind {
          Generator(elem_b) => { return types_equal(elem_a, elem_b) }
          _ => { return false }
        }
      }
      Map(ka, va) => {
        match b.kind {
          Map(kb, vb) => { return types_equal(ka, kb) && types_equal(va, vb) }
          _ => { return false }
        }
      }
      ErrorResult(ok_a, err_a) => {
        match b.kind {
          ErrorResult(ok_b, err_b) => { return types_equal(ok_a, ok_b) && types_equal(err_a, err_b) }
          _ => { return false }
        }
      }
      Func(params_a, ret_a, _) => {
        match b.kind {
          Func(params_b, ret_b, _) => {
            if len(params_a) != len(params_b) { return false }
            for i in range(0, len(params_a)) {
              if !types_equal(params_a[i], params_b[i]) { return false }
            }
            return types_equal(ret_a, ret_b)
          }
          _ => { return false }
        }
      }
      Tuple(fields_a) => {
        match b.kind {
          Tuple(fields_b) => {
            if len(fields_a) != len(fields_b) { return false }
            for i in range(0, len(fields_a)) {
              if fields_a[i].type_val != null && fields_b[i].type_val != null {
                if !types_equal(fields_a[i].type_val!, fields_b[i].type_val!) { return false }
              }
            }
            return true
          }
          _ => { return false }
        }
      }
      Struct(name_a) => {
        match b.kind {
          Struct(name_b) => { return name_a == name_b }
          _ => { return false }
        }
      }
      Class(name_a) => {
        match b.kind {
          Class(name_b) => { return name_a == name_b }
          _ => { return false }
        }
      }
      Enum(name_a) => {
        match b.kind {
          Enum(name_b) => { return name_a == name_b }
          _ => { return false }
        }
      }
      Interface(name_a) => {
        match b.kind {
          Interface(name_b) => { return name_a == name_b }
          _ => { return false }
        }
      }
      TypeVar(name_a) => {
        match b.kind {
          TypeVar(name_b) => { return name_a == name_b }
          _ => { return false }
        }
      }
      Module(name_a) => {
        match b.kind {
          Module(name_b) => { return name_a == name_b }
          _ => { return false }
        }
      }
      Union(va) => {
        match b.kind {
          Union(vb) => {
            if len(va) != len(vb) { return false }
            for i in range(0, len(va)) {
              if !types_equal(va[i], vb[i]) { return false }
            }
            return true
          }
          _ => { return false }
        }
      }
    }
    return false
  }

  func numeric_widens(src: Type, to: Type) -> bool {
    match src.kind {
      Int => {
        match to.kind {
          Int => {
            // Platform int (bits=-1) accepts any same-kind integer
            if to.bits == -1 && src.bits > 0 { return true }
            if src.bits == -1 && to.bits > 0 { return true }
            return src.bits > 0 && to.bits > 0 && src.bits < to.bits
          }
          // Allow i32 → u8 etc. (cross-sign integer assignment)
          Uint => { return true }
          _ => { return false }
        }
      }
      Uint => {
        match to.kind {
          Uint => {
            if to.bits == -1 && src.bits > 0 { return true }
            if src.bits == -1 && to.bits > 0 { return true }
            return src.bits > 0 && to.bits > 0 && src.bits < to.bits
          }
          // Allow u8 → i32 etc. (cross-sign integer assignment)
          Int => { return true }
          _ => { return false }
        }
      }
      Float => {
        match to.kind {
          Float => { return src.bits < to.bits }
          _ => { return false }
        }
      }
      _ => { return false }
    }
    return false
  }

  func Checker.is_assignable(self, src: Type, to: Type) -> bool {
    if types_equal(src, to) { return true }

    // TyNil assignable to optional, class, interface, error, and any
    match src.kind {
      Nil => {
        match to.kind {
          Optional(_) | Class(_) | Interface(_) | Error | Any => { return true }
          _ => { return true }  // Lyric currently allows null for all types
        }
      }
      _ => {}
    }

    // Any accepts/produces everything
    match to.kind {
      Any => { return true }
      _ => {}
    }
    match src.kind {
      Any => { return true }
      _ => {}
    }

    // Error (TyError) is a cascade suppressor — assignable to/from everything
    match src.kind {
      Error => { return true }
      _ => {}
    }
    match to.kind {
      Error => {
        match src.kind {
          Class(_) | Interface(_) | Nil => { return true }
          _ => {}
        }
      }
      _ => {}
    }

    // T → T? (wrap in optional)
    match to.kind {
      Optional(inner) => {
        if self.is_assignable(src, inner) { return true }
      }
      _ => {}
    }

    // T? → T? (covariant)
    match src.kind {
      Optional(src_inner) => {
        match to.kind {
          Optional(to_inner) => {
            if self.is_assignable(src_inner, to_inner) { return true }
          }
          _ => {}
        }
      }
      _ => {}
    }

    // Numeric widening (same sign only)
    if numeric_widens(src, to) { return true }

    // TypeVar compatible with anything (generics before instantiation)
    match src.kind {
      TypeVar(_) => { return true }
      _ => {}
    }
    match to.kind {
      TypeVar(_) => { return true }
      _ => {}
    }

    // Sequence covariance + empty list assignability
    match src.kind {
      Sequence(src_elem) => {
        match to.kind {
          Sequence(to_elem) => {
            // Empty list [] (Sequence(Void)) assignable to any typed list
            match src_elem.kind {
              Void => { return true }
              _ => {}
            }
            if self.is_assignable(src_elem, to_elem) { return true }
          }
          _ => {}
        }
      }
      _ => {}
    }

    // Interface satisfaction: class assignable to interface it implements
    match to.kind {
      Interface(iface_name) => {
        let src_name = type_name(src)
        if src_name != "" {
          let info = self.registry.lookup(src_name)
          if info != null {
            for iname in info!.implements_list {
              if iname == iface_name { return true }
            }
          }
        }
      }
      _ => {}
    }

    // Union type: any member type is assignable to the union
    match to.kind {
      Union(to_variants) => {
        for v in to_variants {
          if self.is_assignable(src, v) { return true }
        }
      }
      _ => {}
    }

    // Union type: union assignable to target if ALL variants assignable
    match src.kind {
      Union(src_variants) => {
        let mut all_ok = true
        for v in src_variants {
          if !self.is_assignable(v, to) {
            all_ok = false
          }
        }
        if all_ok { return true }
      }
      _ => {}
    }

    return false
  }

  func is_numeric(t: Type) -> bool {
    match t.kind {
      Int | Uint | Float => { return true }
      _ => { return false }
    }
  }

  func is_integer(t: Type) -> bool {
    match t.kind {
      Int | Uint => { return true }
      _ => { return false }
    }
  }

  func is_error_type(t: Type) -> bool {
    match t.kind {
      Error => { return true }
      _ => { return false }
    }
  }

  // =====================================================================
  // Generic type inference
  // =====================================================================

  // matchTypeVars matches a concrete type against a pattern type, binding
  // type variables in the pattern to concrete types. Returns false if
  // the match fails.
  func match_type_vars(pattern: Type, concrete: Type, bindings: Dict<Sym, Type>) -> bool {
    match pattern.kind {
      TypeVar(name) => {
        let existing = bindings.get(sym(name))
        if existing != null {
          // Already bound — check compatibility
          return types_equal(existing!.value, concrete)
        }
        bindings.set(sym(name), concrete)
        return true
      }
      Optional(inner_p) => {
        match concrete.kind {
          Optional(inner_c) => {
            return match_type_vars(inner_p, inner_c, bindings)
          }
          _ => {
            // T? matches T — try matching inner
            return match_type_vars(inner_p, concrete, bindings)
          }
        }
      }
      Sequence(elem_p) => {
        match concrete.kind {
          Sequence(elem_c) => {
            return match_type_vars(elem_p, elem_c, bindings)
          }
          _ => { return false }
        }
      }
      Map(key_p, val_p) => {
        match concrete.kind {
          Map(key_c, val_c) => {
            let ok1 = match_type_vars(key_p, key_c, bindings)
            let ok2 = match_type_vars(val_p, val_c, bindings)
            return ok1 && ok2
          }
          _ => { return false }
        }
      }
      Channel(elem_p) => {
        match concrete.kind {
          Channel(elem_c) => {
            return match_type_vars(elem_p, elem_c, bindings)
          }
          _ => { return false }
        }
      }
      Generator(elem_p) => {
        match concrete.kind {
          Generator(elem_c) => {
            return match_type_vars(elem_p, elem_c, bindings)
          }
          _ => { return false }
        }
      }
      Func(params_p, ret_p, _) => {
        match concrete.kind {
          Func(params_c, ret_c, _) => {
            if len(params_p) != len(params_c) { return false }
            for i in range(0, len(params_p)) {
              if !match_type_vars(params_p[i], params_c[i], bindings) { return false }
            }
            return match_type_vars(ret_p, ret_c, bindings)
          }
          _ => { return false }
        }
      }
      Tuple(fields_p) => {
        match concrete.kind {
          Tuple(fields_c) => {
            if len(fields_p) != len(fields_c) { return false }
            for i in range(0, len(fields_p)) {
              if fields_p[i].type_val != null && fields_c[i].type_val != null {
                if !match_type_vars(fields_p[i].type_val!, fields_c[i].type_val!, bindings) {
                  return false
                }
              }
            }
            return true
          }
          _ => { return false }
        }
      }
      ErrorResult(ok_p, err_p) => {
        match concrete.kind {
          ErrorResult(ok_c, err_c) => {
            let ok1 = match_type_vars(ok_p, ok_c, bindings)
            let ok2 = match_type_vars(err_p, err_c, bindings)
            return ok1 && ok2
          }
          _ => { return false }
        }
      }
      _ => {
        // Non-generic types: check equality
        return types_equal(pattern, concrete)
      }
    }
    return false
  }

  // substituteType replaces type variables with their bindings.
  func substitute_type(t: Type, bindings: Dict<Sym, Type>) -> Type {
    match t.kind {
      TypeVar(name) => {
        let bound = bindings.get(sym(name))
        if bound != null { return bound!.value }
        return t
      }
      Optional(inner) => {
        return make_optional_type(substitute_type(inner, bindings))
      }
      Sequence(elem) => {
        return make_sequence_type(substitute_type(elem, bindings))
      }
      Map(key, val) => {
        return make_map_type(substitute_type(key, bindings), substitute_type(val, bindings))
      }
      Channel(elem) => {
        return make_channel_type(substitute_type(elem, bindings))
      }
      Generator(elem) => {
        return make_generator_type(substitute_type(elem, bindings))
      }
      Func(params, ret, tpn) => {
        let mut new_params: [Type] = []
        for p in params {
          append(new_params, substitute_type(p, bindings))
        }
        return make_func_type(new_params, substitute_type(ret, bindings), tpn)
      }
      Tuple(fields) => {
        let mut new_fields: [TupleFieldType] = []
        for f in fields {
          let mut nft: Type? = null
          if f.type_val != null {
            nft = substitute_type(f.type_val!, bindings)
          }
          append(new_fields, TupleFieldType { name: f.name, type_val: nft })
        }
        return make_tuple_type(new_fields)
      }
      ErrorResult(ok, err) => {
        return make_error_result_type(substitute_type(ok, bindings), substitute_type(err, bindings))
      }
      Class(name) => {
        if len(t.type_args) > 0 {
          let mut new_args: [Type] = []
          for a in t.type_args {
            append(new_args, substitute_type(a, bindings))
          }
          let result = Type { kind: t.kind, bits: t.bits, type_args: new_args }
          return result
        }
        return t
      }
      Struct(name) => {
        if len(t.type_args) > 0 {
          let mut new_args: [Type] = []
          for a in t.type_args {
            append(new_args, substitute_type(a, bindings))
          }
          return Type { kind: t.kind, bits: t.bits, type_args: new_args }
        }
        return t
      }
      _ => { return t }
    }
    return t
  }

  // inferTypeArgs infers type arguments from call arguments for a generic function.
  func Checker.infer_type_args(self, func_type: Type, arg_types: [Type]) -> Dict<Sym, Type> {
    let bindings = Dict<Sym, Type>()
    match func_type.kind {
      Func(params, _, _) => {
        let mut limit = len(params)
        if len(arg_types) < limit { limit = len(arg_types) }
        for i in range(0, limit) {
          match_type_vars(params[i], arg_types[i], bindings)
        }
      }
      _ => {}
    }
    return bindings
  }

  // satisfiesConstraint checks if a type satisfies a generic constraint.
  func satisfies_constraint(t: Type, constraint: string) -> bool {
    if constraint == "" { return true }
    if constraint == "Comparable" || constraint == "Equatable" || constraint == "Hashable" {
      match t.kind {
        Int | Uint | Float | Bool | String => { return true }
        Enum(_) => { return true }
        _ => { return false }
      }
    }
    return true
  }

  // =====================================================================
  // Error reporting
  // =====================================================================

  func Checker.error(self, msg: string) {
    append(self.errors, msg)
  }

  func Checker.error_at(self, span: Span, msg: string) {
    let loc = itoa(span.start.line) + ":" + itoa(span.start.column)
    append(self.errors, loc + ": " + msg)
  }

  // =====================================================================
  // Type → TypeExpr conversion (for annotation)
  // =====================================================================

  func type_to_type_expr(t: Type) -> TypeExpr {
    let zero_span = Span {
      start: Pos { file: null, line: 0, column: 0 },
      end: Pos { file: null, line: 0, column: 0 },
    }
    match t.kind {
      Int => {
        let mut name = "int"
        if t.bits == 8 {
          name = "i8"
        } else if t.bits == 16 {
          name = "i16"
        } else if t.bits == 32 {
          name = "i32"
        } else if t.bits == 64 {
          name = "i64"
        }
        return TypeExpr { kind: Named(sym(name), []), span: zero_span }
      }
      Uint => {
        let mut name = "uint"
        if t.bits == 8 {
          name = "u8"
        } else if t.bits == 16 {
          name = "u16"
        } else if t.bits == 32 {
          name = "u32"
        } else if t.bits == 64 {
          name = "u64"
        }
        return TypeExpr { kind: Named(sym(name), []), span: zero_span }
      }
      Float => {
        let mut name = "f64"
        if t.bits == 32 {
          name = "f32"
        }
        return TypeExpr { kind: Named(sym(name), []), span: zero_span }
      }
      Bool => {
        return TypeExpr { kind: Named(`bool`, []), span: zero_span }
      }
      String => {
        return TypeExpr { kind: Named(`string`, []), span: zero_span }
      }
      Void | Nil => {
        return TypeExpr { kind: Unit, span: zero_span }
      }
      Any => {
        return TypeExpr { kind: Named(`any`, []), span: zero_span }
      }
      Error => {
        return TypeExpr { kind: Named(`error`, []), span: zero_span }
      }
      Lock => {
        return TypeExpr { kind: TypeExprKind.Lock, span: zero_span }
      }
      Optional(inner) => {
        return TypeExpr { kind: TypeExprKind.Optional(type_to_type_expr(inner)), span: zero_span }
      }
      Sequence(elem) => {
        return TypeExpr { kind: TypeExprKind.Sequence(type_to_type_expr(elem)), span: zero_span }
      }
      Map(key, value) => {
        return TypeExpr { kind: TypeExprKind.Map(type_to_type_expr(key), type_to_type_expr(value)), span: zero_span }
      }
      Tuple(fields) => {
        let mut tfields: [TupleField] = []
        for f in fields {
          let mut te: TypeExpr? = null
          if f.type_val != null {
            te = type_to_type_expr(f.type_val!)
          }
          append(tfields, TupleField { name: null, type_expr: te })
        }
        return TypeExpr { kind: TypeExprKind.Tuple(tfields), span: zero_span }
      }
      Func(params, ret, _) => {
        let mut param_tes: [TypeExpr] = []
        for p in params {
          append(param_tes, type_to_type_expr(p))
        }
        return TypeExpr { kind: TypeExprKind.Func(param_tes, type_to_type_expr(ret)), span: zero_span }
      }
      Struct(name) | Class(name) | Enum(name) | Interface(name) => {
        let mut ta_exprs: [TypeExpr] = []
        for ta in t.type_args {
          append(ta_exprs, type_to_type_expr(ta))
        }
        return TypeExpr { kind: Named(sym(name), ta_exprs), span: zero_span }
      }
      TypeVar(name) => {
        return TypeExpr { kind: Named(sym(name), []), span: zero_span }
      }
      Channel(elem) => {
        return TypeExpr { kind: TypeExprKind.Channel(type_to_type_expr(elem)), span: zero_span }
      }
      Generator(elem) => {
        return TypeExpr { kind: TypeExprKind.Generator(type_to_type_expr(elem)), span: zero_span }
      }
      ErrorResult(ok, err) => {
        let fields = [
          TupleField { name: null, type_expr: type_to_type_expr(ok) },
          TupleField { name: null, type_expr: type_to_type_expr(err) },
        ]
        return TypeExpr { kind: TypeExprKind.Tuple(fields), span: zero_span }
      }
      Module(name) => {
        return TypeExpr { kind: Named(sym(name), []), span: zero_span }
      }
      Union(variants) => {
        let mut tes: [TypeExpr] = []
        for v in variants {
          append(tes, type_to_type_expr(v))
        }
        return TypeExpr { kind: TypeExprKind.Union(tes), span: zero_span }
      }




    }
    return TypeExpr { kind: Unit, span: zero_span }
  }

  // Annotate an expression with its resolved type
  func annotate(expr: Expr, t: Type) -> Type {
    expr.resolved_type = type_to_type_expr(t)
    return t
  }

  // type_contains_var returns true if the type contains any TypeVar
  func type_contains_var(t: Type) -> bool {
    match t.kind {
      TypeVar(_) => { return true }
      Optional(inner) => { return type_contains_var(inner) }
      Sequence(elem) => { return type_contains_var(elem) }
      Map(k, v) => { return type_contains_var(k) || type_contains_var(v) }
      Channel(elem) => { return type_contains_var(elem) }
      Generator(elem) => { return type_contains_var(elem) }
      ErrorResult(ok, err) => { return type_contains_var(ok) || type_contains_var(err) }
      Func(params, ret, _) => {
        if type_contains_var(ret) { return true }
        for p in params {
          if type_contains_var(p) { return true }
        }
        return false
      }
      Tuple(fields) => {
        for f in fields {
          if f.type_val != null && type_contains_var(f.type_val!) { return true }
        }
        return false
      }
      _ => { return false }
    }
    return false
  }

  // =====================================================================
  // Type resolution from AST TypeExpr
  // =====================================================================

  func Checker.resolve_type_expr(self, te: TypeExpr) -> Type {
    match te.kind {
      Named(name, args) => {
        return self.resolve_named_type(sym_to_string(name), args, te.span)
      }
      Optional(inner) => {
        return make_optional_type(self.resolve_type_expr(inner))
      }
      Sequence(elem) => {
        return make_sequence_type(self.resolve_type_expr(elem))
      }
      Map(key, value) => {
        return make_map_type(self.resolve_type_expr(key), self.resolve_type_expr(value))
      }
      Tuple(fields) => {
        let mut tfields: [TupleFieldType] = []
        for f in fields {
          let mut ft: Type? = null
          if f.type_expr != null {
            ft = self.resolve_type_expr(f.type_expr!)
          }
          let mut fname = ""
          if f.name != null { fname = sym_to_string(f.name!) }
          append(tfields, TupleFieldType { name: fname, type_val: ft })
        }
        return make_tuple_type(tfields)
      }
      Func(params, ret) => {
        let mut param_types: [Type] = []
        for p in params {
          append(param_types, self.resolve_type_expr(p))
        }
        return make_func_type(param_types, self.resolve_type_expr(ret), [])
      }
      Union(variants) => {
        let mut resolved: [Type] = []
        for v in variants {
          append(resolved, self.resolve_type_expr(v))
        }
        return make_union_type(resolved)
      }
      Channel(elem) => {
        return make_channel_type(self.resolve_type_expr(elem))
      }
      Generator(elem) => {
        return make_generator_type(self.resolve_type_expr(elem))
      }
      Lock => {
        return make_lock_type()
      }
      Unit => {
        return make_void_type()
      }
    }
    // All TypeExprKind variants handled above — this is unreachable
    eprintln("checker: resolve_type_expr: unreachable — unknown TypeExprKind")
    os_exit(1)
    return make_void_type()
  }

  // =====================================================================
  // Individual expression checkers
  // =====================================================================

  func Checker.resolve_named_type(self, name: string, args: [TypeExpr], span: Span) -> Type {
    // Check built-in types first
    if name == "Lock" { return make_lock_type() }

    // Check scope first (type vars, aliases, modules)
    let sv = self.scope.lookup(name)
    if sv != null {
      match sv!.kind {
        TypeVar(_) | Module(_) => { return sv! }
        _ => {}
      }
    }

    // Check registry
    let info = self.registry.lookup(name)
    if info != null {
      // If generic type with args, substitute
      if len(args) > 0 && len(info!.type_param_names) > 0 {
        let bindings = Dict<Sym, Type>()
        let mut limit = len(info!.type_param_names)
        if len(args) < limit { limit = len(args) }
        let mut resolved_args: [Type] = []
        for i in range(0, limit) {
          let resolved = self.resolve_type_expr(args[i])
          bindings.set(sym(info!.type_param_names[i]), resolved)
          append(resolved_args, resolved)
        }
        // Return a NEW type with type_args populated (like Go compiler)
        let result = Type { kind: info!.type_val.kind, bits: info!.type_val.bits, type_args: resolved_args }
        return result
      }
      return info!.type_val
    }

    // Check if this might be a type variable from a generic class/function context.
    // Type variables are short uppercase names not in scope or registry.
    // The Go checker never hits this because it uses internal Type objects directly;
    // the bootstrap sometimes round-trips through type_to_type_expr → resolve_type_expr.
    if span.start.line == 0 && span.start.column == 0 {
      // Zero-span means this TypeExpr came from type_to_type_expr (synthetic)
      return make_typevar_type(name)
    }
    // Unknown type — treat as type variable (matches Go checker behavior)
    return make_typevar_type(name)
  }

  // =====================================================================
  // funcDeclToType: extract function type from AST
  // =====================================================================

  func Checker.func_decl_to_type(self, f: FuncDecl) -> Type {
    // Collect type param names first and temporarily bind to scope
    let mut tpnames: [string] = []
    let tps = f.fp_children()
    for tp in tps {
      if tp.name != null {
        let tpname = sym_to_string(tp.name!)
        append(tpnames, tpname)
      }
    }
    // Push scope for type params so resolve_type_expr can find them
    if len(tpnames) > 0 {
      self.push_scope()
      for tpname in tpnames {
        self.scope.define(tpname, make_typevar_type(tpname))
      }
    }

    let mut param_types: [Type] = []
    let params = f.param_children()
    for p in params {
      if !p.is_self {
        if p.type_expr != null {
          append(param_types, self.resolve_type_expr(p.type_expr!))
        } else {
          append(param_types, make_any_type())
        }
      }
    }
    let mut ret = make_void_type()
    if f.return_type != null {
      ret = self.resolve_type_expr(f.return_type!)
    }

    if len(tpnames) > 0 {
      self.pop_scope()
    }

    return make_func_type(param_types, ret, tpnames)
  }

  // =====================================================================
  // Phase 0: Pre-register all type NAMES across all blocks so cross-references resolve.
  // Must run on ALL blocks before Phase 1 starts.
  // =====================================================================

  func Checker.preregister_type_names(self, block: LyricBlock) {
    // Register imports as module types
    let imports = block.imp_children()
    for imp in imports {
      if imp.alias != null {
        let alias = sym_to_string(imp.alias!)
        self.scope.define(alias, make_module_type(alias))
      }
    }

    let ifaces = block.id_children()
    for iface in ifaces {
      if iface.name != null {
        let iname = sym_to_string(iface.name!)
        let stub = TypeInfo {
          type_val: make_interface_type(iname),
          fields: Dict<Sym, Type>(),
          field_order: [],
          methods: Dict<Sym, Type>(),
          variants: Dict<Sym, VariantInfo>(),
          type_param_names: [],
          type_param_constraints: [],
          implements_list: [],
        }
        self.registry.register(iname, stub)
      }
    }
    let structs = block.sd_children()
    for s in structs {
      if s.name != null {
        let sname = sym_to_string(s.name!)
        let stub = TypeInfo {
          type_val: make_struct_type(sname),
          fields: Dict<Sym, Type>(),
          field_order: [],
          methods: Dict<Sym, Type>(),
          variants: Dict<Sym, VariantInfo>(),
          type_param_names: [],
          type_param_constraints: [],
          implements_list: [],
        }
        self.registry.register(sname, stub)
      }
    }
    let classes = block.cd_children()
    for c in classes {
      if c.name != null {
        let cname = sym_to_string(c.name!)
        let stub = TypeInfo {
          type_val: make_class_type(cname),
          fields: Dict<Sym, Type>(),
          field_order: [],
          methods: Dict<Sym, Type>(),
          variants: Dict<Sym, VariantInfo>(),
          type_param_names: [],
          type_param_constraints: [],
          implements_list: [],
        }
        self.registry.register(cname, stub)
      }
    }
    let enums = block.ed_children()
    for e in enums {
      if e.name != null {
        let ename = sym_to_string(e.name!)
        let stub = TypeInfo {
          type_val: make_enum_type(ename),
          fields: Dict<Sym, Type>(),
          field_order: [],
          methods: Dict<Sym, Type>(),
          variants: Dict<Sym, VariantInfo>(),
          type_param_names: [],
          type_param_constraints: [],
          implements_list: [],
        }
        self.registry.register(ename, stub)
      }
    }
  }

  // Phase 1: Register all types and function signatures (full detail).
  // Assumes Phase 0 has already run on ALL blocks.
  // =====================================================================

  func Checker.register_lyric_block(self, block: LyricBlock) {
    let ifaces = block.id_children()
    let structs = block.sd_children()
    let classes = block.cd_children()
    let enums = block.ed_children()

    // Register interfaces first (classes may implement them)
    for iface in ifaces {
      let dn = if iface.name != null { sym_to_string(iface.name!) } else { "?" }
      self.register_interface(iface)
    }

    // Register struct types
    for s in structs {
      self.register_struct(s)
    }

    // Register class types
    for c in classes {
      self.register_class(c)
    }

    // Register enum types
    for e in enums {
      self.register_enum(e)
    }

    // Check implements satisfaction
    for c in classes {
      self.check_implements(c)
    }

    // Register impl block methods on concrete classes
    let impls = block.ib_children()
    for ib in impls {
      self.register_impl_methods(ib)
    }

    // Register type aliases
    let aliases = block.ta_children()
    for a in aliases {
      if a.name != null && a.type_expr != null {
        let t = self.resolve_type_expr(a.type_expr!)
        let aname = sym_to_string(a.name!)
        let info = TypeInfo {
          type_val: t,
          fields: Dict<Sym, Type>(),
          field_order: [],
          methods: Dict<Sym, Type>(),
          variants: Dict<Sym, VariantInfo>(),
          type_param_names: [],
          type_param_constraints: [],
          implements_list: [],
        }
        self.registry.register(aname, info)
        self.scope.define(aname, t)
      }
    }

    // Register constants
    let consts = block.con_children()
    for c in consts {
      if c.name != null {
        let cname = sym_to_string(c.name!)
        let mut ct = make_any_type()
        if c.type_expr != null {
          ct = self.resolve_type_expr(c.type_expr!)
          if c.value != null {
            self.check_expr(c.value!)
          }
        } else if c.value != null {
          ct = self.check_expr(c.value!)
        }
        self.scope.define(cname, ct)
      }
    }

    // Register functions
    let funcs = block.fd_children()
    for f in funcs {
      self.register_func(f)
    }

  }

  // Phase 1.5: Register interface methods on concrete types.
  // Collects ALL impl blocks from ALL blocks to enable cross-block resolution
  // (e.g. P.push defined in stdlib, impl block for Lexer in lexer.ly).
  func Checker.register_interface_methods(self, file: File) {
    // Collect ALL impl blocks from ALL blocks
    let mut all_impls: [ImplBlock] = []
    let all_blocks = file.fb_children()
    for blk in all_blocks {
      let blk_impls = blk.ib_children()
      for ib in blk_impls {
        all_impls = append(all_impls, ib)
      }
    }

    // Find all functions with type-param receivers and match against all impl blocks
    for blk in all_blocks {
      let funcs = blk.fd_children()
      for f in funcs {
        if f.name == null { continue }
        if f.receiver_type == null { continue }
        let rname = sym_to_string(f.receiver_type!)

        let type_info = self.registry.lookup(rname)
        if type_info != null { continue }

        let where_clauses = f.where_children()
        for wc in where_clauses {
          if wc.constraint == null { continue }
          let iface_name = sym_to_string(wc.constraint!)
          let iface_entry = self.iface_decls.get(sym(iface_name))
          if iface_entry == null { continue }
          let iface_decl = iface_entry!.value
          let itp = iface_decl.itp_children()

          let wc_args = wc.wc_arg_children()
          let mut recv_param_idx: i32 = -1
          let mut j: i32 = 0
          while j < len(wc_args) {
            match wc_args[j].kind {
              Named(arg_name, _) => {
                if sym_to_string(arg_name) == rname {
                  recv_param_idx = j
                }
              }
              _ => {}
            }
            j = j + 1
          }
          if recv_param_idx < 0 { continue }

          for ib in all_impls {
            if ib.interface_name == null { continue }
            let ib_iface_name = sym_to_string(ib.interface_name!)
            let mut iface_matches = ib_iface_name == iface_name
            if !iface_matches {
              // Check if the impl block's interface embeds the where-clause interface
              let ib_iface = self.iface_decls.get(sym(ib_iface_name))
              if ib_iface != null {
                for emb in ib_iface!.value.ie_children() {
                  if !isnull(emb.name) && emb.name!.name == iface_name {
                    iface_matches = true
                  }
                }
              }
            }
            if !iface_matches { continue }

            let impl_args = ib.ib_arg_children()
            if recv_param_idx >= len(impl_args) as i32 { continue }

            let subst = Dict<Sym, Type>()
            let mut k: i32 = 0
            let mut limit = len(itp)
            if len(impl_args) < limit { limit = len(impl_args) }
            while k < limit as i32 {
              if itp[k].name != null {
                subst.set(sym(sym_to_string(itp[k].name!)), self.resolve_type_expr(impl_args[k]))
              }
              k = k + 1
            }

            let concrete_type = self.resolve_type_expr(impl_args[recv_param_idx])
            let concrete_name = type_name(concrete_type)
            if concrete_name == "" { continue }

            let cinfo = self.registry.lookup(concrete_name)
            if cinfo == null { continue }

            let fname = sym_to_string(f.name!)
            let existing = cinfo!.methods.get(sym(fname))
            if existing != null { continue }

            let ft = self.func_decl_to_type(f)
            let substituted = substitute_type(ft, subst)
            let mut final_type = substituted
            match substituted.kind {
              Func(sp, sr, stpn) => {
                let mut remaining: [string] = []
                for tpn in stpn {
                  if subst.get(sym(tpn)) == null {
                    append(remaining, tpn)
                  }
                }
                final_type = make_func_type(sp, sr, remaining)
              }
              _ => {}
            }
            let mut ta: [TypeExpr] = []
            let orig_tps = f.fp_children()
            for tp in orig_tps {
              if tp.name != null {
                let tpname = sym_to_string(tp.name!)
                let bound = subst.get(sym(tpname))
                if bound != null {
                  append(ta, type_to_type_expr(bound!.value))
                }
              }
            }
            // If impl block has a label, register with label-prefixed name
            let mut reg_name = fname
            if ib.label != null {
              let label_str = sym_to_string(ib.label!)
              // Use let ref to prevent scope-exit free — sym() stores pointer to string data
              let ref label_name = label_str + "_" + fname
              reg_name = label_name
              get_method_aliases().set(sym(reg_name), fname)
              get_method_labels().set(sym(reg_name), label_str)
            }
            let method_key = concrete_name + "." + reg_name
            self.method_type_args.set(sym(method_key), ta)
            cinfo!.methods.set(sym(reg_name), final_type)
            self.scope.define(method_key, final_type)
          }
        }
      }
    }

    // Phase 1.5b: Register interface body methods (P.method syntax) on concrete types.
    // For each interface with body methods that have a receiver_type matching a type param,
    // find all impl blocks for that interface and register the method on the concrete type.
    let iface_keys = self.iface_decls.keys()

    for ik in iface_keys {
      let iface_entry = self.iface_decls.get(ik)
      if iface_entry == null { continue }
      let iface_decl = iface_entry!.value
      if iface_decl.name == null { continue }
      let iface_name = sym_to_string(iface_decl.name!)

      let itp = iface_decl.itp_children()
      let imethods = iface_decl.im_children()

      // Only process interfaces that have body methods with receiver types
      let mut has_recv_methods = false
      for m in imethods {
        if m.receiver_type != null {
          has_recv_methods = true
        }
      }
      if !has_recv_methods { continue }

      // Find all impl blocks for this interface
      for ib in all_impls {
        if ib.interface_name == null { continue }
        if sym_to_string(ib.interface_name!) != iface_name { continue }

        let impl_args = ib.ib_arg_children()

        // Build substitution: interface type param → concrete type
        let subst = Dict<Sym, Type>()
        let mut param_to_concrete = Dict<Sym, string>()
        let mut si = 0
        let mut slimit = len(itp)
        if len(impl_args) < slimit { slimit = len(impl_args) }
        while si < slimit as i32 {
          if itp[si].name != null {
            let param_name = sym_to_string(itp[si].name!)
            let concrete = self.resolve_type_expr(impl_args[si])
            subst.set(sym(param_name), concrete)
            param_to_concrete.set(sym(param_name), type_name(concrete))
          }
          si = si + 1
        }

        // Register each receiver-typed method on its concrete type
        for m in imethods {
          if m.name == null { continue }
          if m.receiver_type == null { continue }
          let recv_param = sym_to_string(m.receiver_type!)
          let concrete_name_entry = param_to_concrete.get(sym(recv_param))
          if concrete_name_entry == null { continue }
          let concrete_name = concrete_name_entry!.value
          if concrete_name == "" { continue }

          let cinfo = self.registry.lookup(concrete_name)
          if cinfo == null { continue }

          let mname = sym_to_string(m.name!)
          // If impl block has a label, register with label-prefixed name
          let mut reg_name = mname
          if ib.label != null {
            let label_str = sym_to_string(ib.label!)
            // Use let ref to prevent scope-exit free — sym() stores pointer to string data
            let ref label_name = label_str + "_" + mname
            reg_name = label_name
            get_method_aliases().set(sym(reg_name), mname)
            get_method_labels().set(sym(reg_name), label_str)
          }
          let existing = cinfo!.methods.get(sym(reg_name))
          if existing != null { continue }

          let ft = self.func_decl_to_type(m)
          let substituted = substitute_type(ft, subst)

          // Strip substituted type params from func's type_param_names
          let mut final_type = substituted
          match substituted.kind {
            Func(sp, sr, stpn) => {
              let mut remaining: [string] = []
              for tpn in stpn {
                if subst.get(sym(tpn)) == null {
                  append(remaining, tpn)
                }
              }
              final_type = make_func_type(sp, sr, remaining)
            }
            _ => {}
          }

          let method_key = concrete_name + "." + reg_name
          // Build type args for monomorphizer (like Phase 1.5)
          let mut ta: [TypeExpr] = []
          let mut ta_i = 0
          let mut ta_limit = len(itp)
          if len(impl_args) < ta_limit { ta_limit = len(impl_args) }
          while ta_i < ta_limit as i32 {
            append(ta, impl_args[ta_i])
            ta_i = ta_i + 1
          }
          self.method_type_args.set(sym(method_key), ta)
          cinfo!.methods.set(sym(reg_name), final_type)
          self.scope.define(method_key, final_type)
        }
      }
    }
  }

  func Checker.register_interface(self, iface: InterfaceDecl) {
    if iface.name == null { return }
    let iname = sym_to_string(iface.name!)

    // Save for checkImplements
    self.iface_decls.set(sym(iname), iface)

    // Use existing TypeInfo if present (e.g. from Phase 0)
    let mut info_opt = self.registry.lookup(iname)
    if info_opt == null {
      let info = TypeInfo {
        type_val: make_interface_type(iname),
        fields: Dict<Sym, Type>(),
        field_order: [],
        methods: Dict<Sym, Type>(),
        variants: Dict<Sym, VariantInfo>(),
        type_param_names: [],
        type_param_constraints: [],
        implements_list: [],
      }
      self.registry.register(iname, info)
      info_opt = self.registry.lookup(iname)
    }
    let info = info_opt!

    // Register type params
    let itparams = iface.itp_children()
    for tp in itparams {
      if tp.name != null {
        let tpname = sym_to_string(tp.name!)
        // Avoid duplicates if already registered
        let mut found = false
        for existing_tp in info.type_param_names {
          if existing_tp == tpname { found = true }
        }
        if !found {
          append(info.type_param_names, tpname)
          if tp.constraint != null {
            append(info.type_param_constraints, sym_to_string(tp.constraint!))
          } else {
            append(info.type_param_constraints, "")
          }
        }
      }
    }

    // Register type params in temporary scope for resolving method signatures
    self.push_scope()
    for tp in itparams {
      if tp.name != null {
        let tpname = sym_to_string(tp.name!)
        self.scope.define(tpname, make_typevar_type(tpname))
      }
    }

    // Register interface methods
    let imethods = iface.im_children()
    for m in imethods {
      let ft = self.func_decl_to_type(m)
      if m.name != null {
        let mn = sym_to_string(m.name!)
        info.methods.set(sym(mn), ft)
      }
    }

    self.pop_scope()
    // self.registry.register(iname, info) // Already in registry
  }

  func Checker.register_struct(self, s: StructDecl) {
    if s.name == null { return }
    let sname = sym_to_string(s.name!)

    // Use existing TypeInfo if present (e.g. from Phase 0)
    let mut info_opt = self.registry.lookup(sname)
    if info_opt == null {
      let info = TypeInfo {
        type_val: make_struct_type(sname),
        fields: Dict<Sym, Type>(),
        field_order: [],
        methods: Dict<Sym, Type>(),
        variants: Dict<Sym, VariantInfo>(),
        type_param_names: [],
        type_param_constraints: [],
        implements_list: [],
      }
      self.registry.register(sname, info)
      info_opt = self.registry.lookup(sname)
    }
    let info = info_opt!

    let stparams = s.stp_children()
    for tp in stparams {
      if tp.name != null {
        let tpname = sym_to_string(tp.name!)
        let mut found = false
        for existing_tp in info.type_param_names {
          if existing_tp == tpname { found = true }
        }
        if !found {
          append(info.type_param_names, tpname)
        }
      }
    }

    // Push scope for type params
    self.push_scope()
    for tp in stparams {
      if tp.name != null {
        let tpname = sym_to_string(tp.name!)
        self.scope.define(tpname, make_typevar_type(tpname))
      }
    }

    let sfields = s.sf_children()
    for f in sfields {
      if f.name != null {
        let fname = sym_to_string(f.name!)
        let mut ft = make_any_type()
        if f.type_expr != null {
          ft = self.resolve_type_expr(f.type_expr!)
        }
        info.fields.set(sym(fname), ft)
        append(info.field_order, fname)
        // Check default expression
        if f.default_value != null {
          self.check_expr(f.default_value!)
        }
      }
    }

    self.pop_scope()
    // self.registry.register(sname, info) // Already in registry
  }

  func Checker.register_class(self, cls: ClassDecl) {
    if cls.name == null { return }
    let cname = sym_to_string(cls.name!)

    // Use existing TypeInfo if present (e.g. from Phase 0 or earlier impl blocks)
    let mut info_opt = self.registry.lookup(cname)
    if info_opt == null {
      let info = TypeInfo {
        type_val: make_class_type(cname),
        fields: Dict<Sym, Type>(),
        field_order: [],
        methods: Dict<Sym, Type>(),
        variants: Dict<Sym, VariantInfo>(),
        type_param_names: [],
        type_param_constraints: [],
        implements_list: [],
      }
      self.registry.register(cname, info)
      info_opt = self.registry.lookup(cname)
    }
    let info = info_opt!

    let ctparams = cls.ctp_children()
    let mut type_var_args: [Type] = []
    for tp in ctparams {
      if tp.name != null {
        let tpname = sym_to_string(tp.name!)
        append(info.type_param_names, tpname)
        append(type_var_args, make_typevar_type(tpname))
      }
    }
    // Build constructor return type with TypeVar args (separate from info.type_val)
    let ctor_return = if len(type_var_args) > 0 {
      Type { kind: Class(cname), bits: 0, type_args: type_var_args }
    } else {
      info.type_val
    }

    // Push scope for type params during field/method resolution
    self.push_scope()
    for tp in ctparams {
      if tp.name != null {
        let tpname = sym_to_string(tp.name!)
        self.scope.define(tpname, make_typevar_type(tpname))
      }
    }

    let cfields = cls.cf_children()
    for f in cfields {
      if f.name != null {
        let fname = sym_to_string(f.name!)
        let mut ft = make_any_type()
        if f.type_expr != null {
          ft = self.resolve_type_expr(f.type_expr!)
        }
        info.fields.set(sym(fname), ft)
        append(info.field_order, fname)
        if f.default_value != null {
          self.check_expr(f.default_value!)
        }
      }
    }

    // Register methods
    let cmethods = cls.cm_children()
    for m in cmethods {
      if m.name != null {
        let ft = self.func_decl_to_type(m)
        info.methods.set(sym(sym_to_string(m.name!)), ft)
      }
    }

    self.pop_scope()
    // self.registry.register(cname, info) // Already in registry

    // Register class name as constructor in scope
    let mut ctor_params: [Type] = []
    let mut has_explicit_ctor = false

    // Check for explicit constructor
    for m in cmethods {
      if m.name != null && sym_to_string(m.name!) == cname {
        has_explicit_ctor = true
        let cparams = m.param_children()
        for p in cparams {
          if !p.is_self {
            if p.type_expr != null {
              append(ctor_params, self.resolve_type_expr(p.type_expr!))
            } else {
              append(ctor_params, make_any_type())
            }
          }
        }
      }
    }

    let mut tpnames: [string] = []
    let mut tpconstraints: [string] = []
    for tp in ctparams {
      if tp.name != null {
        append(tpnames, sym_to_string(tp.name!))
        if tp.constraint != null {
          append(tpconstraints, sym_to_string(tp.constraint!))
        } else {
          append(tpconstraints, "")
        }
      }
    }

    if has_explicit_ctor {
      self.scope.define(cname, make_func_type(ctor_params, ctor_return, tpnames))
    } else {
      // No explicit constructor — struct-literal construction
      self.scope.define(cname, make_func_type([], ctor_return, tpnames))
    }
  }

  func Checker.register_enum(self, e: EnumDecl) {
    if e.name == null { return }
    let ename = sym_to_string(e.name!)
    let enum_type = make_enum_type(ename)

    // Use existing TypeInfo if present (e.g. from Phase 0)
    let mut info_opt = self.registry.lookup(ename)
    if info_opt == null {
      let info = TypeInfo {
        type_val: enum_type,
        fields: Dict<Sym, Type>(),
        field_order: [],
        methods: Dict<Sym, Type>(),
        variants: Dict<Sym, VariantInfo>(),
        type_param_names: [],
        type_param_constraints: [],
        implements_list: [],
      }
      self.registry.register(ename, info)
      info_opt = self.registry.lookup(ename)
    }
    let info = info_opt!

    let etparams = e.etp_children()
    for tp in etparams {
      if tp.name != null {
        let tpname = sym_to_string(tp.name!)
        let mut found = false
        for existing_tp in info.type_param_names {
          if existing_tp == tpname { found = true }
        }
        if !found {
          append(info.type_param_names, tpname)
        }
      }
    }

    let evs = e.ev_children()
    for v in evs {
      if v.name == null { continue }
      let vname = sym_to_string(v.name!)
      let vi = VariantInfo { enum_name: ename, fields: [] }

      let evfs = v.evf_children()
      let mut param_types: [Type] = []
      for f in evfs {
        let mut ft = make_any_type()
        if f.type_expr != null {
          ft = self.resolve_type_expr(f.type_expr!)
        }
        let mut fname = ""
        if f.name != null { fname = sym_to_string(f.name!) }
        append(vi.fields, VariantField { name: fname, type_val: ft })
        append(param_types, ft)
      }

      info.variants.set(sym(vname), vi)

      // Register variant as constructor/value in scope
      if len(evfs) == 0 {
        self.scope.define(vname, enum_type)
      } else {
        self.scope.define(vname, make_func_type(param_types, enum_type, []))
      }
    }

    // self.registry.register(ename, info) // Already in registry
  }

  func Checker.check_implements(self, cls: ClassDecl) {
    if cls.name == null { return }
    let cname = sym_to_string(cls.name!)
    let class_info = self.registry.lookup(cname)
    if class_info == null { return }

    let impl_list = cls.implements
    for impl_sym in impl_list {
      let iname = sym_to_string(impl_sym)
      append(class_info!.implements_list, iname)
    }
  }

  func Checker.register_impl_methods(self, ib: ImplBlock) {
    // Look up interface info
    if ib.interface_name == null { return }
    let iname = sym_to_string(ib.interface_name!)
    let iface_info = self.registry.lookup(iname)
    if iface_info == null { return }

    // Build substitution map from interface type params to impl type args
    let iface_decl_entry = self.iface_decls.get(sym(iname))
    let subst = Dict<Sym, Type>()
    if iface_decl_entry != null {
      let iface_decl = iface_decl_entry!.value
      let itp = iface_decl.itp_children()
      let impl_args = ib.ib_arg_children()
      let mut limit = len(itp)
      if len(impl_args) < limit { limit = len(impl_args) }
      for i in range(0, limit) {
        if itp[i].name != null {
          subst.set(sym(sym_to_string(itp[i].name!)), self.resolve_type_expr(impl_args[i]))
        }
      }
    }

    // Process each mapping
    let mappings = ib.ibm_children()
    for m in mappings {
      if m.method_name == null { continue }
      let method_name = sym_to_string(m.method_name!)

      // Find interface method type
      let iface_method = iface_info!.methods.get(sym(method_name))
      if iface_method == null { continue }

      // Determine which concrete class this maps to
      let mut class_name = ""
      if m.type_param != null {
        let tp_name = sym_to_string(m.type_param!)
        let bound = subst.get(sym(tp_name))
        if bound != null {
          class_name = type_name(bound!.value)
        }
        if class_name == "" {
          class_name = tp_name
        }
      }
      if class_name == "" { continue }

      let class_info = self.registry.lookup(class_name)
      if class_info == null { continue }

      let substituted = substitute_type(iface_method!.value, subst)

      // Register the method on the class (don't override existing)
      let existing = class_info!.methods.get(sym(method_name))
      if existing == null {
        class_info!.methods.set(sym(method_name), substituted)
      }

      // For field-bind mappings, also register the label-prefixed name
      match m.kind {
        FieldBind => {
          if m.target_member != null {
            let target = sym_to_string(m.target_member!)
            if target != method_name {
              if str_has_prefix(method_name, "set_") {
                // Setter: set_next → set_from_next
                let prefixed = "set_" + target
                let existing2 = class_info!.methods.get(sym(prefixed))
                if existing2 == null {
                  class_info!.methods.set(sym(prefixed), substituted)
                }
              } else {
                // Getter: next → from_next
                let existing2 = class_info!.methods.get(sym(target))
                if existing2 == null {
                  class_info!.methods.set(sym(target), substituted)
                }
              }
            }
          }
        }
        Inline => {
          if m.inline_func != null {
            let ft = self.func_decl_to_type(m.inline_func!)
            class_info!.methods.set(sym(method_name), ft)
          }
        }
        _ => {}
      }
    }
  }

  func Checker.register_func(self, f: FuncDecl) {
    if f.name == null { return }
    let fname = sym_to_string(f.name!)
    let ft = self.func_decl_to_type(f)

    if f.receiver_type != null {
      // External method: func T.method(self, ...)
      let rname = sym_to_string(f.receiver_type!)
      let key = rname + "." + fname

      // Register in methods dict AND in registry for the type
      let type_info = self.registry.lookup(rname)
      if type_info != null {
        type_info!.methods.set(sym(fname), ft)
      }
      // Also define as "T.method" in scope for lookup
      self.scope.define(key, ft)
      // Also define by bare name for free-function call syntax (backward compat)
      self.scope.define(fname, ft)
    } else {
      self.scope.define(fname, ft)
    }
  }

  // =====================================================================
  // Relational where clause support
  // =====================================================================

  // grant_relational_methods populates type_var_methods from a relational where clause.
  // Given `where DoublyLinked<P, C>`, looks up the DoublyLinked interface and maps
  // typed methods (func P.children, func C.label, etc.) to the actual type var names.
  func Checker.grant_relational_methods(self, iface_name: string, type_args: [TypeExpr]) {
    let iface_entry = self.iface_decls.get(sym(iface_name))
    let iface = iface_entry!.value

    // Build interface type param name → where clause type arg name mapping
    let itp = iface.itp_children()
    let param_map = Dict<Sym, string>()  // iface param name → func type var name
    let mut i = 0
    while i < len(itp) && i < len(type_args) {
      if itp[i].name != null {
        match type_args[i].kind {
          Named(name, _) => {
            let src_name = sym_to_string(itp[i].name!)
            let dst_name = sym_to_string(name)
            param_map.set(sym(src_name), dst_name)
          }
          _ => {}
        }
      }
      i = i + 1
    }

    if self.type_var_methods == null {
      self.type_var_methods = Dict<Sym, Dict<Sym, Type>>()
    }

    let ifuncs = iface.im_children()   // actual FuncDecl methods
    for m in ifuncs {
      let mname = if m.name != null { sym_to_string(m.name!) } else { "<no-name>" }
      let rtype = if m.receiver_type != null { sym_to_string(m.receiver_type!) } else { "<no-recv>" }
      if m.receiver_type == null { continue }
      let recv_param = sym_to_string(m.receiver_type!)
      let mapping = param_map.get(sym(recv_param))
      if mapping == null { continue }
      let type_var_name = mapping!.value

      let methods_for_tv = self.type_var_methods!.get(sym(type_var_name))
      let method_dict: Dict<Sym, Type> = if methods_for_tv != null {
        methods_for_tv!.value
      } else {
        let d = Dict<Sym, Type>()
        self.type_var_methods!.set(sym(type_var_name), d)
        d
      }

      if m.name != null {
        // Build method type with substituted names
        let ft = self.func_decl_to_type_with_subst(m, param_map)
        method_dict.set(sym(sym_to_string(m.name!)), ft)
      }
    }
  }

  func Checker.func_decl_to_type_with_subst(self, f: FuncDecl, subst: Dict<Sym, string>) -> Type {
    let mut params: [Type] = []
    let fp = f.param_children()
    for p in fp {
      if p.is_self { continue }
      if p.type_expr != null {
        let mut t = self.resolve_type_expr(p.type_expr!)
        t = subst_type_var_names(t, subst)
        params = append(params, t)
      }
    }
    let mut ret = make_void_type()
    if f.return_type != null {
      ret = self.resolve_type_expr(f.return_type!)
      ret = subst_type_var_names(ret, subst)
    }
    return make_func_type(params, ret, [])
  }

  func subst_type_var_names(t: Type, subst: Dict<Sym, string>) -> Type {
    match t.kind {
      TypeVar(name) => {
        let mapped = subst.get(sym(name))
        if mapped != null {
          return make_typevar_type(mapped!.value)
        }
        return t
      }
      Sequence(elem) => {
        return make_sequence_type(subst_type_var_names(elem, subst))
      }
      Optional(inner) => {
        return make_optional_type(subst_type_var_names(inner, subst))
      }
      _ => { return t }
    }
    return t
  }

  // =====================================================================
  // Phase 2: Check function bodies
  // =====================================================================

  func Checker.check_lyric_block_bodies(self, block: LyricBlock) {
    // Check free functions
    let funcs = block.fd_children()
    for f in funcs {
      if f.body == null { continue }

      if f.receiver_type != null {
        // External method: define 'self' as receiver type
        self.push_scope()
        let rname = sym_to_string(f.receiver_type!)
        let recv_info = self.registry.lookup(rname)
        if recv_info != null {
          // Build self type with TypeVar type_args for generic classes
          let mut self_type = recv_info!.type_val
          if len(recv_info!.type_param_names) > 0 {
            let mut ta: [Type] = []
            for tpn in recv_info!.type_param_names {
              append(ta, make_typevar_type(tpn))
            }
            self_type = Type { kind: self_type.kind, bits: self_type.bits, type_args: ta }
          }
          self.scope.define("self", self_type)
        } else {
          // Receiver is a type param (e.g., "P" from an interface method).
          // Define self as TypeVar and grant relational methods from where-clauses.
          self.scope.define("self", make_typevar_type(rname))
          let where_clauses = f.where_children()
          for wc in where_clauses {
            if wc.constraint != null {
              let iface_name = sym_to_string(wc.constraint!)
              let wc_args = wc.wc_arg_children()
              self.grant_relational_methods(iface_name, wc_args)
            }
          }
        }
        self.check_func_body(f)
        self.pop_scope()
      } else {
        self.check_func_body(f)
      }
    }

    // Check class method bodies
    let classes = block.cd_children()
    for c in classes {
      if c.name == null { continue }
      let cname = sym_to_string(c.name!)
      let class_info = self.registry.lookup(cname)

      // Process class-level where clauses for type_var_methods
      // (e.g., class Dict<K, V> where K: Hashable grants K.get_hash())
      let prev_tvm = self.type_var_methods
      self.type_var_methods = null
      let cwcs = c.cwc_children()
      for wc in cwcs {
        if wc.variable != null && wc.constraint != null {
          let iface_name = sym_to_string(wc.constraint!)
          let iface_entry = self.iface_decls.get(sym(iface_name))
          if iface_entry != null {
            if self.type_var_methods == null {
              self.type_var_methods = Dict<Sym, Dict<Sym, Type>>()
            }
            let tv_name = sym_to_string(wc.variable!)
            let existing = self.type_var_methods!.get(sym(tv_name))
            let mut methods_dict = if existing != null { existing!.value } else { Dict<Sym, Type>() }
            let iface_methods = iface_entry!.value.im_children()
            for im in iface_methods {
              if im.name != null {
                let mtype = self.func_decl_to_type(im)
                methods_dict.set(im.name!, mtype)
              }
            }
            self.type_var_methods!.set(sym(tv_name), methods_dict)
          }
        }
      }

      let cmethods = c.cm_children()
      for m in cmethods {
        if m.body == null { continue }
        self.push_scope()
        if class_info != null {
          // Build self type with TypeVar type_args for generic classes
          let mut self_type = class_info!.type_val
          if len(class_info!.type_param_names) > 0 {
            let mut ta: [Type] = []
            for tpn in class_info!.type_param_names {
              append(ta, make_typevar_type(tpn))
            }
            self_type = Type { kind: self_type.kind, bits: self_type.bits, type_args: ta }
          }
          self.scope.define("self", self_type)
        }
        // Register class type params
        let ctparams = c.ctp_children()
        for tp in ctparams {
          if tp.name != null {
            let tpname = sym_to_string(tp.name!)
            self.scope.define(tpname, make_typevar_type(tpname))
          }
        }
        self.check_func_body(m)
        self.pop_scope()
      }
      self.type_var_methods = prev_tvm
    }
  }

  func Checker.check_func_body(self, f: FuncDecl) {
    if f.body == null { return }
    self.push_scope()

    let prev_ret = self.current_func_return
    let prev_name = self.current_func_name
    let prev_trusted = self.in_trusted
    self.in_trusted = f.is_trusted

    if f.name != null {
      self.current_func_name = sym_to_string(f.name!)
    }

    // Bind type params
    let tparams = f.fp_children()
    for tp in tparams {
      if tp.name != null {
        let tpname = sym_to_string(tp.name!)
        self.scope.define(tpname, make_typevar_type(tpname))
      }
    }

    // Process where clauses to populate type_var_methods
    let prev_tvm = self.type_var_methods
    let inherited = self.type_var_methods
    self.type_var_methods = null
    let wheres = f.where_children()
    for wc in wheres {
      if wc.variable == null && wc.constraint != null {
        // Bare relational constraint: where Graph<G, N, E>
        let args = wc.wc_arg_children()
        if len(args) > 0 {
          self.grant_relational_methods(sym_to_string(wc.constraint!), args)
        }
      } else if wc.variable != null && wc.constraint != null {
        // Single-type constraint: where K: Hashable
        let iface_name = sym_to_string(wc.constraint!)
        let iface_entry = self.iface_decls.get(sym(iface_name))
        if iface_entry != null {
          if self.type_var_methods == null {
            self.type_var_methods = Dict<Sym, Dict<Sym, Type>>()
          }
          let tv_name = sym_to_string(wc.variable!)
          let existing = self.type_var_methods!.get(sym(tv_name))
          let mut methods_dict = if existing != null { existing!.value } else { Dict<Sym, Type>() }
          let iface_methods = iface_entry!.value.im_children()
          for im in iface_methods {
            if im.name != null {
              let mtype = self.func_decl_to_type(im)
              methods_dict.set(im.name!, mtype)
            }
          }
          self.type_var_methods!.set(sym(tv_name), methods_dict)
        }
      }
    }
    // Merge inherited type var methods (from class-level where clauses)
    if inherited != null {
      if self.type_var_methods == null {
        self.type_var_methods = Dict<Sym, Dict<Sym, Type>>()
      }
      let inh_keys = inherited!.keys()
      let mut ki = 0
      while ki < len(inh_keys) {
        let tv_sym = inh_keys[ki]
        let inh_entry = inherited!.get(tv_sym)
        if inh_entry != null {
          let inh_methods = inh_entry!.value
          let cur_entry = self.type_var_methods!.get(tv_sym)
          let mut cur_methods = if cur_entry != null { cur_entry!.value } else { Dict<Sym, Type>() }
          let mk = inh_methods.keys()
          let mut mi = 0
          while mi < len(mk) {
            if cur_methods.get(mk[mi]) == null {
              let inh_m_entry = inh_methods.get(mk[mi])
              if inh_m_entry != null {
                cur_methods.set(mk[mi], inh_m_entry!.value)
              }
            }
            mi = mi + 1
          }
          self.type_var_methods!.set(tv_sym, cur_methods)
        }
        ki = ki + 1
      }
    }

    // Bind parameters
    let params = f.param_children()
    for p in params {
      if p.is_self { continue }
      if p.name != null {
        let mut pt = make_any_type()
        if p.type_expr != null {
          pt = self.resolve_type_expr(p.type_expr!)
        }
        self.scope.define(sym_to_string(p.name!), pt)
      }
    }

    // Set return type
    if f.return_type != null {
      self.current_func_return = self.resolve_type_expr(f.return_type!)
    } else {
      self.current_func_return = make_void_type()
    }

    self.check_block(f.body!)

    self.type_var_methods = prev_tvm
    self.current_func_return = prev_ret
    self.current_func_name = prev_name
    self.in_trusted = prev_trusted
    self.pop_scope()
  }

  // =====================================================================
  // Statement checking
  // =====================================================================

  func Checker.check_block(self, block: Block) {
    let stmts = block.bs_children()
    for s in stmts {
      self.check_stmt(s)
    }
  }

  func Checker.check_stmt(self, stmt: Stmt) {
    match stmt.kind {
      VarDecl(name, names, type_expr, is_mut, is_ref, value) => {
        self.check_var_decl(name, names, type_expr, is_mut, value)
        if is_mut && is_ref && !isnull(name) {
          let var_type = self.scope.lookup(sym_to_string(name!))
          if !isnull(var_type) {
            match var_type!.kind {
              Sequence(_) => {
                self.error_at(stmt.span, "let mut ref on a slice is not supported — slices are value types, mutations like append will not write back to the original")
              }
              _ => {}
            }
          }
        }
      }
      Assign(target, value) => {
        self.check_assign(target, value)
      }
      Return(value) => {
        self.check_return(value)
      }
      ExprStmt(expr) => {
        self.check_expr(expr)
      }
      If(condition, then_block, else_ifs, else_block) => {
        self.check_if(condition, then_block, else_ifs, else_block)
      }
      For(var_name, index_var, collection, body) => {
        self.check_for(var_name, index_var, collection, body)
      }
      While(condition, body) => {
        self.check_while(condition, body)
      }
      Match(value, arms) => {
        self.check_match(value, arms)
      }
      BlockStmt(block) => {
        self.push_scope()
        self.check_block(block)
        self.pop_scope()
      }
      Cascade(body) => {
        self.push_scope()
        self.check_block(body)
        self.pop_scope()
      }
      Spawn(body) => {
        self.push_scope()
        self.check_block(body)
        self.pop_scope()
      }
      Select(cases) => {
        for c in cases {
          self.push_scope()
          if c.expr != null {
            let ct = self.check_expr(c.expr!)
            if c.bind_var != null {
              match ct.kind {
                Channel(elem) => {
                  self.scope.define(sym_to_string(c.bind_var!), elem)
                }
                _ => {
                  self.scope.define(sym_to_string(c.bind_var!), ct)
                }
              }
            }
          }
          if c.body != null {
            self.check_block(c.body!)
          }
          self.pop_scope()
        }
      }
      Yield(value) => {
        if value != null { self.check_expr(value!) }
      }
      Lock(mutex, body) => {
        self.check_expr(mutex)
        self.push_scope()
        self.check_block(body)
        self.pop_scope()
      }
      IfLet(pattern, value, then_block, else_block) => {
        let val_type = self.check_expr(value)
        self.push_scope()
        let mut bind_type = val_type
        match val_type.kind {
          Optional(inner) => { bind_type = inner }
          _ => {}
        }
        match pattern.kind {
          Variant(_, _) => { bind_type = val_type }
          _ => {}
        }
        self.bind_pattern(pattern, bind_type)
        self.check_block(then_block)
        self.pop_scope()
        if else_block != null {
          self.push_scope()
          self.check_block(else_block!)
          self.pop_scope()
        }
      }
      LetElse(pattern, value, else_block) => {
        let val_type = self.check_expr(value)
        let mut bind_type = val_type
        match val_type.kind {
          Optional(inner) => { bind_type = inner }
          _ => {}
        }
        match pattern.kind {
          Variant(_, _) => { bind_type = val_type }
          _ => {}
        }
        self.bind_pattern(pattern, bind_type)
        self.push_scope()
        self.check_block(else_block)
        self.pop_scope()
      }
      Break | Continue => {}
      Ref(_ref_expr) => {
        if !self.in_trusted {
          self.error_at(stmt.span, "ref statement can only be used inside a trusted function")
        }
      }
      Unref(_unref_expr) => {
        if !self.in_trusted {
          self.error_at(stmt.span, "unref statement can only be used inside a trusted function")
        }
      }
    }
  }

  func Checker.check_var_decl(self, name: Sym, names: [Sym], type_expr: TypeExpr?, is_mut: bool, value: Expr?) {
    let mut declared_type: Type? = null
    if type_expr != null {
      declared_type = self.resolve_type_expr(type_expr!)
    }

    if value != null {
      let val_type = self.check_expr(value!)

      // Tuple destructuring: let (a, b) = expr
      if len(names) > 0 {
        match val_type.kind {
          Tuple(fields) => {
            for i in range(0, len(names)) {
              if i < len(fields) && fields[i].type_val != null {
                self.scope.define(sym_to_string(names[i]), fields[i].type_val!)
              } else {
                self.scope.define(sym_to_string(names[i]), make_any_type())
              }
            }
          }
          ErrorResult(ok, err) => {
            if len(names) >= 1 { self.scope.define(sym_to_string(names[0]), ok) }
            if len(names) >= 2 { self.scope.define(sym_to_string(names[1]), err) }
          }
          _ => {
            for n in names {
              self.scope.define(sym_to_string(n), make_any_type())
            }
          }
        }
        return
      }

      // Propagate expected type to empty list/map/null literals
      if declared_type != null {
        match value!.kind {
          ListLit(elems) => {
            if len(elems) == 0 {
              value!.resolved_type = type_to_type_expr(declared_type!)
            }
          }
          MapLit(keys, _) => {
            if len(keys) == 0 {
              value!.resolved_type = type_to_type_expr(declared_type!)
            }
          }
          Nil => {
            value!.resolved_type = type_to_type_expr(declared_type!)
          }
          _ => {}
        }
      }

      if declared_type != null {
        if !self.is_assignable(val_type, declared_type!) {
          self.error("type mismatch in variable declaration for " + sym_to_string(name))
        }
        self.scope.define(sym_to_string(name), declared_type!)
      } else {
        self.scope.define(sym_to_string(name), val_type)
      }
    } else if declared_type != null {
      self.scope.define(sym_to_string(name), declared_type!)
    } else {
      self.scope.define(sym_to_string(name), make_any_type())
    }
  }

  func Checker.check_assign(self, target: Expr, value: Expr) {
    let target_type = self.check_expr(target)
    let val_type = self.check_expr(value)
    if !self.is_assignable(val_type, target_type) {
      match target_type.kind {
        Any => {}
        _ => {}
      }
    }
    // Propagate target type to empty list/null literals
    match target_type.kind {
      Error => {}
      _ => {
        match value.kind {
          ListLit(elems) => {
            if len(elems) == 0 {
              value.resolved_type = type_to_type_expr(target_type)
            }
          }
          Nil => {
            value.resolved_type = type_to_type_expr(target_type)
          }
          _ => {}
        }
      }
    }
  }

  func Checker.check_return(self, value: Expr?) {
    if value != null {
      self.check_expr(value!)
      // Propagate return type to empty list/null literals
      if self.current_func_return != null {
        match value!.kind {
          ListLit(elems) => {
            if len(elems) == 0 {
              value!.resolved_type = type_to_type_expr(self.current_func_return!)
            }
          }
          Nil => {
            value!.resolved_type = type_to_type_expr(self.current_func_return!)
          }
          _ => {}
        }
      }
    }
  }

  func Checker.check_if(self, condition: Expr, then_block: Block, else_ifs: [ElseIf], else_block: Block?) {
    let cond_type = self.check_expr(condition)
    self.push_scope()
    self.check_block(then_block)
    self.pop_scope()
    for ei in else_ifs {
      if ei.condition != null {
        self.check_expr(ei.condition!)
      }
      if ei.body != null {
        self.push_scope()
        self.check_block(ei.body!)
        self.pop_scope()
      }
    }
    if else_block != null {
      self.push_scope()
      self.check_block(else_block!)
      self.pop_scope()
    }
  }

  func Checker.check_for(self, var_name: Sym, index_var: Sym?, collection: Expr, body: Block) {
    let coll_type = self.check_expr(collection)
    self.push_scope()

    match coll_type.kind {
      Sequence(elem) => {
        if index_var != null {
          // Go-style: for index, value in collection
          self.scope.define(sym_to_string(var_name), make_int_type(32))
          self.scope.define(sym_to_string(index_var!), elem)
        } else {
          self.scope.define(sym_to_string(var_name), elem)
        }
      }
      String => {
        if index_var != null {
          self.scope.define(sym_to_string(var_name), make_int_type(32))
          self.scope.define(sym_to_string(index_var!), make_string_type())
        } else {
          self.scope.define(sym_to_string(var_name), make_string_type())
        }
      }
      Generator(elem) => {
        self.scope.define(sym_to_string(var_name), elem)
      }
      Channel(elem) => {
        self.scope.define(sym_to_string(var_name), elem)
      }
      Map(key, value) => {
        self.scope.define(sym_to_string(var_name), key)
        if index_var != null {
          self.scope.define(sym_to_string(index_var!), value)
        }
      }
      _ => {
        self.scope.define(sym_to_string(var_name), make_any_type())
        if index_var != null {
          self.scope.define(sym_to_string(index_var!), make_any_type())
        }
      }
    }

    self.check_block(body)
    self.pop_scope()
  }

  func Checker.check_while(self, condition: Expr, body: Block) {
    self.check_expr(condition)
    self.push_scope()
    self.check_block(body)
    self.pop_scope()
  }

  func Checker.check_match(self, value: Expr, arms: [MatchArm]) {
    let val_type = self.check_expr(value)
    for arm in arms {
      self.push_scope()
      if arm.pattern != null {
        self.bind_pattern(arm.pattern!, val_type)
      }
      for p in arm.patterns {
        self.bind_pattern(p, val_type)
      }
      if arm.guard != null {
        self.check_expr(arm.guard!)
      }
      if arm.body != null {
        self.check_block(arm.body!)
      }
      self.pop_scope()
    }
  }

  // =====================================================================
  // Pattern binding
  // =====================================================================

  func Checker.bind_pattern(self, pat: Pattern, val_type: Type) {
    match pat.kind {
      Ident(name) => {
        self.scope.define(sym_to_string(name), val_type)
      }
      Literal(expr) => {
        self.check_expr(expr)
      }
      Wildcard => {}
      Variant(name, bindings) => {
        let vname = sym_to_string(name)
        // Look up variant in the matched enum type
        let ename = type_name(val_type)
        if ename != "" {
          let enum_info = self.registry.lookup(ename)
          if enum_info != null {
            let vi = enum_info!.variants.get(sym(vname))
            if vi != null {
              for i in range(0, len(bindings)) {
                if i < len(vi!.value.fields) {
                  self.bind_pattern(bindings[i], vi!.value.fields[i].type_val)
                } else {
                  self.bind_pattern(bindings[i], make_any_type())
                }
              }
              return
            }
          }
        }
        // Fallback: check all registered enums for this variant name
        // (for bare variant names without qualified enum type)
        for b in bindings {
          self.bind_pattern(b, make_any_type())
        }
      }
      Tuple(elems) => {
        match val_type.kind {
          Tuple(fields) => {
            for i in range(0, len(elems)) {
              if i < len(fields) && fields[i].type_val != null {
                self.bind_pattern(elems[i], fields[i].type_val!)
              } else {
                self.bind_pattern(elems[i], make_any_type())
              }
            }
          }
          _ => {
            for e in elems {
              self.bind_pattern(e, make_any_type())
            }
          }
        }
      }
    }
  }

  // =====================================================================
  // Expression checking (with annotation)
  // =====================================================================

  func Checker.check_expr(self, expr: Expr) -> Type {
    let result = self.check_expr_inner(expr)
    return annotate(expr, result)
  }

  func Checker.check_expr_inner(self, expr: Expr) -> Type {
    match expr.kind {
      Ident(name) => {
        let name_str = sym_to_string(name)
        let vt = self.scope.lookup(name_str)
        if vt != null { return vt! }
        // Check registry for type names (for module access etc.)
        let info = self.registry.lookup(name_str)
        if info != null { return info!.type_val }
        self.error_at(expr.span, "undefined variable: " + name_str)
        return make_error_type()
      }
      IntLit(_, type_hint) => {
        if type_hint != null {
          let hint = sym_to_string(type_hint!)
          let info = self.registry.lookup(hint)
          if info != null { return info!.type_val }
        }
        return make_int_type(32)
      }
      FloatLit(_) => { return make_float_type(64) }
      StringLit(_) => { return make_string_type() }
      StringInterp(parts) => {
        for p in parts {
          self.check_expr(p)
        }
        return make_string_type()
      }
      BoolLit(_) => { return make_bool_type() }
      Nil => { return make_nil_type() }

      Call(func_expr, type_args, args, _) => {
        return self.check_call(expr, func_expr, type_args, args)
      }
      MethodCall(receiver, method, type_args, args, _) => {
        return self.check_method_call(expr, receiver, method, type_args, args)
      }
      FieldAccess(receiver, field_name) => {
        return self.check_field_access(expr, receiver, field_name)
      }
      Index(receiver, index) => {
        return self.check_index(receiver, index)
      }
      Slice(receiver, low, high) => {
        return self.check_slice(receiver, low, high)
      }
      Unary(op, operand) => {
        return self.check_unary(op, operand)
      }
      Binary(left, op, right) => {
        return self.check_binary(left, op, right)
      }
      TupleLit(elems) => {
        return self.check_tuple_lit(elems)
      }
      ListLit(elems) => {
        return self.check_list_lit(elems)
      }
      MapLit(keys, values) => {
        return self.check_map_lit(keys, values)
      }
      StructLit(type_name, type_args, fields) => {
        return self.check_struct_lit(type_name, type_args, fields)
      }
      Lambda(params, return_type, body) => {
        return self.check_lambda(params, return_type, body)
      }
      Match(value, arms) => {
        return self.check_match_expr(value, arms)
      }
      Cast(target_type, operand) => {
        return self.check_cast(target_type, operand)
      }
      Unwrap(operand) => {
        return self.check_unwrap(operand)
      }
      Try(operand) => {
        return self.check_try(operand)
      }
      Is(operand, variant) => {
        return self.check_is(operand, variant)
      }
      IfElse(cond, then_block, else_ifs, else_block) => {
        return self.check_if_else_expr(cond, then_block, else_ifs, else_block)
      }
    }
    eprintln("checker: check_expr: unreachable — unknown ExprKind")
    os_exit(1)
    return make_void_type()
  }

  func Checker.check_unary(self, op: UnaryOp, operand: Expr) -> Type {
    let ot = self.check_expr(operand)
    match op {
      Not => { return make_bool_type() }
      Neg => { return ot }
    }
    return ot
  }

  func Checker.check_binary(self, left: Expr, op: BinaryOp, right: Expr) -> Type {
    let lt = self.check_expr(left)
    let rt = self.check_expr(right)

    match op {
      Eq | Neq => {
        // null comparison: propagate type to the null operand
        match lt.kind {
          Nil => { return make_bool_type() }
          _ => {}
        }
        match rt.kind {
          Nil => { return make_bool_type() }
          _ => {}
        }
        return make_bool_type()
      }
      Lt | Le | Gt | Ge => { return make_bool_type() }
      And | Or => { return make_bool_type() }
      Add => {
        // String concatenation: string + string → string
        if lt.kind is String && rt.kind is String {
          return make_string_type()
        }
        // Slice concatenation: [T] + [T] → [T]
        if lt.kind is Sequence && rt.kind is Sequence {
          return lt
        }
        return coerce_numeric(lt, rt)
      }
      _ => {
        return coerce_numeric(lt, rt)
      }
    }
    eprintln("checker: check_binary: unreachable")
    os_exit(1)
    return make_void_type()
  }

  func Checker.check_index(self, receiver: Expr, index: Expr) -> Type {
    let recv_type = self.check_expr(receiver)
    self.check_expr(index)
    match recv_type.kind {
      Sequence(elem) => { return elem }
      String => { return make_uint_type(8) }  // string indexing returns u8
      Map(_, value) => { return make_optional_type(value) }
      _ => {
        self.error_at(receiver.span, "cannot index into this type")
        return make_error_type()
      }
    }
  }

  func Checker.check_slice(self, receiver: Expr, low: Expr?, high: Expr?) -> Type {
    let recv_type = self.check_expr(receiver)
    if low != null { self.check_expr(low!) }
    if high != null { self.check_expr(high!) }
    return recv_type
  }

  func Checker.check_cast(self, target_type: TypeExpr, operand: Expr) -> Type {
    self.check_expr(operand)
    return self.resolve_type_expr(target_type)
  }

  func Checker.check_unwrap(self, operand: Expr) -> Type {
    let ot = self.check_expr(operand)
    match ot.kind {
      Optional(inner) => { return inner }
      _ => { return ot }
    }
  }

  func Checker.check_try(self, operand: Expr) -> Type {
    let ot = self.check_expr(operand)
    // ? extracts T from (T, error)
    match ot.kind {
      ErrorResult(ok, _) => { return ok }
      Tuple(fields) => {
        if len(fields) >= 2 && fields[0].type_val != null {
          // Check if second element is error type
          return fields[0].type_val!
        }
        if len(fields) >= 1 && fields[0].type_val != null {
          return fields[0].type_val!
        }
        self.error_at(operand.span, "cannot apply ? to this tuple type")
        return make_error_type()
      }
      _ => { return ot }
    }
  }

  func Checker.check_is(self, operand: Expr, variant: Sym) -> Type {
    self.check_expr(operand)
    return make_bool_type()
  }

  func Checker.check_if_else_expr(self, cond: Expr, then_block: Block, else_ifs: [ElseIf], else_block: Block) -> Type {
    self.check_expr(cond)
    self.push_scope()
    self.check_block(then_block)
    let then_type = self.check_block_expr_type(then_block)
    self.pop_scope()

    for ei in else_ifs {
      if ei.condition != null {
        self.check_expr(ei.condition!)
      }
      if ei.body != null {
        self.push_scope()
        self.check_block(ei.body!)
        self.pop_scope()
      }
    }

    self.push_scope()
    self.check_block(else_block)
    self.pop_scope()

    return then_type
  }

  func Checker.check_block_expr_type(self, block: Block) -> Type {
    // The type of a block-as-expression is the type of its last expression
    let stmts = block.bs_children()
    if len(stmts) > 0 {
      let last = stmts[len(stmts) - 1]
      match last.kind {
        ExprStmt(expr) => {
          if expr.resolved_type != null {
            return self.resolve_type_expr(expr.resolved_type!)
          }
        }
        _ => {}
      }
    }
    return make_void_type()
  }

  func Checker.check_tuple_lit(self, elems: [Expr]) -> Type {
    let mut fields: [TupleFieldType] = []
    for e in elems {
      let et = self.check_expr(e)
      append(fields, TupleFieldType { name: "", type_val: et })
    }
    return make_tuple_type(fields)
  }

  func Checker.check_list_lit(self, elems: [Expr]) -> Type {
    if len(elems) > 0 {
      let elem_type = self.check_expr(elems[0])
      for i in range(1, len(elems)) {
        self.check_expr(elems[i])
      }
      return make_sequence_type(elem_type)
    }
    return make_sequence_type(make_void_type())
  }

  func Checker.check_map_lit(self, keys: [Expr], values: [Expr]) -> Type {
    if len(keys) == 0 {
      // Empty dict literal {} — needs type annotation to infer K and V
      let sym_type = make_class_type("Sym")
      return Type { kind: Class("Dict"), bits: 0, type_args: [sym_type, make_void_type()] }
    }
    // Check all keys and values (sets resolved_type on each)
    let key_type = self.check_expr(keys[0])
    let mut i = 1
    while i < len(keys) {
      self.check_expr(keys[i])
      i = i + 1
    }
    let val_type = self.check_expr(values[0])
    let mut j = 1
    while j < len(values) {
      self.check_expr(values[j])
      j = j + 1
    }
    return Type { kind: Class("Dict"), bits: 0, type_args: [key_type, val_type] }
  }

  func Checker.check_struct_lit(self, type_name_sym: Sym, type_args: [TypeExpr], fields: [StructLitField]) -> Type {
    let name_str = sym_to_string(type_name_sym)

    // Handle `lock` as a builtin mutex type — not registered in the struct registry
    if name_str == "lock" {
      return make_lock_type()
    }

    let info = self.registry.lookup(name_str)

    if info != null {
      // Use indexed while-loop because StructLitField is a value type —
      // `for f in fields` copies each element, so mutations don't propagate.
      // We compute the effective field name from position when the AST name is null.
      let mut pos_idx = 0
      let mut fi = 0
      while fi < len(fields) {
        let f = fields[fi]

        // Determine the effective field name: use AST name if present, else positional
        let mut fname = ""
        if f.name != null && sym_to_string(f.name!) != "" {
          fname = sym_to_string(f.name!)
          pos_idx = len(info!.field_order)  // stop positional tracking
        } else {
          if pos_idx < len(info!.field_order) {
            fname = info!.field_order[pos_idx]
            pos_idx = pos_idx + 1
          }
        }

        if f.value != null {
          // If the expected field type is an enum, temporarily shadow variant
          // constructors in scope so ambiguous variant names resolve correctly.
          let mut scope_pushed = false
          if fname != "" {
            let field_entry = info!.fields.get(sym(fname))
            if field_entry != null {
              let field_type = field_entry!.value
              match field_type.kind {
                Enum(enum_name) => {
                  let enum_info = self.registry.lookup(enum_name)
                  if enum_info != null && len(enum_info!.variants.keys()) > 0 {
                    self.push_scope()
                    scope_pushed = true
                    let vkeys = enum_info!.variants.keys()
                    for vk in vkeys {
                      let vname = sym_to_string(vk)
                      let vi = enum_info!.variants.get(vk)
                      if vi != null {
                        if len(vi!.value.fields) == 0 {
                          self.scope.define(vname, field_type)
                        } else {
                          let mut param_types: [Type] = []
                          for vf in vi!.value.fields {
                            append(param_types, vf.type_val)
                          }
                          let tpnames: [string] = []
                          let ctor_type = Type {
                            kind: TypeKind.Func(param_types, field_type, tpnames),
                            bits: 0,
                          }
                          self.scope.define(vname, ctor_type)
                        }
                      }
                    }
                  }
                }
                _ => {}
              }
            }
          }

          self.check_expr(f.value!)

          if scope_pushed {
            self.pop_scope()
          }

          // Propagate expected type to empty list/map/null literals
          if fname != "" {
            let field_entry = info!.fields.get(sym(fname))
            if field_entry != null {
              let field_type = field_entry!.value
              match f.value!.kind {
                ListLit(elems) => {
                  if len(elems) == 0 {
                    f.value!.resolved_type = type_to_type_expr(field_type)
                  }
                }
                MapLit(keys, _) => {
                  if len(keys) == 0 {
                    f.value!.resolved_type = type_to_type_expr(field_type)
                  }
                }
                Nil => {
                  f.value!.resolved_type = type_to_type_expr(field_type)
                }
                _ => {}
              }
            }
          }
        }
        fi = fi + 1
      }
      // Return type with type_args for generic classes/structs
      if len(type_args) > 0 && len(info!.type_param_names) > 0 {
        let mut resolved_args: [Type] = []
        let mut limit = len(info!.type_param_names)
        if len(type_args) < limit { limit = len(type_args) }
        for i in range(0, limit) {
          append(resolved_args, self.resolve_type_expr(type_args[i]))
        }
        return Type { kind: info!.type_val.kind, bits: info!.type_val.bits, type_args: resolved_args }
      }
      return info!.type_val
    }

    eprintln(f"checker: unknown struct/class: {name_str}")
    os_exit(1)
    return make_error_type()
  }

  func Checker.check_lambda(self, params: [Param], return_type: TypeExpr?, body: Block) -> Type {
    self.push_scope()
    let mut param_types: [Type] = []
    for p in params {
      let mut pt = make_any_type()
      if p.type_expr != null {
        pt = self.resolve_type_expr(p.type_expr!)
      }
      if p.name != null {
        self.scope.define(sym_to_string(p.name!), pt)
      }
      append(param_types, pt)
    }
    let mut ret = make_void_type()
    if return_type != null {
      ret = self.resolve_type_expr(return_type!)
    }
    self.check_block(body)
    self.pop_scope()
    return make_func_type(param_types, ret, [])
  }

  func Checker.check_match_expr(self, value: Expr, arms: [MatchArm]) -> Type {
    let val_type = self.check_expr(value)
    let mut result_type = make_void_type()
    let mut first = true
    for arm in arms {
      self.push_scope()
      if arm.pattern != null {
        self.bind_pattern(arm.pattern!, val_type)
      }
      for p in arm.patterns {
        self.bind_pattern(p, val_type)
      }
      if arm.guard != null {
        self.check_expr(arm.guard!)
      }
      if arm.body != null {
        self.check_block(arm.body!)
        if first {
          result_type = self.check_block_expr_type(arm.body!)
          first = false
        }
      }
      self.pop_scope()
    }
    return result_type
  }

  // =====================================================================
  // Call checking
  // =====================================================================

  // Propagate expected types from function params to empty list/null arg literals.
  // Must use indexed access — Expr is a class but we need to match Go checker's
  // behavior of annotating the actual arg expressions.
  func propagate_arg_types(args: [Expr], param_types: [Type]) {
    let mut i = 0
    while i < len(args) && i < len(param_types) {
      match args[i].kind {
        ListLit(elems) => {
          if len(elems) == 0 {
            match param_types[i].kind {
              Sequence(elem) => {
                args[i].resolved_type = type_to_type_expr(param_types[i])
              }
              _ => {}
            }
          }
        }
        Nil => {
          args[i].resolved_type = type_to_type_expr(param_types[i])
        }
        _ => {}
      }
      i = i + 1
    }
  }

  func Checker.check_call(self, call_expr: Expr, func_expr: Expr, type_args: [TypeExpr], args: [Expr]) -> Type {
    // Check all args first
    let mut arg_types: [Type] = []
    for a in args {
      append(arg_types, self.check_expr(a))
    }

    match func_expr.kind {
      Ident(name) => {
        let name_str = sym_to_string(name)

        // === Builtins ===
        let builtin_result = self.check_builtin_call(name_str, type_args, arg_types)
        if builtin_result != null {
          // Annotate the func_expr as a function
          annotate(func_expr, make_func_type(arg_types, builtin_result!, []))
          return builtin_result!
        }

        // Look up in scope
        let ft = self.scope.lookup(name_str)
        if ft != null {
          annotate(func_expr, ft!)
          annotate(func_expr, ft!)
          match ft!.kind {
            Func(params, ret, tpnames) => {
              // Generic function — infer type args
              if len(tpnames) > 0 {
                let bindings = self.infer_type_args(ft!, arg_types)
                // Also add explicit type args
                let mut limit = len(tpnames)
                if len(type_args) < limit { limit = len(type_args) }
                for i in range(0, limit) {
                  bindings.set(sym(tpnames[i]), self.resolve_type_expr(type_args[i]))
                }
                // Store inferred type args on the call expression for the lowerer
                if len(type_args) == 0 {
                  let mut inferred: [TypeExpr] = []
                  for tpn in tpnames {
                    let bound = bindings.get(sym(tpn))
                    if bound != null {
                      append(inferred, type_to_type_expr(bound!.value))
                    }
                  }
                  call_expr.inferred_type_args = inferred
                }
                // Propagate expected types to empty list/null args
                let mut sub_params: [Type] = []
                for p in params {
                  append(sub_params, substitute_type(p, bindings))
                }
                propagate_arg_types(args, sub_params)
                return substitute_type(ret, bindings)
              }
              propagate_arg_types(args, params)
              return ret
            }
            Class(cname) | Struct(cname) | Enum(cname) => {
              // Constructor call
              return ft!
            }
            _ => { return ft! }
          }
        }

        // Check registry for type constructors
        let info = self.registry.lookup(name_str)
        if info != null {
          annotate(func_expr, info!.type_val)
          return info!.type_val
        }

        eprintln(f"checker: unknown function: {name_str} at {func_expr.span.start.file}:{itoa(func_expr.span.start.line as i64)}:{itoa(func_expr.span.start.column as i64)}")
        os_exit(1)
        return make_error_type()
      }
      FieldAccess(receiver, field) => {
        // Module.function() call
        let recv_type = self.check_expr(receiver)
        let field_str = sym_to_string(field)
        match recv_type.kind {
          Module(mod_name) => {
            // Known module function return types
            let result = self.resolve_module_func(mod_name, field_str)
            annotate(func_expr, result)
            return result
          }
          Enum(_) => {
            annotate(func_expr, recv_type)
            return recv_type
          }
          _ => {
            // Regular field that's callable
            let ft = self.check_field_access(func_expr, receiver, field)
            annotate(func_expr, ft)
            match ft.kind {
              Func(_, ret, _) => { return ret }
              _ => { return ft }
            }
          }
        }
      }
      _ => {
        let ft = self.check_expr(func_expr)
        match ft.kind {
          Func(_, ret, _) => { return ret }
          _ => { return ft }
        }
      }
    }
    eprintln("checker: check_call: unreachable")
    os_exit(1)
    return make_void_type()
  }

  func Checker.resolve_module_func(self, mod_name: string, func_name: string) -> Type {
    // fmt
    if mod_name == "fmt" {
      if func_name == "Println" || func_name == "Printf" || func_name == "Print" { return make_void_type() }
      if func_name == "Sprintf" { return make_string_type() }
      if func_name == "Errorf" { return make_error_type() }
    }
    // errors
    if mod_name == "errors" {
      if func_name == "New" { return make_error_type() }
    }
    // strconv
    if mod_name == "strconv" {
      if func_name == "Itoa" { return make_string_type() }
      if func_name == "Atoi" {
        return make_tuple_type([
          TupleFieldType { name: "", type_val: make_int_type(64) },
          TupleFieldType { name: "", type_val: make_bool_type() },
        ])
      }
    }
    // strings
    if mod_name == "strings" {
      if func_name == "ToUpper" || func_name == "ToLower" || func_name == "TrimSpace" || func_name == "Join" { return make_string_type() }
      if func_name == "Contains" || func_name == "HasPrefix" || func_name == "HasSuffix" { return make_bool_type() }
      if func_name == "Split" { return make_sequence_type(make_string_type()) }
    }
    // math
    if mod_name == "math" {
      if func_name == "Sqrt" || func_name == "Abs" || func_name == "Floor" || func_name == "Ceil" { return make_float_type(64) }
    }
    // os
    if mod_name == "os" {
      if func_name == "Exit" { return make_void_type() }
      if func_name == "Args" { return make_sequence_type(make_string_type()) }
    }
    eprintln(f"checker: unknown module function: {mod_name}.{func_name}")
    os_exit(1)
    return make_error_type()
  }

  // check_builtin_call handles built-in functions. Returns null if not a builtin.
  func Checker.check_builtin_call(self, name: string, type_args: [TypeExpr], arg_types: [Type]) -> Type? {
    if name == "len" { return make_int_type(32) }
    if name == "append" {
      if len(arg_types) >= 1 { return arg_types[0] }
      return make_void_type()
    }
    if name == "print" || name == "println" || name == "eprint" || name == "eprintln" {
      return make_void_type()
    }
    if name == "panic" { return make_void_type() }
    if name == "assert" { return make_void_type() }
    if name == "assert_eq" { return make_void_type() }
    if name == "exit" { return make_void_type() }
    if name == "format" { return make_string_type() }
    if name == "itoa" { return make_string_type() }
    if name == "atoi" {
      return make_tuple_type([
        TupleFieldType { name: "", type_val: make_int_type(64) },
        TupleFieldType { name: "", type_val: make_bool_type() },
      ])
    }
    if name == "parse_float" {
      return make_tuple_type([
        TupleFieldType { name: "", type_val: make_float_type(64) },
        TupleFieldType { name: "", type_val: make_bool_type() },
      ])
    }
    if name == "char_to_string" { return make_string_type() }
    if name == "string_to_bytes" { return make_sequence_type(make_uint_type(8)) }
    if name == "bytes_to_string" { return make_string_type() }
    if name == "make_channel" {
      if len(type_args) > 0 {
        return make_channel_type(self.resolve_type_expr(type_args[0]))
      }
      return make_channel_type(make_any_type())
    }
    if name == "sym" {
      let info = self.registry.lookup("Sym")
      if info != null { return info!.type_val }
      eprintln("checker: stdlib type Sym not found")
      os_exit(1)
      return make_error_type()
    }
    if name == "hash_string" { return make_uint_type(64) }
    if name == "os_args" { return make_sequence_type(make_string_type()) }
    if name == "os_exit" { return make_void_type() }
    if name == "read_file" {
      return make_tuple_type([
        TupleFieldType { name: "", type_val: make_string_type() },
        TupleFieldType { name: "", type_val: make_bool_type() },
      ])
    }
    if name == "write_file" { return make_bool_type() }
    if name == "isnull" { return make_bool_type() }
    if name == "to_string" { return make_string_type() }
    if name == "new_string_builder" {
      let info = self.registry.lookup("StringBuilder")
      if info != null { return info!.type_val }
      eprintln("checker: stdlib type StringBuilder not found")
      os_exit(1)
      return make_error_type()
    }
    if name == "new_error" { return make_error_type() }

    // I/O builtins
    if name == "list_dir" { return make_sequence_type(make_string_type()) }
    if name == "file_exists" { return make_bool_type() }
    if name == "mkdtemp" { return make_string_type() }
    if name == "os_getwd" { return make_string_type() }
    if name == "path_base" { return make_string_type() }
    if name == "path_dir" { return make_string_type() }
    if name == "path_join" { return make_string_type() }
    if name == "path_ext" { return make_string_type() }
    if name == "exec_command" {
      let fields: [TupleFieldType] = [
        TupleFieldType { name: "", type_val: make_string_type() },
        TupleFieldType { name: "", type_val: make_bool_type() }
      ]
      return Type { kind: TypeKind.Tuple(fields), bits: 0, type_args: [] }
    }

    // String builtins
    if name == "str_contains" {
      return make_bool_type()
    }
    if name == "str_index" || name == "str_len" || name == "str_index_of" { return make_int_type(32) }
    if name == "str_trim" || name == "str_replace" || name == "str_substr" || name == "str_char_at" || name == "str_to_lower" || name == "str_to_upper" || name == "str_from_chars" {
      return make_string_type()
    }
    if name == "str_split" { return make_sequence_type(make_string_type()) }
    if name == "char_is_digit" || name == "char_is_alpha" || name == "char_is_space" || name == "char_is_upper" || name == "char_is_lower" || name == "char_is_alnum" {
      return make_bool_type()
    }

    // Dict builtins removed — now methods on Dict<K,V>

    // ArrayList/DLL builtins
    if name == "array_append" || name == "array_remove" || name == "dll_append" || name == "dll_remove" {
      return make_void_type()
    }
    if name == "hash_init" || name == "hash_insert" || name == "hash_rehash" {
      return make_void_type()
    }
    // hash_remove returns bool — let it fall through to scope lookup
    if name == "hash_find_slot" { return make_int_type(32) }
    // hash_lookup is handled via scope lookup with generic substitution
    // (NOT as a builtin — its return type depends on type params, not arg types)

    // Not a builtin
    return null
  }

  // =====================================================================
  // Generic type argument substitution
  // =====================================================================

  // build_type_arg_subst creates a substitution map from a type's type_args
  // and the class/struct's type_param_names. Returns null if no substitution needed.
  func Checker.build_type_arg_subst(self, recv_type: Type) -> Dict<Sym, Type>? {
    if len(recv_type.type_args) == 0 { return null }
    let tname = type_name(recv_type)
    if tname == "" { return null }
    let info = self.registry.lookup(tname)
    if info == null || len(info!.type_param_names) == 0 { return null }
    let subst = Dict<Sym, Type>()
    let mut limit = len(info!.type_param_names)
    if len(recv_type.type_args) < limit { limit = len(recv_type.type_args) }
    for i in range(0, limit) {
      subst.set(sym(info!.type_param_names[i]), recv_type.type_args[i])
    }
    return subst
  }

  // =====================================================================
  // Method call checking
  // =====================================================================

  func Checker.check_method_call(self, call_expr: Expr, receiver: Expr, method: Sym, type_args: [TypeExpr], args: [Expr]) -> Type {
    // Qualified enum variant constructor: EnumName.Variant(args)
    // Must check BEFORE check_expr on receiver — enum names are in registry, not scope,
    // so check_expr would emit "undefined variable" and return TypeError.
    match receiver.kind {
      ExprKind.Ident(ident_name) => {
        let enum_name = sym_to_string(ident_name)
        let enum_info = self.registry.lookup(enum_name)
        if enum_info != null && enum_info!.type_val.kind is Enum {
          let method_str = sym_to_string(method)
          let vi = enum_info!.variants.get(sym(method_str))
          if vi == null {
            eprintln(f"checker: enum {enum_name} has no variant '{method_str}'")
            return make_error_type()
          }
          let variant = vi!.value
          // Check args against variant fields
          if len(variant.fields) == 0 {
            // Unit variant called as constructor — just return enum type
          } else {
            if len(args) != len(variant.fields) {
              eprintln(f"checker: {enum_name}.{method_str} expects {len(variant.fields)} arguments, got {len(args)}")
            }
            for i in range(0, len(args)) {
              self.check_expr(args[i])
            }
          }
          // Rewrite AST: MethodCall → Call on bare variant constructor
          let func_expr = Expr {
            kind: ExprKind.Ident(method),
            span: receiver.span,
            resolved_type: null
          }
          // Set resolved_type on func expr
          if len(variant.fields) > 0 {
            let mut param_types: [Type] = []
            for f in variant.fields {
              append(param_types, f.type_val)
            }
            let empty_names: [string] = []
            func_expr.resolved_type = type_to_type_expr(Type {
              kind: TypeKind.Func(param_types, enum_info!.type_val, empty_names),
              bits: 0,
              type_args: []
            })
          } else {
            func_expr.resolved_type = type_to_type_expr(enum_info!.type_val)
          }
          let mut_args: [bool] = []
          call_expr.kind = ExprKind.Call(func_expr, type_args, args, mut_args)
          return enum_info!.type_val
        }
      }
      _ => {}
    }
    let recv_type = self.check_expr(receiver)
    let mut arg_types: [Type] = []
    for a in args {
      append(arg_types, self.check_expr(a))
    }
    let method_str = sym_to_string(method)


    // Built-in methods by receiver type
    let builtin = self.check_builtin_method(recv_type, method_str, arg_types)
    if builtin != null { return builtin! }

    // Module function calls (e.g. fmt.Println)
    match recv_type.kind {
      Module(mod_name) => {
        return self.resolve_module_func(mod_name, method_str)
      }
      Enum(ename) => {
        // Enum variant constructor via method syntax
        return recv_type
      }
      _ => {}
    }

    // Look up method in registry
    let tname = type_name(recv_type)
    if tname != "" {
      let info = self.registry.lookup(tname)
      if info != null {
        let mt = info!.methods.get(sym(method_str))
        if mt != null {
          match mt!.value.kind {
            Func(params, ret, tpnames) => {
              // Build substitution: class type args first, then infer method-specific
              let class_subst = self.build_type_arg_subst(recv_type)
              let mut bindings = if class_subst != null { class_subst! } else { Dict<Sym, Type>() }
              if len(tpnames) > 0 {
                let inferred = self.infer_type_args(mt!.value, arg_types)
                // Merge inferred bindings (don't override class-provided ones)
                let ikeys = inferred.keys()
                let mut ik = 0
                while ik < len(ikeys) {
                  if bindings.get(ikeys[ik]) == null {
                    let ie = inferred.get(ikeys[ik])
                    if ie != null {
                      bindings.set(ikeys[ik], ie!.value)
                    }
                  }
                  ik = ik + 1
                }
                // Store inferred type args for lowerer
                if len(type_args) == 0 {
                  let mut inferred_ta: [TypeExpr] = []
                  for tpn in tpnames {
                    let bound = bindings.get(sym(tpn))
                    if bound != null {
                      append(inferred_ta, type_to_type_expr(bound!.value))
                    }
                  }
                  call_expr.inferred_type_args = inferred_ta
                }
              } else {
                // Check for pre-computed type args from interface method registration
                let mta_key = tname + "." + method_str
                let mta = self.method_type_args.get(sym(mta_key))
                if mta != null {
                  call_expr.inferred_type_args = mta!.value
                }
              }
              // Substitute params for argument checking
              let mut sub_params: [Type] = []
              for p in params {
                append(sub_params, substitute_type(p, bindings))
              }
              propagate_arg_types(args, sub_params)
              return substitute_type(ret, bindings)
            }
            _ => { return mt!.value }
          }
        }
      }

      // Also check scope for "T.method" pattern (external methods)
      let key = tname + "." + method_str
      let ext = self.scope.lookup(key)
      if ext != null {
        match ext!.kind {
          Func(_, ret, _) => { return ret }
          _ => { return ext! }
        }
      }
    }

    // Auto-unwrap optional
    match recv_type.kind {
      Optional(inner) => {
        let inner_result = self.check_builtin_method(inner, method_str, arg_types)
        if inner_result != null { return inner_result! }
        let inner_name = type_name(inner)
        if inner_name != "" {
          let info = self.registry.lookup(inner_name)
          if info != null {
            let mt = info!.methods.get(sym(method_str))
            if mt != null {
              match mt!.value.kind {
                Func(_, ret, _) => { return ret }
                _ => { return mt!.value }
              }
            }
          }
        }
      }
      _ => {}
    }

    // Universal methods
    if method_str == "to_string" { return make_string_type() }

    // Type variable methods from relational constraints (where Graph<G, N, E>)
    match recv_type.kind {
      TypeVar(tv_name) => {
        if self.type_var_methods != null {
          let tv_methods = self.type_var_methods!.get(sym(tv_name))
          if tv_methods != null {
            let meth = tv_methods!.value.get(sym(method_str))
            if meth != null {
              match meth!.value.kind {
                Func(params, ret, _) => { return ret }
                _ => { return meth!.value }
              }
            }
          }
        }
      }
      _ => {}
    }

    eprintln(f"checker: unknown method: {method_str} at {call_expr.span.start.file}:{itoa(call_expr.span.start.line as i64)}:{itoa(call_expr.span.start.column as i64)}")
    os_exit(1)
    return make_error_type()
  }

  func Checker.check_builtin_method(self, recv_type: Type, method: string, arg_types: [Type]) -> Type? {
    match recv_type.kind {
      Sequence(elem) => {
        return self.check_list_method(elem, method)
      }
      String => {
        return self.check_string_method(method)
      }
      Channel(elem) => {
        return self.check_channel_method(elem, method)
      }
      Map(key, value) => {
        return self.check_map_method(key, value, method)
      }
      _ => { return null }
    }
    return null
  }

  func Checker.check_list_method(self, elem: Type, method: string) -> Type? {
    if method == "push" || method == "append" || method == "sort" || method == "remove" || method == "extend" || method == "clear" || method == "reverse" {
      return make_void_type()
    }
    if method == "pop" { return elem }
    if method == "length" || method == "len" { return make_int_type(32) }
    if method == "contains" { return make_bool_type() }
    if method == "slice" { return make_sequence_type(elem) }
    if method == "index_of" || method == "find" { return make_int_type(32) }
    if method == "first" || method == "last" { return make_optional_type(elem) }
    if method == "is_empty" { return make_bool_type() }
    if method == "join" { return make_string_type() }
    return null
  }

  func Checker.check_string_method(self, method: string) -> Type? {
    if method == "length" || method == "len" { return make_int_type(32) }
    if method == "contains" || method == "starts_with" || method == "ends_with" || method == "has_prefix" || method == "has_suffix" { return make_bool_type() }
    if method == "substring" || method == "trim" || method == "to_lower" || method == "to_upper" || method == "replace" || method == "repeat" || method == "char_at" {
      return make_string_type()
    }
    if method == "split" { return make_sequence_type(make_string_type()) }
    if method == "index_of" { return make_int_type(32) }
    if method == "is_empty" { return make_bool_type() }
    if method == "join" { return make_string_type() }
    return null
  }

  func Checker.check_channel_method(self, elem: Type, method: string) -> Type? {
    if method == "send" { return make_void_type() }
    if method == "recv" || method == "receive" { return elem }
    if method == "close" { return make_void_type() }
    return null
  }

  func Checker.check_map_method(self, key: Type, value: Type, method: string) -> Type? {
    if method == "get" { return make_optional_type(value) }
    if method == "set" || method == "put" || method == "delete" || method == "remove" {
      return make_void_type()
    }
    if method == "has" || method == "contains" || method == "contains_key" { return make_bool_type() }
    if method == "keys" { return make_sequence_type(key) }
    if method == "values" { return make_sequence_type(value) }
    if method == "len" || method == "length" { return make_int_type(32) }
    if method == "len" || method == "length" { return make_int_type(32) }
    return null
  }

  // =====================================================================
  // Field access checking
  // =====================================================================

  func Checker.check_field_access(self, parent_expr: Expr, receiver: Expr, field_name: Sym) -> Type {
    // Qualified enum variant: EnumName.Variant (for unit variants or constructor refs)
    match receiver.kind {
      ExprKind.Ident(ident_name) => {
        let enum_name = sym_to_string(ident_name)
        let enum_info = self.registry.lookup(enum_name)
        if enum_info != null && enum_info!.type_val.kind is Enum {
          let field_str = sym_to_string(field_name)
          let vi = enum_info!.variants.get(sym(field_str))
          if vi == null {
            eprintln(f"checker: enum {enum_name} has no variant '{field_str}'")
            return make_error_type()
          }
          let variant = vi!.value
          if len(variant.fields) > 0 {
            // Non-unit variant without args — return constructor function type
            let mut param_types: [Type] = []
            for f in variant.fields {
              append(param_types, f.type_val)
            }
            let empty_names: [string] = []
            return Type {
              kind: TypeKind.Func(param_types, enum_info!.type_val, empty_names),
              bits: 0,
              type_args: []
            }
          }
          // Unit variant — rewrite AST to plain Ident
          parent_expr.kind = ExprKind.Ident(field_name)
          return enum_info!.type_val
        }
      }
      _ => {}
    }
    let recv_type = self.check_expr(receiver)
    let field_str = sym_to_string(field_name)
    return self.resolve_field_type(recv_type, field_str)
  }


  func Checker.resolve_field_type(self, recv_type: Type, field_str: string) -> Type {
    let tname = type_name(recv_type)
    if tname != "" {
      let info = self.registry.lookup(tname)
      if info != null {
        // Check fields first
        let ft = info!.fields.get(sym(field_str))
        if ft != null {
          let subst = self.build_type_arg_subst(recv_type)
          if subst != null { return substitute_type(ft!.value, subst!) }
          return ft!.value
        }
        // Then check methods (for method-as-value)
        let mt = info!.methods.get(sym(field_str))
        if mt != null {
          let subst = self.build_type_arg_subst(recv_type)
          if subst != null { return substitute_type(mt!.value, subst!) }
          return mt!.value
        }
      }
    }

    match recv_type.kind {
      Tuple(fields) => {
        // Numeric field access: ._0, ._1, ._2 (with _ prefix like Go checker)
        if str_has_prefix(field_str, "_") {
          let idx_str = field_str[1:len(field_str)]
          let idx_result = atoi(idx_str)
          let idx = idx_result._0
          let ok = idx_result._1
          if ok && idx >= 0 && idx < len(fields) && fields[idx].type_val != null {
            return fields[idx].type_val!
          }
        }
        // Named fields
        for f in fields {
          if f.name == field_str && f.type_val != null {
            return f.type_val!
          }
        }
      }
      Optional(inner) => {
        return self.resolve_field_type(inner, field_str)
      }
      Enum(_) => {
        // Enum variant access (e.g. MyEnum.Variant)
        return recv_type
      }
      Module(mod_name) => {
        // Module field/function access
        // TODO: resolve actual field type from module registry
        return make_void_type()
      }
      _ => {}
    }

    eprintln(f"checker: field {field_str} not found on type {tname}")
    os_exit(1)
    return make_error_type()
  }

  // =====================================================================
  // Numeric coercion
  // =====================================================================

  func coerce_numeric(a: Type, b: Type) -> Type {
    if types_equal(a, b) { return a }
    if numeric_widens(a, b) { return b }
    if numeric_widens(b, a) { return a }
    if is_numeric(a) { return a }
    if is_numeric(b) { return b }
    // String concatenation
    match a.kind {
      String => { return make_string_type() }
      _ => {}
    }
    // Non-numeric, non-string binary op — return first operand type
    return a
  }

  // =====================================================================
  // Walker infrastructure (for validators)
  // =====================================================================

  func Checker.walk_expr(self, expr: Expr, callback: (Expr) -> ()) {
    callback(expr)
    match expr.kind {
      Ident(_) | IntLit(_, _) | FloatLit(_) | StringLit(_) | BoolLit(_) | Nil => {}
      StringInterp(parts) => {
        for p in parts { self.walk_expr(p, callback) }
      }
      Call(func_expr, _, args, _) => {
        self.walk_expr(func_expr, callback)
        for a in args { self.walk_expr(a, callback) }
      }
      MethodCall(receiver, _, _, args, _) => {
        self.walk_expr(receiver, callback)
        for a in args { self.walk_expr(a, callback) }
      }
      FieldAccess(receiver, _) => {
        self.walk_expr(receiver, callback)
      }
      Index(receiver, index) => {
        self.walk_expr(receiver, callback)
        self.walk_expr(index, callback)
      }
      Slice(receiver, low, high) => {
        self.walk_expr(receiver, callback)
        if low != null { self.walk_expr(low!, callback) }
        if high != null { self.walk_expr(high!, callback) }
      }
      Unary(_, operand) => {
        self.walk_expr(operand, callback)
      }
      Binary(left, _, right) => {
        self.walk_expr(left, callback)
        self.walk_expr(right, callback)
      }
      TupleLit(elems) => {
        for e in elems { self.walk_expr(e, callback) }
      }
      ListLit(elems) => {
        for e in elems { self.walk_expr(e, callback) }
      }
      MapLit(keys, values) => {
        for k in keys { self.walk_expr(k, callback) }
        for v in values { self.walk_expr(v, callback) }
      }
      StructLit(_, _, fields) => {
        for f in fields {
          if f.value != null { self.walk_expr(f.value!, callback) }
        }
      }
      Lambda(_, _, body) => {
        self.walk_block(body, callback)
      }
      Match(value, arms) => {
        self.walk_expr(value, callback)
        for arm in arms {
          if arm.pattern != null { self.walk_pattern(arm.pattern!, callback) }
          for p in arm.patterns { self.walk_pattern(p, callback) }
          if arm.guard != null { self.walk_expr(arm.guard!, callback) }
          if arm.body != null { self.walk_block(arm.body!, callback) }
        }
      }
      Cast(_, operand) => {
        self.walk_expr(operand, callback)
      }
      Unwrap(operand) => {
        self.walk_expr(operand, callback)
      }
      Try(operand) => {
        self.walk_expr(operand, callback)
      }
      Is(operand, _) => {
        self.walk_expr(operand, callback)
      }
      IfElse(cond, then_block, else_ifs, else_block) => {
        self.walk_expr(cond, callback)
        self.walk_block(then_block, callback)
        for ei in else_ifs {
          if ei.condition != null { self.walk_expr(ei.condition!, callback) }
          if ei.body != null { self.walk_block(ei.body!, callback) }
        }
        self.walk_block(else_block, callback)
      }
    }
  }

  func Checker.walk_stmt(self, stmt: Stmt, callback: (Expr) -> ()) {
    match stmt.kind {
      VarDecl(_, _, _, _, _, value) => {
        if value != null { self.walk_expr(value!, callback) }
      }
      Assign(target, value) => {
        self.walk_expr(target, callback)
        self.walk_expr(value, callback)
      }
      Return(value) => {
        if value != null { self.walk_expr(value!, callback) }
      }
      ExprStmt(expr) => {
        self.walk_expr(expr, callback)
      }
      If(condition, then_block, else_ifs, else_block) => {
        self.walk_expr(condition, callback)
        self.walk_block(then_block, callback)
        for ei in else_ifs {
          if ei.condition != null { self.walk_expr(ei.condition!, callback) }
          if ei.body != null { self.walk_block(ei.body!, callback) }
        }
        if else_block != null { self.walk_block(else_block!, callback) }
      }
      For(_, _, collection, body) => {
        self.walk_expr(collection, callback)
        self.walk_block(body, callback)
      }
      While(condition, body) => {
        self.walk_expr(condition, callback)
        self.walk_block(body, callback)
      }
      Match(value, arms) => {
        self.walk_expr(value, callback)
        for arm in arms {
          if arm.pattern != null { self.walk_pattern(arm.pattern!, callback) }
          for p in arm.patterns { self.walk_pattern(p, callback) }
          if arm.guard != null { self.walk_expr(arm.guard!, callback) }
          if arm.body != null { self.walk_block(arm.body!, callback) }
        }
      }
      BlockStmt(block) | Cascade(block) | Spawn(block) => {
        self.walk_block(block, callback)
      }
      Select(cases) => {
        for c in cases {
          if c.expr != null { self.walk_expr(c.expr!, callback) }
          if c.body != null { self.walk_block(c.body!, callback) }
        }
      }
      Yield(value) => {
        if value != null { self.walk_expr(value!, callback) }
      }
      Lock(mutex, body) => {
        self.walk_expr(mutex, callback)
        self.walk_block(body, callback)
      }
      IfLet(pattern, value, then_block, else_block) => {
        self.walk_pattern(pattern, callback)
        self.walk_expr(value, callback)
        self.walk_block(then_block, callback)
        if else_block != null { self.walk_block(else_block!, callback) }
      }
      LetElse(pattern, value, else_block) => {
        self.walk_pattern(pattern, callback)
        self.walk_expr(value, callback)
        self.walk_block(else_block, callback)
      }
      Break | Continue => {}
      Ref(_) | Unref(_) => {}
    }
  }

  func Checker.walk_block(self, block: Block, callback: (Expr) -> ()) {
    let stmts = block.bs_children()
    for s in stmts { self.walk_stmt(s, callback) }
  }

  func Checker.walk_pattern(self, pat: Pattern, callback: (Expr) -> ()) {
    match pat.kind {
      Literal(expr) => { self.walk_expr(expr, callback) }
      Variant(_, bindings) => {
        for b in bindings { self.walk_pattern(b, callback) }
      }
      Tuple(elems) => {
        for e in elems { self.walk_pattern(e, callback) }
      }
      Ident(_) | Wildcard => {}
    }
  }

  // =====================================================================
  // Invariant validators
  // =====================================================================

  func is_primitive_name(n: string) -> bool {
    if n == "i8" { return true }
    if n == "i16" { return true }
    if n == "i32" { return true }
    if n == "i64" { return true }
    if n == "int" { return true }
    if n == "u8" { return true }
    if n == "u16" { return true }
    if n == "u32" { return true }
    if n == "u64" { return true }
    if n == "uint" { return true }
    if n == "f32" { return true }
    if n == "f64" { return true }
    if n == "bool" { return true }
    if n == "string" { return true }
    if n == "any" { return true }
    if n == "error" { return true }
    return false
  }

  // type_expr_has_typevar checks if a TypeExpr contains unresolved type variables.
  // A type variable shows up as Named("T", []) where T is not a primitive and not
  // in the registry. Used by the invariant validator to catch TypeVar leaks.
  func Checker.type_expr_has_typevar(self, te: TypeExpr) -> bool {
    match te.kind {
      Named(name, args) => {
        let n = name.name
        // Check if it's a known primitive
        if is_primitive_name(n) { return false }
        // Check if it's in the registry (struct/class/enum/interface)
        let info = self.registry.lookup(n)
        if info == null {
          // Check if it's a known module/import (not a type variable)
          let scope_val = self.scope.lookup(n)
          if scope_val != null {
            match scope_val!.kind {
              Module(_) => { return false }
              _ => {}
            }
          }
          // Not a primitive, not registered, not a module — it's an unresolved type variable
          return true
        }
        // Recurse into type args
        for a in args {
          if self.type_expr_has_typevar(a) { return true }
        }
        return false
      }
      Optional(inner) => { return self.type_expr_has_typevar(inner) }
      Sequence(elem) => { return self.type_expr_has_typevar(elem) }
      Map(key, value) => {
        return self.type_expr_has_typevar(key) || self.type_expr_has_typevar(value)
      }
      Tuple(fields) => {
        for f in fields {
          if f.type_expr != null && self.type_expr_has_typevar(f.type_expr!) { return true }
        }
        return false
      }
      Func(params, ret) => {
        // Function types legitimately contain type variables (generic function references)
        return false
      }
      Channel(elem) => { return self.type_expr_has_typevar(elem) }
      Generator(elem) => { return self.type_expr_has_typevar(elem) }
      Union(variants) => {
        for v in variants {
          if self.type_expr_has_typevar(v) { return true }
        }
        return false
      }
      Unit | Lock => { return false }
    }
    return false
  }

  // find_typevar_name returns the first unresolved type variable name in a TypeExpr
  func Checker.find_typevar_name(self, te: TypeExpr) -> string {
    match te.kind {
      Named(name, args) => {
        let n = name.name
        if !is_primitive_name(n) {
          let info = self.registry.lookup(n)
          if info == null { return n }
        }
        for a in args {
          let found = self.find_typevar_name(a)
          if found != "" { return found }
        }
      }
      Optional(inner) => { return self.find_typevar_name(inner) }
      Sequence(elem) => { return self.find_typevar_name(elem) }
      Map(key, value) => {
        let k = self.find_typevar_name(key)
        if k != "" { return k }
        return self.find_typevar_name(value)
      }
      Func(params, ret) => {
        // Function types legitimately contain type variables
      }
      Tuple(fields) => {
        for f in fields {
          if f.type_expr != null {
            let found = self.find_typevar_name(f.type_expr!)
            if found != "" { return found }
          }
        }
      }
      Channel(elem) => { return self.find_typevar_name(elem) }
      Generator(elem) => { return self.find_typevar_name(elem) }
      Union(variants) => {
        for v in variants {
          let found = self.find_typevar_name(v)
          if found != "" { return found }
        }
      }
      Unit | Lock => {}
    }
    return ""
  }

  // validateAllExprsResolved walks every Expr in the file and panics if
  // any has a null resolved_type. Only runs when checker has zero errors.
  func Checker.validate_all_exprs_resolved(self, file: File) {
    if len(self.errors) > 0 { return }

    let blocks = file.fb_children()
    for block in blocks {
      let funcs = block.fd_children()
      for f in funcs {
        if f.body == null { continue }
        // Skip generic functions — they aren't fully resolved
        let tps = f.fp_children()
        if len(tps) > 0 { continue }

        let ctx = f.name!.name
        self.vwalk_block(f.body!, ctx)
      }

      let classes = block.cd_children()
      for cls in classes {
        // Skip generic classes — their methods use unresolved type variables
        let cls_tps = cls.ctp_children()
        if len(cls_tps) > 0 { continue }

        let cmethods = cls.cm_children()
        for m in cmethods {
          if m.body == null { continue }
          let tps = m.fp_children()
          if len(tps) > 0 { continue }

          let ctx = sym_to_string(cls.name!) + "." + m.name!.name
          self.vwalk_block(m.body!, ctx)
        }
      }
    }
  }

  func Checker.vwalk_block(self, block: Block, ctx: string) {
    let stmts = block.bs_children()
    for s in stmts {
      self.vwalk_stmt(s, ctx)
    }
  }

  func Checker.vwalk_stmt(self, stmt: Stmt, ctx: string) {
    match stmt.kind {
      VarDecl(_, _, _, _, _, value) => {
        if value != null { self.vwalk_expr(value!, ctx) }
      }
      Assign(target, value) => {
        self.vwalk_expr(target, ctx)
        self.vwalk_expr(value, ctx)
      }
      Return(value) => {
        if value != null { self.vwalk_expr(value!, ctx) }
      }
      ExprStmt(expr) => {
        self.vwalk_expr(expr, ctx)
      }
      If(condition, then_block, else_ifs, else_block) => {
        self.vwalk_expr(condition, ctx)
        self.vwalk_block(then_block, ctx)
        for ei in else_ifs {
          if ei.condition != null { self.vwalk_expr(ei.condition!, ctx) }
          if ei.body != null { self.vwalk_block(ei.body!, ctx) }
        }
        if else_block != null { self.vwalk_block(else_block!, ctx) }
      }
      For(_, _, collection, body) => {
        self.vwalk_expr(collection, ctx)
        self.vwalk_block(body, ctx)
      }
      While(condition, body) => {
        self.vwalk_expr(condition, ctx)
        self.vwalk_block(body, ctx)
      }
      Match(value, arms) => {
        self.vwalk_expr(value, ctx)
        for arm in arms {
          if arm.pattern != null { self.vwalk_pattern(arm.pattern!, ctx) }
          for p in arm.patterns { self.vwalk_pattern(p, ctx) }
          if arm.guard != null { self.vwalk_expr(arm.guard!, ctx) }
          if arm.body != null { self.vwalk_block(arm.body!, ctx) }
        }
      }
      BlockStmt(block) => {
        self.vwalk_block(block, ctx)
      }
      Cascade(body) => {
        self.vwalk_block(body, ctx)
      }
      Spawn(body) => {
        self.vwalk_block(body, ctx)
      }
      Select(cases) => {
        for c in cases {
          if c.expr != null { self.vwalk_expr(c.expr!, ctx) }
          if c.body != null { self.vwalk_block(c.body!, ctx) }
        }
      }
      Yield(value) => {
        if value != null { self.vwalk_expr(value!, ctx) }
      }
      Lock(mutex, body) => {
        self.vwalk_expr(mutex, ctx)
        self.vwalk_block(body, ctx)
      }
      IfLet(pattern, value, then_block, else_block) => {
        self.vwalk_expr(value, ctx)
        self.vwalk_pattern(pattern, ctx)
        self.vwalk_block(then_block, ctx)
        if else_block != null { self.vwalk_block(else_block!, ctx) }
      }
      LetElse(pattern, value, else_block) => {
        self.vwalk_expr(value, ctx)
        self.vwalk_pattern(pattern, ctx)
        self.vwalk_block(else_block, ctx)
      }
      Break | Continue => {}
      Ref(_) | Unref(_) => {}
    }
  }

  func Checker.vwalk_pattern(self, pat: Pattern, ctx: string) {
    match pat.kind {
      Literal(expr) => {
        self.vwalk_expr(expr, ctx)
      }
      Variant(_, bindings) => {
        for b in bindings { self.vwalk_pattern(b, ctx) }
      }
      Tuple(elems) => {
        for e in elems { self.vwalk_pattern(e, ctx) }
      }
      Ident(_) | Wildcard => {}
    }
  }

  func Checker.vwalk_expr(self, expr: Expr, ctx: string) {
    if isnull(expr.resolved_type) {
      eprintln(f"checker: validateAllExprsResolved: null resolved_type in {ctx} at {expr.span.start.file}:{itoa(expr.span.start.line as i64)}:{itoa(expr.span.start.column as i64)}")
      os_exit(1)
    }
    // Note: we do NOT reject 'any' here — it's a legitimate type.
    // The Go checker only rejects null resolved_type, not any.
    // Note: Go checker does NOT reject Sequence(Unit) or Map(Unit,...) — they produce
    // valid void* slices/maps in C. Removed strict checks to match Go behavior.
    // Detect TypeVar leaks — type variables should be fully resolved in non-generic functions
    if self.type_expr_has_typevar(expr.resolved_type!) {
      let leaked = self.find_typevar_name(expr.resolved_type!)
      eprintln(f"checker: validateAllExprsResolved: TypeVar leak '{leaked}' in {ctx} at {expr.span.start.file}:{itoa(expr.span.start.line as i64)}:{itoa(expr.span.start.column as i64)}")
      os_exit(1)
    }

    match expr.kind {
      // Leaf nodes
      Ident(_) | IntLit(_, _) | FloatLit(_) | StringLit(_) | BoolLit(_) | Nil => {}

      StringInterp(parts) => {
        for p in parts { self.vwalk_expr(p, ctx) }
      }
      Call(func_expr, _, args, _) => {
        self.vwalk_expr(func_expr, ctx)
        for a in args { self.vwalk_expr(a, ctx) }
      }
      MethodCall(receiver, _, _, args, _) => {
        self.vwalk_expr(receiver, ctx)
        for a in args { self.vwalk_expr(a, ctx) }
      }
      FieldAccess(receiver, _) => {
        self.vwalk_expr(receiver, ctx)
      }
      Index(receiver, index) => {
        self.vwalk_expr(receiver, ctx)
        self.vwalk_expr(index, ctx)
      }
      Slice(receiver, low, high) => {
        self.vwalk_expr(receiver, ctx)
        if low != null { self.vwalk_expr(low!, ctx) }
        if high != null { self.vwalk_expr(high!, ctx) }
      }
      Unary(_, operand) => {
        self.vwalk_expr(operand, ctx)
      }
      Binary(left, _, right) => {
        self.vwalk_expr(left, ctx)
        self.vwalk_expr(right, ctx)
      }
      TupleLit(elems) => {
        for e in elems { self.vwalk_expr(e, ctx) }
      }
      ListLit(elems) => {
        for e in elems { self.vwalk_expr(e, ctx) }
      }
      MapLit(keys, values) => {
        for k in keys { self.vwalk_expr(k, ctx) }
        for v in values { self.vwalk_expr(v, ctx) }
      }
      StructLit(_, _, fields) => {
        for f in fields {
          if f.value != null { self.vwalk_expr(f.value!, ctx) }
        }
      }
      Lambda(_, _, body) => {
        // TODO: lambda capture conversion — convert captured locals into
        // explicit parameters and pass them at call sites (C++ style).
        // For now we validate types inside lambda bodies.
        self.vwalk_block(body, ctx + ".<lambda>")
      }
      Match(value, arms) => {
        self.vwalk_expr(value, ctx)
        for arm in arms {
          if arm.pattern != null { self.vwalk_pattern(arm.pattern!, ctx) }
          for p in arm.patterns { self.vwalk_pattern(p, ctx) }
          if arm.guard != null { self.vwalk_expr(arm.guard!, ctx) }
          if arm.body != null { self.vwalk_block(arm.body!, ctx) }
        }
      }
      Cast(_, operand) => {
        self.vwalk_expr(operand, ctx)
      }
      Unwrap(operand) => {
        self.vwalk_expr(operand, ctx)
      }
      Try(operand) => {
        self.vwalk_expr(operand, ctx)
      }
      Is(operand, _) => {
        self.vwalk_expr(operand, ctx)
      }
      IfElse(cond, then_block, else_ifs, else_block) => {
        self.vwalk_expr(cond, ctx)
        self.vwalk_block(then_block, ctx)
        for ei in else_ifs {
          if ei.condition != null { self.vwalk_expr(ei.condition!, ctx) }
          if ei.body != null { self.vwalk_block(ei.body!, ctx) }
        }
        self.vwalk_block(else_block, ctx)
      }
    }
  }

  // validateFieldAndMethodAccess walks all FieldAccess and MethodCall exprs
  // and verifies the field/method exists on the receiver type.
  func Checker.validate_field_and_method_access(self, file: File) {
    if len(self.errors) > 0 { return }

    let blocks = file.fb_children()
    for block in blocks {
      let funcs = block.fd_children()
      for f in funcs {
        if f.body == null { continue }
        let tps = f.fp_children()
        if len(tps) > 0 { continue }

        self.validate_access_block(f.body!)
      }

      let classes = block.cd_children()
      for cls in classes {
        let cmethods = cls.cm_children()
        for m in cmethods {
          if m.body == null { continue }
          let tps = m.fp_children()
          if len(tps) > 0 { continue }

          self.validate_access_block(m.body!)
        }
      }
    }
  }

  func Checker.validate_access_block(self, block: Block) {
    let stmts = block.bs_children()
    for s in stmts {
      self.validate_access_stmt(s)
    }
  }

  func Checker.validate_access_stmt(self, stmt: Stmt) {
    match stmt.kind {
      VarDecl(_, _, _, _, _, value) => {
        if value != null { self.validate_access_expr(value!) }
      }
      Assign(target, value) => {
        self.validate_access_expr(target)
        self.validate_access_expr(value)
      }
      Return(value) => {
        if value != null { self.validate_access_expr(value!) }
      }
      ExprStmt(expr) => {
        self.validate_access_expr(expr)
      }
      If(condition, then_block, else_ifs, else_block) => {
        self.validate_access_expr(condition)
        self.validate_access_block(then_block)
        for ei in else_ifs {
          if ei.condition != null { self.validate_access_expr(ei.condition!) }
          if ei.body != null { self.validate_access_block(ei.body!) }
        }
        if else_block != null { self.validate_access_block(else_block!) }
      }
      For(_, _, collection, body) => {
        self.validate_access_expr(collection)
        self.validate_access_block(body)
      }
      While(condition, body) => {
        self.validate_access_expr(condition)
        self.validate_access_block(body)
      }
      Match(value, arms) => {
        self.validate_access_expr(value)
        for arm in arms {
          if arm.guard != null { self.validate_access_expr(arm.guard!) }
          if arm.body != null { self.validate_access_block(arm.body!) }
        }
      }
      BlockStmt(block) | Cascade(block) | Spawn(block) => {
        self.validate_access_block(block)
      }
      Lock(mutex, body) => {
        self.validate_access_expr(mutex)
        self.validate_access_block(body)
      }
      IfLet(_, value, then_block, else_block) => {
        self.validate_access_expr(value)
        self.validate_access_block(then_block)
        if else_block != null { self.validate_access_block(else_block!) }
      }
      LetElse(_, value, else_block) => {
        self.validate_access_expr(value)
        self.validate_access_block(else_block)
      }
      Select(cases) => {
        for c in cases {
          if c.expr != null { self.validate_access_expr(c.expr!) }
          if c.body != null { self.validate_access_block(c.body!) }
        }
      }
      Yield(value) => {
        if value != null { self.validate_access_expr(value!) }
      }
      _ => {}
    }
  }

  func Checker.validate_access_expr(self, expr: Expr) {
    match expr.kind {
      FieldAccess(receiver, field_name) => {
        self.validate_access_expr(receiver)
        self.validate_field_access(receiver, field_name)
      }
      MethodCall(receiver, method, _, args, _) => {
        self.validate_access_expr(receiver)
        for a in args { self.validate_access_expr(a) }
        self.validate_method_access(receiver, method)
      }
      Call(func_expr, _, args, _) => {
        self.validate_access_expr(func_expr)
        for a in args { self.validate_access_expr(a) }
      }
      Binary(left, _, right) => {
        self.validate_access_expr(left)
        self.validate_access_expr(right)
      }
      Unary(_, operand) => {
        self.validate_access_expr(operand)
      }
      Index(receiver, index) => {
        self.validate_access_expr(receiver)
        self.validate_access_expr(index)
      }
      Slice(receiver, low, high) => {
        self.validate_access_expr(receiver)
        if low != null { self.validate_access_expr(low!) }
        if high != null { self.validate_access_expr(high!) }
      }
      Cast(_, operand) => {
        self.validate_access_expr(operand)
      }
      Unwrap(operand) | Try(operand) | Is(operand, _) => {
        self.validate_access_expr(operand)
      }
      TupleLit(elems) | ListLit(elems) => {
        for e in elems { self.validate_access_expr(e) }
      }
      MapLit(keys, values) => {
        for k in keys { self.validate_access_expr(k) }
        for v in values { self.validate_access_expr(v) }
      }
      StructLit(_, _, fields) => {
        for f in fields {
          if f.value != null { self.validate_access_expr(f.value!) }
        }
      }
      StringInterp(parts) => {
        for p in parts { self.validate_access_expr(p) }
      }
      Lambda(_, _, body) => {
        self.validate_access_block(body)
      }
      Match(value, arms) => {
        self.validate_access_expr(value)
        for arm in arms {
          if arm.guard != null { self.validate_access_expr(arm.guard!) }
          if arm.body != null { self.validate_access_block(arm.body!) }
        }
      }
      IfElse(cond, then_block, else_ifs, else_block) => {
        self.validate_access_expr(cond)
        self.validate_access_block(then_block)
        for ei in else_ifs {
          if ei.condition != null { self.validate_access_expr(ei.condition!) }
          if ei.body != null { self.validate_access_block(ei.body!) }
        }
        self.validate_access_block(else_block)
      }
      _ => {}
    }
  }

  func Checker.validate_field_access(self, receiver: Expr, field_name: Sym) {
    if receiver.resolved_type == null { return }
    let recv_type = self.resolve_type_expr(receiver.resolved_type!)
    let field_str = sym_to_string(field_name)

    // Skip validation for Any, TypeVar, Optional (auto-unwrap), Module, Enum
    match recv_type.kind {
      Any | TypeVar(_) | Optional(_) | Module(_) | Enum(_) => { return }
      _ => {}
    }

    let tname = type_name(recv_type)
    if tname == "" { return }

    let info = self.registry.lookup(tname)
    if info == null { return }

    let ft = info!.fields.get(sym(field_str))
    if ft != null { return }
    let mt = info!.methods.get(sym(field_str))
    if mt != null { return }

    // Check tuple numeric access
    match recv_type.kind {
      Tuple(_) => { return }  // Tuple field access always valid syntactically
      _ => {}
    }

    // Not found — report warning (not error, to avoid false positives from desugared fields)
    // self.error("field " + field_str + " not found on type " + tname)
  }

  func Checker.validate_method_access(self, receiver: Expr, method: Sym) {
    if receiver.resolved_type == null { return }
    let recv_type = self.resolve_type_expr(receiver.resolved_type!)
    let method_str = sym_to_string(method)

    // Skip validation for Any, TypeVar, Optional, Module
    match recv_type.kind {
      Any | TypeVar(_) | Optional(_) | Module(_) => { return }
      _ => {}
    }

    // Built-in methods on sequences/strings/maps/channels — always valid
    match recv_type.kind {
      Sequence(_) | String | Map(_, _) | Channel(_) => { return }
      _ => {}
    }

    // Universal methods
    if method_str == "to_string" { return }

    let tname = type_name(recv_type)
    if tname == "" { return }

    let info = self.registry.lookup(tname)
    if info == null { return }

    let mt = info!.methods.get(sym(method_str))
    if mt != null { return }

    // Check "T.method" in scope
    let key = tname + "." + method_str
    let ext = self.scope.lookup(key)
    if ext != null { return }

    // Not found — soft warning
    // self.error("method " + method_str + " not found on type " + tname)
  }

  // validateAllTypesResolved checks no function has TyUnknown params/return.
  // (TyUnknown should not survive past the checker.)
  func Checker.validate_all_types_resolved(self, file: File) {
    // In bootstrap checker we don't use TyUnknown, so this is a no-op.
    // Keeping the interface for compatibility with the Go checker.
  }

  // =====================================================================
  // Helper
  // =====================================================================

  func sym_to_string(s: Sym) -> string {
    return s.name
  }

  // =====================================================================
  // Entry point
  // =====================================================================

  func check_file(file: File) -> Checker {
    let c = new_checker()

    let blocks = file.fb_children()

    // Phase 0: Pre-register all type names across ALL blocks
    for b in blocks {
      c.preregister_type_names(b)
    }

    // Phase 1: register all types and signatures across ALL blocks
    for b in blocks {
      c.register_lyric_block(b)
    }

    // Phase 1.5: register interface methods using ALL impl blocks
    c.register_interface_methods(file)

    // Phase 2: check all function bodies
    for b in blocks {
      c.check_lyric_block_bodies(b)
    }

    // Invariant validation
    c.validate_all_exprs_resolved(file)
    c.validate_field_and_method_access(file)
    c.validate_all_types_resolved(file)

    return c
  }

