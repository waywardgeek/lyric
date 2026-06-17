// c_backend.ly — Bootstrap port of pkg/lir/c_backend.go
// Generates C source code from a monomorphized LIR program.
// The program MUST be monomorphized before calling this — C has no generics.

// --- C code generator state ---

permanent class CGen {
  prog: LProgram?
  buf: StringBuilder?
  indent: i32
  temp_used: Dict<Sym, bool>?
  temp_types: Dict<Sym, LType?>?     // resolved types for temps
  var_types: Dict<Sym, LType?>?      // resolved types for variables
  slice_types: Dict<Sym, string>?    // cType(elem) → typedef name
  opt_types: Dict<Sym, string>?      // cType(elem) → typedef name
  result_types: Dict<Sym, string>?   // cType(elem) → typedef name
  chan_types: Dict<Sym, string>?      // cType(elem) → suffix name
  slice_count: i32
  opt_count: i32
  result_count: i32
  lambda_id: i32
  lambdas: [CLambda]
  current_func: LFuncDecl?

  // Generator state machine support
  in_generator: bool
  gen_struct_name: string
  gen_yield_count: i32

  // Select label uniqueness
  select_id: i32

  // Spawn support
  spawn_id: i32
  spawn_funcs: [CSpawnFunc]
  spawn_captures: Dict<Sym, bool>?    // variables captured by current spawn block

  // Interface vtable dispatch
  iface_by_name: Dict<Sym, LInterfaceDecl>?
  impl_map: Dict<Sym, [string]>?      // class name → interfaces it implements
  func_by_name: Dict<Sym, LFuncDecl>?

  // OS builtin helpers needed
  needs_os_args: bool
  needs_exec_cmd: bool
  needs_path_join: bool

  // Simple enum tracking
  simple_enums: Dict<Sym, bool>?

  // Mut param tracking
  mut_params: Dict<Sym, bool>?

  // Tuple type tracking
  tuple_types: Dict<Sym, string>?    // "field_types_key" → typedef name
  tuple_count: i32

  // Generator function lookup (pre-built)
  gen_funcs: Dict<Sym, bool>?

  // Method name → receiver lookup for unknown-receiver calls
  method_to_receiver: Dict<Sym, string>?

  // Struct/class lookup by name
  struct_by_name: Dict<Sym, LStructDecl>?
  class_by_name: Dict<Sym, LClassDecl>?
}

struct CLambda {
  name: string
  ret_type: string
  params: string
  body_str: string
}

struct CSpawnFunc {
  name: string
  body_str: string
  captures: [CCapture]
}

struct CCapture {
  name: string
  typ: string
}

func new_cgen(prog: LProgram?) -> CGen? {
  let g = CGen {
    prog: prog,
    buf: new_string_builder(),
    indent: 0,
    temp_used: Dict<Sym, bool>(),
    temp_types: Dict<Sym, LType?>(),
    var_types: Dict<Sym, LType?>(),
    slice_types: Dict<Sym, string>(),
    opt_types: Dict<Sym, string>(),
    result_types: Dict<Sym, string>(),
    chan_types: Dict<Sym, string>(),
    slice_count: 0,
    opt_count: 0,
    result_count: 0,
    lambda_id: 0,
    lambdas: [],
    current_func: null,
    in_generator: false,
    gen_struct_name: "",
    gen_yield_count: 0,
    spawn_id: 0,
    spawn_funcs: [],
    spawn_captures: null,
    iface_by_name: Dict<Sym, LInterfaceDecl>(),
    impl_map: Dict<Sym, [string]>(),
    func_by_name: Dict<Sym, LFuncDecl>(),
    needs_os_args: false,
    needs_exec_cmd: false,
    needs_path_join: false,
    simple_enums: Dict<Sym, bool>(),
    tuple_types: Dict<Sym, string>(),
    tuple_count: 0,
    gen_funcs: Dict<Sym, bool>(),
    method_to_receiver: Dict<Sym, string>(),
    struct_by_name: Dict<Sym, LStructDecl>(),
    class_by_name: Dict<Sym, LClassDecl>(),
  }
  // Build interface lookup
  let ifaces = prog!.interfaces
  let mut i = 0
  while i < len(ifaces) {
    g.iface_by_name!.set(sym(ifaces[i].name), ifaces[i])
    i = i + 1
  }
  // Build implements map and class lookup from class declarations
  let classes = prog!.classes
  i = 0
  while i < len(classes) {
    g.class_by_name!.set(sym(classes[i].name), classes[i])
    if len(classes[i].implements) > 0 {
      g.impl_map!.set(sym(classes[i].name), classes[i].implements)
    }
    i = i + 1
  }
  // Build struct lookup
  let structs = prog!.structs
  i = 0
  while i < len(structs) {
    g.struct_by_name!.set(sym(structs[i].name), structs[i])
    i = i + 1
  }
  // Build function lookup
  let funcs = prog!.functions
  i = 0
  while i < len(funcs) {
    let mut key = funcs[i].name
    if funcs[i].receiver != "" {
      key = f"{funcs[i].receiver}.{funcs[i].name}"
      // Build method→receiver reverse index
      g.method_to_receiver!.set(sym(funcs[i].name), funcs[i].receiver)
    }
    g.func_by_name!.set(sym(key), funcs[i])
    // Track generator functions
    if !isnull(funcs[i].return_type) && funcs[i].return_type!.kind is TyGenerator {
      g.gen_funcs!.set(sym(funcs[i].name), true)
    }
    i = i + 1
  }
  return g
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func CGen.line(self, s: string) {
  let mut j = 0
  while j < self.indent {
    self.buf!.write("    ")
    j = j + 1
  }
  self.buf!.write(s)
  self.buf!.write("\n")
}

func CGen.write_raw(self, s: string) {
  self.buf!.write(s)
}

// ---------------------------------------------------------------------------
// C reserved words
// ---------------------------------------------------------------------------

func is_c_reserved(name: string) -> bool {
  if name == "auto" { return true }
  if name == "break" { return true }
  if name == "case" { return true }
  if name == "char" { return true }
  if name == "const" { return true }
  if name == "continue" { return true }
  if name == "default" { return true }
  if name == "do" { return true }
  if name == "double" { return true }
  if name == "else" { return true }
  if name == "enum" { return true }
  if name == "extern" { return true }
  if name == "float" { return true }
  if name == "for" { return true }
  if name == "goto" { return true }
  if name == "if" { return true }
  if name == "inline" { return true }
  if name == "int" { return true }
  if name == "long" { return true }
  if name == "register" { return true }
  if name == "restrict" { return true }
  if name == "return" { return true }
  if name == "short" { return true }
  if name == "signed" { return true }
  if name == "sizeof" { return true }
  if name == "static" { return true }
  if name == "struct" { return true }
  if name == "switch" { return true }
  if name == "typedef" { return true }
  if name == "union" { return true }
  if name == "unsigned" { return true }
  if name == "void" { return true }
  if name == "volatile" { return true }
  if name == "while" { return true }
  return false
}

func c_safe_name(name: string) -> string {
  if is_c_reserved(name) {
    return f"{name}_"
  }
  return name
}

func sanitize_c_type_name(s: string) -> string {
  let sb = new_string_builder()
  let mut i = 0
  while i < len(s) {
    let c = s[i]
    if c == ' ' as u8 {
      sb.write("_")
    } else if c == '*' as u8 {
      sb.write("ptr")
    } else if c == '[' as u8 || c == ']' as u8 || c == '(' as u8 || c == ')' as u8 {
      // skip
    } else if c == ',' as u8 || c == '/' as u8 {
      sb.write("_")
    } else {
      sb.write_byte(c)
    }
    i = i + 1
  }
  return sb.to_string()
}



func lc_first(s: string) -> string {
  if len(s) == 0 {
    return s
  }
  let c = s[0]
  if c >= 'A' as u8 && c <= 'Z' as u8 {
    let lower = char_to_string((c as i32 + 32) as u8)
    if len(s) == 1 {
      return lower
    }
    return f"{lower}{s[1:len(s)]}"
  }
  return s
}

func escape_c(s: string) -> string {
  let sb = new_string_builder()
  let mut i = 0
  while i < len(s) {
    let c = s[i]
    if c == '\\' as u8 {
      sb.write("\\\\")
    } else if c == '"' as u8 {
      sb.write("\\\"")
    } else if c == '\n' as u8 {
      sb.write("\\n")
    } else if c == '\r' as u8 {
      sb.write("\\r")
    } else if c == '\t' as u8 {
      sb.write("\\t")
    } else if c < 32 as u8 || c > 126 as u8 {
      sb.write("\\x")
      let hi = c / 16 as u8
      let lo = c % 16 as u8
      if hi < 10 as u8 {
        sb.write_byte(hi + '0' as u8)
      } else {
        sb.write_byte(hi - 10 as u8 + 'a' as u8)
      }
      if lo < 10 as u8 {
        sb.write_byte(lo + '0' as u8)
      } else {
        sb.write_byte(lo - 10 as u8 + 'a' as u8)
      }
    } else {
      sb.write_byte(c)
    }
    i = i + 1
  }
  return sb.to_string()
}

// Like escape_c but also escapes % → %% for printf format strings
func escape_printf(s: string) -> string {
  let escaped = escape_c(s)
  let sb = new_string_builder()
  let mut i = 0
  while i < len(escaped) {
    let c = escaped[i]
    if c == '%' as u8 {
      sb.write("%%")
    } else {
      sb.write_byte(c)
    }
    i = i + 1
  }
  return sb.to_string()
}

// ---------------------------------------------------------------------------
// Binary/Unary operators
// ---------------------------------------------------------------------------

func c_bin_op(op: LBinOpKind) -> string {
  match op {
    BinAdd => { return "+" }
    BinSub => { return "-" }
    BinMul => { return "*" }
    BinDiv => { return "/" }
    BinMod => { return "%" }
    BinEq => { return "==" }
    BinNe => { return "!=" }
    BinLt => { return "<" }
    BinLe => { return "<=" }
    BinGt => { return ">" }
    BinGe => { return ">=" }
    BinAnd => { return "&&" }
    BinOr => { return "||" }
    BinBitAnd => { return "&" }
    BinBitOr => { return "|" }
    BinBitXor => { return "^" }
    BinShl => { return "<<" }
    BinShr => { return ">>" }
  }
}

func c_un_op(op: LUnOpKind) -> string {
  match op {
    UnNeg => { return "-" }
    UnNot => { return "!" }
    UnBitNot => { return "~" }
  }
}

// ---------------------------------------------------------------------------
// Type mapping
// ---------------------------------------------------------------------------

func CGen.c_type(self, t: LType?) -> string {
  if isnull(t) {
    return "void"
  }
  match t!.kind {
    TyI8 => { return "int8_t" }
    TyI16 => { return "int16_t" }
    TyI32 => { return "int32_t" }
    TyI64 => { return "int64_t" }
    TyU8 => { return "uint8_t" }
    TyU16 => { return "uint16_t" }
    TyU32 => { return "uint32_t" }
    TyU64 => { return "uint64_t" }
    TyF32 => { return "float" }
    TyF64 => { return "double" }
    TyBool => { return "bool" }
    TyString => { return "lyric_string" }
    TyPlatformInt => { return "int" }
    TyPlatformUint => { return "unsigned int" }
    TyUnit => { return "void" }
    TyError => { return "const char*" }
    TyMutex => { return "pthread_mutex_t" }
    TyAny => {
      // Named interface type → boxed struct
      if t!.name != "" {
        let iface_entry = self.iface_by_name!.get(sym(t!.name))
        if !isnull(iface_entry) {
          return t!.name
        }
      }
      return "void*"
    }
    TyUnion => { return "LyricUnion" }
    TySlice => {
      return self.slice_type_name(t!.elem)
    }
    TyMap => { return "void* /* map */" }
    TyStruct => { return t!.name }
    TyClassHandle => {
      let mut name = t!.name
      if !isnull(self.prog!.class_renames) {
        let ren = self.prog!.class_renames!.get(sym(name))
        if !isnull(ren) {
          name = ren!.value
        }
      }
      if self.prog!.slab_mode_soa {
        return name
      }
      return f"{name}*"
    }
    TyOptional => {
      // Class handles are already pointers — NULL = none
      if !isnull(t!.elem) && t!.elem!.kind is TyClassHandle {
        if self.prog!.slab_mode_soa {
          return self.c_type(t!.elem)
        }
        return self.c_type(t!.elem)
      }
      return self.opt_type_name(t!.elem)
    }
    TyTaggedUnion => { return t!.name }
    TyGenerator => { return "void* /* generator */" }
    TyChannel => {
      if !isnull(t!.elem) {
        let suffix = self.chan_suffix(t!.elem)
        return f"LyricChan_{suffix}*"
      }
      return "void* /* channel */"
    }
    TyFuncPtr => {
      let ret = self.c_return_type(t!.ret)
      let sb = new_string_builder()
      let params = t!.params
      if len(params) == 0 {
        return f"{ret} (*)(void)"
      }
      let mut i = 0
      while i < len(params) {
        if i > 0 {
          sb.write(", ")
        }
        sb.write(self.c_type(params[i]))
        i = i + 1
      }
      return f"{ret} (*)({sb.to_string()})"
    }
    TyErrorResult => {
      return self.result_type_name(t!.elem)
    }
    TyTuple => {
      return self.c_tuple_type(t!)
    }
    TyTypeVar => {
      return f"void* /* typevar {t!.name} */"
    }
    _ => {
      return "/* unknown type */"
    }
  }
}

func CGen.c_field_decl(self, t: LType?, name: string) -> string {
  if !isnull(t) && t!.kind is TyFuncPtr {
    let ret = self.c_return_type(t!.ret)
    let params = t!.params
    if len(params) == 0 {
      return f"{ret} (*{name})(void)"
    }
    let sb = new_string_builder()
    let mut i = 0
    while i < len(params) {
      if i > 0 {
        sb.write(", ")
      }
      sb.write(self.c_type(params[i]))
      i = i + 1
    }
    return f"{ret} (*{name})({sb.to_string()})"
  }
  return f"{self.c_type(t)} {name}"
}

func CGen.c_return_type(self, t: LType?) -> string {
  if isnull(t) {
    return "void"
  }
  return self.c_type(t)
}

func CGen.c_tuple_type(self, t: LType?) -> string {
  if !isnull(t) && t!.kind is TyTuple && len(t!.fields) == 2 {
    let f0 = t!.fields[0]
    let f1 = t!.fields[1]
    if !isnull(f0.typ) && f0.typ!.kind is TyString && !isnull(f1.typ) && f1.typ!.kind is TyBool {
      return "lyric_str_bool_t"
    }
    if !isnull(f0.typ) && f0.typ!.kind is TyI64 && !isnull(f1.typ) && f1.typ!.kind is TyBool {
      return "lyric_atoi_result"
    }
    if !isnull(f0.typ) && f0.typ!.kind is TyF64 && !isnull(f1.typ) && f1.typ!.kind is TyBool {
      return "lyric_parse_float_result"
    }
  }
  // Fallback: register a named typedef (like slice_types/opt_types pattern)
  let mut fields_key = new_string_builder()
  let mut i2 = 0
  while i2 < len(t!.fields) {
    if i2 > 0 {
      fields_key.write(",")
    }
    fields_key.write(self.c_type(t!.fields[i2].typ))
    i2 = i2 + 1
  }
  let key = fields_key.to_string()
  let existing = self.tuple_types!.get(sym(key))
  if !isnull(existing) {
    return existing!.value
  }
  let name = f"LyricTuple_{self.tuple_count}"
  self.tuple_count = self.tuple_count + 1
  self.tuple_types!.set(sym(key), name)
  return name
}

// ---------------------------------------------------------------------------
// Composite type name helpers
// ---------------------------------------------------------------------------

func CGen.slice_type_name(self, elem: LType?) -> string {
  let key = self.c_type(elem)
  let entry = self.slice_types!.get(sym(key))
  if !isnull(entry) {
    return entry!.value
  }
  let name = f"LyricSlice_{sanitize_c_type_name(key)}"
  self.slice_types!.set(sym(key), name)
  return name
}

func CGen.opt_type_name(self, elem: LType?) -> string {
  if isnull(elem) {
    return "LyricOpt_void"
  }
  let key = self.c_type(elem)
  let entry = self.opt_types!.get(sym(key))
  if !isnull(entry) {
    return entry!.value
  }
  let name = f"LyricOpt_{sanitize_c_type_name(key)}"
  self.opt_types!.set(sym(key), name)
  return name
}

func CGen.result_type_name(self, elem: LType?) -> string {
  if isnull(elem) {
    return "LyricResult_void"
  }
  let key = self.c_type(elem)
  let entry = self.result_types!.get(sym(key))
  if !isnull(entry) {
    return entry!.value
  }
  let name = f"LyricResult_{sanitize_c_type_name(key)}"
  self.result_types!.set(sym(key), name)
  return name
}

func CGen.chan_suffix(self, elem: LType?) -> string {
  let key = self.c_type(elem)
  let entry = self.chan_types!.get(sym(key))
  if !isnull(entry) {
    return entry!.value
  }
  let suffix = sanitize_c_type_name(key)
  self.chan_types!.set(sym(key), suffix)
  return suffix
}

// ---------------------------------------------------------------------------
// Zero values
// ---------------------------------------------------------------------------

func CGen.zero_value(self, t: LType?) -> string {
  if isnull(t) {
    return "0"
  }
  match t!.kind {
    TyI8 | TyI16 | TyI32 | TyI64 | TyPlatformInt => { return "0" }
    TyU8 | TyU16 | TyU32 | TyU64 | TyPlatformUint => { return "0" }
    TyF32 | TyF64 => { return "0.0" }
    TyBool => { return "false" }
    TyString => { return "LYRIC_STR_EMPTY" }
    TyClassHandle => {
      if self.prog!.slab_mode_soa { return "0" }
      return "NULL"
    }
    TyOptional => {
      if !isnull(t!.elem) && t!.elem!.kind is TyClassHandle {
        if self.prog!.slab_mode_soa { return "0" }
        return "NULL"
      }
      return f"lyric_none({self.opt_type_name(t!.elem)})"
    }
    TySlice => {
      return f"lyric_slice_empty({self.slice_type_name(t!.elem)})"
    }
    TyMutex => { return "(pthread_mutex_t)PTHREAD_MUTEX_INITIALIZER" }
    TyTuple => { return "{0}" }
    TyStruct | TyTaggedUnion => { return "{0}" }
    _ => { return "0" }
  }
}

// ---------------------------------------------------------------------------
// Simple enum detection
// ---------------------------------------------------------------------------

func is_simple_enum(e: LEnumDecl) -> bool {
  let mut i = 0
  while i < len(e.variants) {
    if len(e.variants[i].fields) > 0 {
      return false
    }
    i = i + 1
  }
  return true
}

// ---------------------------------------------------------------------------
// Type helpers
// ---------------------------------------------------------------------------

func CGen.is_class_optional(self, t: LType?) -> bool {
  return (
    !isnull(t) && t!.kind is TyOptional
    && !isnull(t!.elem) && t!.elem!.kind is TyClassHandle
  )
}

// Resolve class name — name is pre-mangled by the monomorphizer; just pass through.
// The invariant is: for any ExClassGet/StClassSet, class_name == handle.typ.name
// after monomorphization. The C backend is a dumb emitter.
func CGen.resolve_class_name(self, name: string, caller: string) -> string {
  return name
}

// Resolve class name from a type (handles Optional<ClassHandle> too)
func CGen.resolve_class_name_from_type(self, t: LType?, caller: string) -> string {
  if isnull(t) { return "unknown" }
  if t!.kind is TyClassHandle {
    return self.resolve_class_name(t!.name, caller)
  }
  if t!.kind is TyOptional && !isnull(t!.elem) && t!.elem!.kind is TyClassHandle {
    return self.resolve_class_name(t!.elem!.name, caller)
  }
  return "unknown"
}

// Check if a destroy function exists for a class
func CGen.has_destroy_func(self, class_name: string) -> bool {
  let target = class_name + "_destroy"
  let mut i = 0
  while i < len(self.prog!.functions) {
    if self.prog!.functions[i].name == target {
      return true
    }
    // Also check "destroy" with receiver matching class_name
    if self.prog!.functions[i].name == "destroy" {
      if len(self.prog!.functions[i].params) > 0 {
        if !isnull(self.prog!.functions[i].params[0].typ) {
          if self.prog!.functions[i].params[0].typ!.name == class_name {
            return true
          }
        }
      }
    }
    i = i + 1
  }
  return false
}


func CGen.contains_type_var(self, t: LType?) -> bool {
  if isnull(t) {
    return false
  }
  if t!.kind is TyTypeVar {
    return true
  }
  let has_elem = self.contains_type_var(t!.elem)
  let has_key = self.contains_type_var(t!.key)
  let has_ret = self.contains_type_var(t!.ret)
  return has_elem || has_key || has_ret
}

func CGen.next_temp(self) -> i32 {
  let id = 9000 + len(self.temp_used!.keys())
  self.temp_used!.set(sym(f"{id}"), true)
  return id
}

// ---------------------------------------------------------------------------
// Function name helpers
// ---------------------------------------------------------------------------

func CGen.func_name(self, f: LFuncDecl) -> string {
  if f.receiver != "" {
    return f"{f.receiver}_{f.name}"
  }
  if f.name == "Main" {
    return "main"
  }
  return c_safe_name(f.name)
}

func CGen.c_param_list(self, f: LFuncDecl) -> string {
  self.mut_params = Dict<Sym, bool>()
  let sb = new_string_builder()
  let mut count = 0
  // For methods, ensure self is the first parameter
  let has_self = len(f.params) > 0 && f.params[0].name == "self"
  if f.receiver != "" && !has_self {
    let self_type = LType { kind: TyClassHandle, name: f.receiver, elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 0, is_exported: false }
    sb.write(self.c_field_decl(self_type, "self"))
    count = count + 1
  }
  let mut i = 0
  while i < len(f.params) {
    if count > 0 {
      sb.write(", ")
    }
    if f.params[i].mutable {
      // Mut param: emit as pointer type
      sb.write(self.c_type(f.params[i].typ) + "* " + c_safe_name(f.params[i].name))
      self.mut_params!.set(sym(f.params[i].name), true)
    } else {
      sb.write(self.c_field_decl(f.params[i].typ, f.params[i].name))
    }
    count = count + 1
    i = i + 1
  }
  if count == 0 {
    return "void"
  }
  return sb.to_string()
}

func CGen.is_generator_func(self, f: LFuncDecl) -> bool {
  return !isnull(f.return_type) && f.return_type!.kind is TyGenerator
}

// ---------------------------------------------------------------------------
// Value emission
// ---------------------------------------------------------------------------

func CGen.emit_value(self, v: LValue?) -> string {
  if isnull(v) {
    return "/* null value */"
  }
  match v!.kind {
    ValVar => {
      let name = c_safe_name(v!.name)
      if self.in_generator {
        return f"_gen->{name}"
      }
      if !isnull(self.spawn_captures) {
        let entry = self.spawn_captures!.get(sym(v!.name))
        if !isnull(entry) {
          return f"(*_ctx->{name})"
        }
      }
      if !isnull(self.mut_params) {
        let mp = self.mut_params!.get(sym(v!.name))
        if !isnull(mp) {
          return f"(*{name})"
        }
      }
      return name
    }
    ValTemp => {
      return f"_t{v!.temp_id}"
    }
    ValGlobal => {
      return v!.name
    }
    ValLitInt => {
      return f"{v!.int_val}"
    }
    ValLitUint => {
      return f"{v!.uint_val}U"
    }
    ValLitFloat => {
      return f"{v!.float_val}"
    }
    ValLitString => {
      return f"LYRIC_STR(\"{escape_c(v!.str_val)}\")"
    }
    ValLitBool => {
      if v!.bool_val {
        return "true"
      }
      return "false"
    }
    ValLitNull => {
      return self.zero_value(v!.typ)
    }
    ValIndexRef => {
      let coll = self.emit_value(v!.collection)
      let idx = self.emit_value(v!.index)
      return f"{coll}.data[{idx}]"
    }
    ValClassFieldRef => {
      let handle = self.emit_value(v!.collection)
      if self.prog!.slab_mode_soa {
        let recv_type = v!.collection!.typ
        let mut cname = ""
        if !isnull(recv_type) {
          let mut inner = recv_type
          if inner!.kind is TyOptional && !isnull(inner!.elem) {
            inner = inner!.elem
          }
          cname = inner!.name
        }
        return f"_lyric_slab_{cname}.{v!.name}[{handle}]"
      }
      return f"{handle}->{v!.name}"
    }
  }
}

func CGen.emit_class_msg_data(self, v: LValue?, class_name: string) -> string {
  let val = self.emit_value(v)
  if self.prog!.slab_mode_soa {
    let cname = self.resolve_class_name(class_name, "emit_class_msg_data")
    return f"(const char*)_lyric_slab_{cname}.msg[{val}].data"
  }
  return f"(const char*){val}->msg.data"
}

func CGen.emit_value_as_cstr(self, v: LValue?) -> string {
  if isnull(v) {
    return "\"<null>\""
  }
  if v!.kind is ValLitString {
    return f"\"{escape_c(v!.str_val)}\""
  }
  // For error types, use directly
  if v!.kind is ValTemp {
    let ty_entry = self.temp_types!.get(sym(f"{v!.temp_id}"))
    if !isnull(ty_entry) && !isnull(ty_entry!.value) {
      if ty_entry!.value!.kind is TyError {
        return self.emit_value(v)
      }
      if ty_entry!.value!.kind is TyClassHandle {
        return self.emit_class_msg_data(v, ty_entry!.value!.name)
      }
    }
  }
  // Also check varTypes — multi-return destructuring creates VarDecls with _tN names
  if v!.kind is ValTemp {
    let var_name = f"_t{v!.temp_id}"
    let vt_entry = self.var_types!.get(sym(var_name))
    if !isnull(vt_entry) && !isnull(vt_entry!.value) {
      if vt_entry!.value!.kind is TyError {
        return self.emit_value(v)
      }
      if vt_entry!.value!.kind is TyClassHandle {
        return self.emit_class_msg_data(v, vt_entry!.value!.name)
      }
    }
  }
  if v!.kind is ValVar {
    let ty_entry = self.var_types!.get(sym(v!.name))
    if !isnull(ty_entry) && !isnull(ty_entry!.value) {
      if ty_entry!.value!.kind is TyError {
        return self.emit_value(v)
      }
      if ty_entry!.value!.kind is TyClassHandle {
        return self.emit_class_msg_data(v, ty_entry!.value!.name)
      }
    }
  }
  return f"(const char*){self.emit_value(v)}.data"
}

func CGen.emit_args(self, args: [LValue?]) -> string {
  let sb = new_string_builder()
  let mut i = 0
  while i < len(args) {
    if i > 0 {
      sb.write(", ")
    }
    sb.write(self.emit_value(args[i]))
    i = i + 1
  }
  return sb.to_string()
}

func CGen.emit_args_with_mut(self, args: [LValue?], mut_args: [bool]) -> string {
  let sb = new_string_builder()
  let mut i = 0
  while i < len(args) {
    if i > 0 {
      sb.write(", ")
    }
    if len(mut_args) > i && mut_args[i] {
      sb.write("&" + self.emit_value(args[i]))
    } else {
      sb.write(self.emit_value(args[i]))
    }
    i = i + 1
  }
  return sb.to_string()
}

// ---------------------------------------------------------------------------
// Resolve value type from temp/var maps
// ---------------------------------------------------------------------------

func CGen.resolve_value_type(self, v: LValue?) -> LType? {
  if isnull(v) {
    return null
  }
  if !isnull(v!.typ) {
    return v!.typ
  }
  if v!.kind is ValTemp {
    let entry = self.temp_types!.get(sym(f"{v!.temp_id}"))
    if !isnull(entry) {
      return entry!.value
    }
  }
  if v!.kind is ValVar {
    let entry = self.var_types!.get(sym(v!.name))
    if !isnull(entry) {
      return entry!.value
    }
  }
  return null
}

func CGen.infer_lval_type(self, v: LValue?) -> LType? {
  if isnull(v) {
    return null
  }
  if !isnull(v!.typ) {
    return v!.typ
  }
  match v!.kind {
    ValLitInt => {
      return LType { kind: TyI32, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 32, is_exported: false }
    }
    ValLitUint => {
      return LType { kind: TyU32, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 32, is_exported: false }
    }
    ValLitFloat => {
      return LType { kind: TyF64, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 64, is_exported: false }
    }
    ValLitString => {
      return LType { kind: TyString, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 0, is_exported: false }
    }
    ValLitBool => {
      return LType { kind: TyBool, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 0, is_exported: false }
    }
    ValTemp => {
      let entry = self.temp_types!.get(sym(f"{v!.temp_id}"))
      if !isnull(entry) {
        return entry!.value
      }
    }
    ValVar => {
      let entry = self.var_types!.get(sym(v!.name))
      if !isnull(entry) {
        return entry!.value
      }
    }
    _ => {}
  }
  return null
}

// ---------------------------------------------------------------------------
// Slice type from value
// ---------------------------------------------------------------------------

func CGen.slice_type_name_from_value(self, v: LValue?) -> string {
  if !isnull(v) && !isnull(v!.typ) && v!.typ!.kind is TySlice {
    return self.slice_type_name(v!.typ!.elem)
  }
  return "/* unknown slice type */"
}

func CGen.slice_type_name_from_type(self, t: LType?) -> string {
  if !isnull(t) && t!.kind is TySlice {
    return self.slice_type_name(t!.elem)
  }
  return "/* unknown slice type */"
}

func CGen.opt_type_name_from_expr(self, e: LExpr?) -> string {
  if !isnull(e) && !isnull(e!.typ) && e!.typ!.kind is TyOptional {
    return self.opt_type_name(e!.typ!.elem)
  }
  if !isnull(e) && e!.kind is ExWrapOptional {
    if !isnull(e!.wrap_opt) && !isnull(e!.wrap_opt!.value) && !isnull(e!.wrap_opt!.value!.typ) {
      return self.opt_type_name(e!.wrap_opt!.value!.typ)
    }
  }
  return "LyricOpt_void"
}

func CGen.result_type_name_from_expr(self, e: LExpr?) -> string {
  if !isnull(e) && !isnull(e!.typ) && e!.typ!.kind is TyErrorResult && !isnull(e!.typ!.elem) {
    return self.result_type_name(e!.typ!.elem)
  }
  return "LyricResult_void"
}

// ---------------------------------------------------------------------------
// Find class field type
// ---------------------------------------------------------------------------

func CGen.find_class_field(self, fields: [LField], name: string) -> LType? {
  let lc_name = lc_first(name)
  let mut i = 0
  while i < len(fields) {
    if fields[i].name == name || fields[i].name == lc_name {
      return fields[i].typ
    }
    i = i + 1
  }
  return null
}

// Resolve field type from program declarations
func CGen.resolve_field_type(self, owner_type: LType?, field: string) -> LType? {
  if isnull(owner_type) {
    return null
  }
  let mut name = owner_type!.name
  if !isnull(self.prog!.class_renames) {
    let ren = self.prog!.class_renames!.get(sym(name))
    if !isnull(ren) {
      name = ren!.value
    }
  }
  let s_entry = self.struct_by_name!.get(sym(name))
  if !isnull(s_entry) {
    let s = s_entry!.value
    let mut j = 0
    while j < len(s.fields) {
      if s.fields[j].name == field {
        return s.fields[j].typ
      }
      j = j + 1
    }
  }
  let c_entry = self.class_by_name!.get(sym(name))
  if !isnull(c_entry) {
    let c = c_entry!.value
    let mut j = 0
    while j < len(c.fields) {
      if c.fields[j].name == field {
        return c.fields[j].typ
      }
      j = j + 1
    }
  }
  return null
}

// ---------------------------------------------------------------------------
// Union support
// ---------------------------------------------------------------------------

func CGen.union_tag_for_type(self, t: LType?) -> string {
  if isnull(t) {
    return "LYRIC_UNION_TAG_PTR"
  }
  match t!.kind {
    TyI8 | TyI16 | TyI32 | TyU8 | TyU16 | TyU32 | TyPlatformInt => { return "LYRIC_UNION_TAG_I32" }
    TyI64 | TyU64 | TyPlatformUint => { return "LYRIC_UNION_TAG_I64" }
    TyF32 => { return "LYRIC_UNION_TAG_F32" }
    TyF64 => { return "LYRIC_UNION_TAG_F64" }
    TyBool => { return "LYRIC_UNION_TAG_BOOL" }
    TyString => { return "LYRIC_UNION_TAG_STRING" }
    _ => { return "LYRIC_UNION_TAG_PTR" }
  }
}

func CGen.c_wrap_union(self, expr: string, src_type: LType?) -> string {
  if isnull(src_type) {
    return f"lyric_union_ptr((void*)({expr}))"
  }
  match src_type!.kind {
    TyI8 | TyI16 | TyI32 | TyU8 | TyU16 | TyU32 | TyPlatformInt => {
      return f"lyric_union_i32((int32_t)({expr}))"
    }
    TyI64 | TyU64 | TyPlatformUint => {
      return f"lyric_union_i64((int64_t)({expr}))"
    }
    TyF32 => { return f"lyric_union_f32({expr})" }
    TyF64 => { return f"lyric_union_f64({expr})" }
    TyBool => { return f"lyric_union_bool({expr})" }
    TyString => { return f"lyric_union_string({expr})" }
    _ => { return f"lyric_union_ptr((void*)({expr}))" }
  }
}

// ---------------------------------------------------------------------------
// Printf format specifiers
// ---------------------------------------------------------------------------

func CGen.printf_spec_and_arg(self, v: LValue?) -> (string, string) {
  let mut t = if !isnull(v) { v!.typ } else { null }
  // Resolve through temp/var types
  if !isnull(v) && v!.kind is ValTemp {
    let entry = self.temp_types!.get(sym(f"{v!.temp_id}"))
    if !isnull(entry) && !isnull(entry!.value) {
      t = entry!.value
    }
  } else if !isnull(v) && v!.kind is ValVar {
    let entry = self.var_types!.get(sym(v!.name))
    if !isnull(entry) && !isnull(entry!.value) {
      t = entry!.value
    }
  }
  let val = self.emit_value(v)
  if isnull(t) {
    return ("%d", val)
  }
  match t!.kind {
    TyI8 | TyI16 | TyI32 | TyPlatformInt => { return ("%d", val) }
    TyI64 => { return ("%lld", val) }
    TyU8 | TyU16 | TyU32 | TyPlatformUint => { return ("%u", val) }
    TyU64 => { return ("%llu", val) }
    TyF32 | TyF64 => { return ("%g", val) }
    TyBool => { return ("%s", f"lyric_bool_str({val})") }
    TyString => { return ("%.*s", f"(int){val}.len, (const char*){val}.data") }
    TyError => { return ("%s", val) }
    TyTaggedUnion => {
      let to_str = f"{t!.name}_to_string({val})"
      return ("%.*s", f"(int){to_str}.len, (const char*){to_str}.data")
    }
    TyAny => { return ("%p", val) }
    _ => { return ("%d", val) }
  }
}

// ---------------------------------------------------------------------------
// Print helpers
// ---------------------------------------------------------------------------

func CGen.emit_fprint(self, stream: string, args: [LValue?], newline: bool) -> string {
  if len(args) == 0 {
    if newline {
      return f"fprintf({stream}, \"\\n\")"
    }
    return f"fprintf({stream}, \"\")"
  }
  // Check if any arg is a union — requires special handling
  let mut has_union = false
  let mut j = 0
  while j < len(args) {
    let t = self.resolve_value_type(args[j])
    if !isnull(t) && t!.kind is TyUnion {
      has_union = true
    }
    j = j + 1
  }
  if has_union {
    let stmts = new_string_builder()
    let mut i = 0
    while i < len(args) {
      if i > 0 {
        if stmts.len() > 0 { stmts.write("; ") }
        stmts.write(f"fprintf({stream}, \" \")")
      }
      let t = self.resolve_value_type(args[i])
      if !isnull(t) && t!.kind is TyUnion {
        if stmts.len() > 0 { stmts.write("; ") }
        stmts.write(f"lyric_union_fprint({stream}, {self.emit_value(args[i])})")
      } else {
        let result = self.printf_spec_and_arg(args[i])
        if stmts.len() > 0 { stmts.write("; ") }
        if result._1 != "" {
          stmts.write(f"fprintf({stream}, \"{result._0}\", {result._1})")
        } else {
          stmts.write(f"fprintf({stream}, \"{result._0}\")")
        }
      }
      i = i + 1
    }
    if newline {
      stmts.write(f"; fprintf({stream}, \"\\n\")")
    }
    return stmts.to_string()
  }
  let fmt_sb = new_string_builder()
  let arg_sb = new_string_builder()
  let mut arg_count = 0
  let mut i = 0
  while i < len(args) {
    if i > 0 {
      fmt_sb.write(" ")
    }
    let result = self.printf_spec_and_arg(args[i])
    fmt_sb.write(result._0)
    if result._1 != "" {
      if arg_count > 0 {
        arg_sb.write(", ")
      }
      arg_sb.write(result._1)
      arg_count = arg_count + 1
    }
    i = i + 1
  }
  let mut fmt_str = fmt_sb.to_string()
  if newline {
    fmt_str = f"{fmt_str}\\n"
  }
  if arg_count == 0 {
    return f"fprintf({stream}, \"{fmt_str}\")"
  }
  return f"fprintf({stream}, \"{fmt_str}\", {arg_sb.to_string()})"
}

func CGen.emit_println(self, args: [LValue?]) -> string {
  return self.emit_fprint("stdout", args, true)
}

func CGen.emit_printf(self, args: [LValue?]) -> string {
  if len(args) == 0 {
    return "printf(\"\")"
  }
  let fmt_arg = f"(const char*){self.emit_value(args[0])}.data"
  let sb = new_string_builder()
  let mut i = 1
  while i < len(args) {
    if i > 1 {
      sb.write(", ")
    }
    sb.write(self.emit_value(args[i]))
    i = i + 1
  }
  if i == 1 {
    return f"printf({fmt_arg})"
  }
  return f"printf({fmt_arg}, {sb.to_string()})"
}

func CGen.emit_sprintf(self, args: [LValue?]) -> string {
  if len(args) == 0 {
    return "LYRIC_STR_EMPTY"
  }
  let fmt_str = f"(const char*){self.emit_value(args[0])}.data"
  if len(args) == 1 {
    return self.emit_value(args[0])
  }
  let sb = new_string_builder()
  let mut i = 1
  while i < len(args) {
    if i > 1 {
      sb.write(", ")
    }
    sb.write(self.emit_value(args[i]))
    i = i + 1
  }
  return f"lyric_sprintf({fmt_str}, {sb.to_string()})"
}

func CGen.emit_format(self, d: LFormatData?) -> string {
  if isnull(d) {
    return "LYRIC_STR_EMPTY"
  }
  let fmt_sb = new_string_builder()
  let arg_sb = new_string_builder()
  let mut arg_count = 0
  let mut i = 0
  while i < len(d!.parts) {
    let p = d!.parts[i]
    if p.is_literal {
      fmt_sb.write(escape_printf(p.text))
    } else {
      let result = self.printf_spec_and_arg(p.value)
      fmt_sb.write(result._0)
      if result._1 != "" {
        if arg_count > 0 {
          arg_sb.write(", ")
        }
        arg_sb.write(result._1)
        arg_count = arg_count + 1
      }
    }
    i = i + 1
  }
  let fmt_str = fmt_sb.to_string()
  if arg_count == 0 {
    return f"LYRIC_STR(\"{fmt_str}\")"
  }
  return f"lyric_sprintf(\"{fmt_str}\", {arg_sb.to_string()})"
}

// ---------------------------------------------------------------------------
// Builtin emission
// ---------------------------------------------------------------------------

func CGen.emit_builtin(self, d: LBuiltinData?) -> string {
  if isnull(d) {
    return "/* null builtin */"
  }
  let name = d!.name
  let args = d!.args

  if name == "len" || name == "string_len" || name == "str_len" {
    if len(args) > 0 {
      return f"{self.emit_value(args[0])}.len"
    }
  }
  if name == "slice_len" {
    if len(args) > 0 {
      return f"{self.emit_value(args[0])}.len"
    }
    return "0"
  }
  if name == "append" || name == "slice_push" {
    if len(args) >= 2 {
      let slice_arg = self.emit_value(args[0])
      let elem_arg = self.emit_value(args[1])
      let slice_type = self.slice_type_name_from_value(args[0])
      return f"({{ lyric_push(&{slice_arg}, {elem_arg}, {slice_type}); {slice_arg}; }})"
    }
  }
  if name == "push_bytes" {
    if len(args) >= 2 {
      let dst_arg = self.emit_value(args[0])
      let src_arg = self.emit_value(args[1])
      return f"({{ lyric_push_bytes(&{dst_arg}, {src_arg}); {dst_arg}; }})"
    }
  }
  if name == "slice_extend" {
    if len(args) >= 2 {
      let dst_arg = self.emit_value(args[0])
      let src_arg = self.emit_value(args[1])
      let slice_type = self.slice_type_name_from_value(args[0])
      return f"({{ lyric_extend(&{dst_arg}, {src_arg}, {slice_type}); {dst_arg}; }})"
    }
  }
  if name == "pop" || name == "slice_pop" {
    if len(args) > 0 {
      return f"lyric_pop(&{self.emit_value(args[0])})"
    }
  }
  if name == "isnull" {
    if len(args) > 0 {
      let arg_type = self.resolve_value_type(args[0])
      if !isnull(arg_type) {
        let is_class = arg_type!.kind is TyClassHandle
        let is_any = arg_type!.kind is TyAny
        let is_opt_class = (
          arg_type!.kind is TyOptional
          && !isnull(arg_type!.elem) && arg_type!.elem!.kind is TyClassHandle
        )
        let is_error = arg_type!.kind is TyError
        if is_class || is_any || is_opt_class || is_error {
          if self.prog!.slab_mode_soa && (is_class || is_opt_class) {
            return f"({self.emit_value(args[0])} == 0)"
          }
          return f"({self.emit_value(args[0])} == NULL)"
        }
      }
      return f"lyric_isnull({self.emit_value(args[0])})"
    }
    return "false"
  }
  if name == "has_prefix" || name == "str_has_prefix" || name == "string_has_prefix" {
    if len(args) >= 2 {
      return f"lyric_str_has_prefix({self.emit_value(args[0])}, {self.emit_value(args[1])})"
    }
  }
  if name == "has_suffix" || name == "str_has_suffix" || name == "string_has_suffix" {
    if len(args) >= 2 {
      return f"lyric_str_has_suffix({self.emit_value(args[0])}, {self.emit_value(args[1])})"
    }
  }
  if name == "starts_with" || name == "str_starts_with" {
    if len(args) >= 2 {
      return f"lyric_str_has_prefix({self.emit_value(args[0])}, {self.emit_value(args[1])})"
    }
  }
  if name == "ends_with" || name == "str_ends_with" {
    if len(args) >= 2 {
      return f"lyric_str_has_suffix({self.emit_value(args[0])}, {self.emit_value(args[1])})"
    }
  }
  if name == "str_is_empty" || name == "string_is_empty" {
    if len(args) > 0 {
      return f"({self.emit_value(args[0])}.len == 0)"
    }
  }
  if name == "str_char_at" || name == "string_char_at" || name == "char_at" {
    if len(args) >= 2 {
      let sv = self.emit_value(args[0])
      let iv = self.emit_value(args[1])
      return "((lyric_string){.data = (char*)" + sv + ".data + " + iv + ", .len = 1, .cap = 0})"
    }
  }
  if name == "contains" || name == "str_contains" || name == "string_contains" || name == "slice_contains" {
    if len(args) >= 2 {
      if !isnull(args[0]) && !isnull(args[0]!.typ) && args[0]!.typ!.kind is TyString {
        return f"lyric_str_contains({self.emit_value(args[0])}, {self.emit_value(args[1])})"
      }
      return f"lyric_contains({self.emit_value(args[0])}, {self.emit_value(args[1])})"
    }
  }
  if name == "index_of" || name == "str_index_of" || name == "string_index_of" {
    if len(args) >= 2 {
      return f"lyric_str_index_of({self.emit_value(args[0])}, {self.emit_value(args[1])})"
    }
  }
  if name == "replace" || name == "str_replace" || name == "string_replace" {
    if len(args) >= 3 {
      return f"lyric_str_replace({self.emit_value(args[0])}, {self.emit_value(args[1])}, {self.emit_value(args[2])})"
    }
  }
  if name == "join" || name == "str_join" || name == "slice_join" {
    if len(args) >= 2 {
      let slice_val = self.emit_value(args[0])
      let sep = self.emit_value(args[1])
      return f"lyric_str_join({sep}, {slice_val}.data, {slice_val}.len)"
    }
  }
  if name == "slice_is_empty" {
    if len(args) > 0 {
      return f"({self.emit_value(args[0])}.len == 0)"
    }
  }
  if name == "slice_first" {
    if len(args) > 0 && !isnull(args[0]) && !isnull(args[0]!.typ) {
      let sv = self.emit_value(args[0])
      let opt_name = self.opt_type_name(args[0]!.typ!.elem)
      return f"(({sv}.len == 0) ? lyric_none({opt_name}) : lyric_some({sv}.data[0], {opt_name}))"
    }
  }
  if name == "slice_last" {
    if len(args) > 0 && !isnull(args[0]) && !isnull(args[0]!.typ) {
      let sv = self.emit_value(args[0])
      let opt_name = self.opt_type_name(args[0]!.typ!.elem)
      return f"(({sv}.len == 0) ? lyric_none({opt_name}) : lyric_some({sv}.data[{sv}.len - 1], {opt_name}))"
    }
  }
  if name == "slice_index_of" {
    if len(args) >= 2 {
      let sv = self.emit_value(args[0])
      let ev = self.emit_value(args[1])
      return "(" + "{ int32_t _idx = -1; for (int32_t _i = 0; _i < " + sv + ".len; _i++) { if (" + sv + ".data[_i] == " + ev + ") { _idx = _i; break; } } _idx; })"
    }
  }
  if name == "slice_remove" {
    if len(args) >= 2 {
      let sv = self.emit_value(args[0])
      let ev = self.emit_value(args[1])
      return "(" + "{ for (int32_t _i = 0; _i < " + sv + ".len; _i++) { if (" + sv + ".data[_i] == " + ev + ") { memmove(&" + sv + ".data[_i], &" + sv + ".data[_i+1], (" + sv + ".len-_i-1)*sizeof(*" + sv + ".data)); (&" + sv + ")->len--; break; } } })"
    }
  }
  if name == "slice_clear" {
    if len(args) > 0 {
      return f"((&{self.emit_value(args[0])})->len = 0)"
    }
  }
  if name == "slice_reverse" {
    if len(args) > 0 {
      let sv = self.emit_value(args[0])
      return "(" + "{ int32_t _l = 0, _r = " + sv + ".len - 1; while (_l < _r) { __typeof__(" + sv + ".data[0]) _tmp = " + sv + ".data[_l]; " + sv + ".data[_l++] = " + sv + ".data[_r]; " + sv + ".data[_r--] = _tmp; } })"
    }
  }
  if name == "repeat" || name == "str_repeat" || name == "string_repeat" {
    if len(args) >= 2 {
      return f"lyric_str_repeat({self.emit_value(args[0])}, {self.emit_value(args[1])})"
    }
  }
  if name == "string_to_upper" || name == "str_to_upper" {
    if len(args) > 0 {
      return f"lyric_toupper({self.emit_value(args[0])})"
    }
  }
  if name == "string_to_lower" || name == "str_to_lower" {
    if len(args) > 0 {
      return f"lyric_tolower({self.emit_value(args[0])})"
    }
  }
  if name == "string_split" || name == "str_split" {
    if len(args) >= 2 {
      return f"lyric_str_split({self.emit_value(args[0])}, {self.emit_value(args[1])})"
    }
  }
  if name == "string_trim" || name == "str_trim" {
    if len(args) > 0 {
      return f"lyric_str_trim({self.emit_value(args[0])})"
    }
  }
  if name == "hash_string" {
    if len(args) > 0 {
      return f"lyric_hash_string({self.emit_value(args[0])})"
    }
    return "0"
  }
  if name == "eprint" {
    return self.emit_fprint("stderr", args, false)
  }
  if name == "eprintln" {
    return self.emit_fprint("stderr", args, true)
  }
  if name == "read_file" {
    if len(args) > 0 {
      return f"lyric_read_file({self.emit_value(args[0])})"
    }
  }
  if name == "write_file" {
    if len(args) >= 2 {
      return f"lyric_write_file({self.emit_value(args[0])}, {self.emit_value(args[1])})"
    }
  }
  if name == "os_args" {
    self.needs_os_args = true
    return "_lyric_os_args(_argc, _argv)"
  }
  if name == "os_exit" {
    if len(args) > 0 {
      return f"exit({self.emit_value(args[0])})"
    }
    return "exit(0)"
  }
  if name == "os_getwd" {
    return "lyric_getwd()"
  }
  if name == "list_dir" {
    if len(args) > 0 {
      return f"lyric_list_dir({self.emit_value(args[0])})"
    }
  }
  if name == "file_exists" {
    if len(args) > 0 {
      return f"lyric_file_exists({self.emit_value(args[0])})"
    }
  }
  if name == "mkdtemp" {
    if len(args) > 0 {
      return f"lyric_mkdtemp({self.emit_value(args[0])})"
    }
  }
  if name == "exec_command" {
    self.needs_exec_cmd = true
    if len(args) >= 2 {
      return f"_lyric_exec_command({self.emit_value(args[0])}, {self.emit_value(args[1])})"
    }
  }
  if name == "path_join" {
    self.needs_path_join = true
    if len(args) > 0 {
      return f"_lyric_path_join({self.emit_value(args[0])})"
    }
  }
  if name == "path_dir" {
    if len(args) > 0 {
      return f"lyric_path_dir({self.emit_value(args[0])})"
    }
  }
  if name == "path_base" {
    if len(args) > 0 {
      return f"lyric_path_base({self.emit_value(args[0])})"
    }
  }
  if name == "path_ext" {
    if len(args) > 0 {
      return f"lyric_path_ext({self.emit_value(args[0])})"
    }
  }
  if name == "itoa" {
    if len(args) > 0 {
      return f"lyric_itoa({self.emit_value(args[0])})"
    }
  }
  if name == "atoi" {
    if len(args) > 0 {
      return f"lyric_atoi({self.emit_value(args[0])})"
    }
  }
  if name == "parse_float" {
    if len(args) > 0 {
      return f"lyric_parse_float({self.emit_value(args[0])})"
    }
  }
  if name == "char_to_string" {
    if len(args) > 0 {
      return f"lyric_char_to_string({self.emit_value(args[0])})"
    }
  }
  if name == "println" || name == "Println" || name == "fmt.Println" {
    return self.emit_println(args)
  }
  if name == "print" || name == "Print" || name == "fmt.Print" {
    return self.emit_fprint("stdout", args, false)
  }
  if name == "assert" {
    if len(args) >= 1 {
      let cond = self.emit_value(args[0])
      let mut msg = "LYRIC_STR(\"assertion failed\")"
      if len(args) >= 2 {
        msg = self.emit_value(args[1])
      }
      return f"lyric_assert({cond}, {msg}, \"{d!.file}\", {itoa(d!.line)})"
    }
  }
  if name == "assert_eq" {
    if len(args) >= 2 {
      let to_str_a = self.to_string_expr(args[0])
      let to_str_e = self.to_string_expr(args[1])
      let eq = self.eq_expr(args[0], args[1])
      let mut msg = "LYRIC_STR(\"assert_eq failed\")"
      if len(args) >= 3 {
        msg = self.emit_value(args[2])
      }
      return f"lyric_assert_eq({eq}, {to_str_a}, {to_str_e}, {msg}, \"{d!.file}\", {itoa(d!.line)})"
    }
  }
  if name == "panic" {
    if len(args) >= 1 {
      return f"lyric_panic({self.emit_value(args[0])})"
    }
  }
  if name == "channel_receive" {
    if len(args) > 0 {
      let chan_type = self.resolve_value_type(args[0])
      let mut suffix = "void"
      if !isnull(chan_type) && chan_type!.kind is TyChannel && !isnull(chan_type!.elem) {
        suffix = self.chan_suffix(chan_type!.elem)
      }
      return f"lyric_chan_recv_{suffix}({self.emit_value(args[0])})"
    }
  }
  if name == "channel_close" {
    if len(args) > 0 {
      let chan_type = self.resolve_value_type(args[0])
      let mut suffix = "void"
      if !isnull(chan_type) && chan_type!.kind is TyChannel && !isnull(chan_type!.elem) {
        suffix = self.chan_suffix(chan_type!.elem)
      }
      return f"lyric_chan_close_{suffix}({self.emit_value(args[0])})"
    }
  }
  // Map builtins — stubs
  if name == "map_len" { return "0 /* map_len: maps not supported */" }
  if name == "map_contains_key" || name == "contains_key" { return "false /* contains_key: not supported */" }

  // I/O builtins
  if name == "read_file" {
    if len(args) > 0 {
      return f"lyric_read_file({self.emit_value(args[0])})"
    }
  }
  if name == "write_file" {
    if len(args) >= 2 {
      return f"lyric_write_file({self.emit_value(args[0])}, {self.emit_value(args[1])})"
    }
  }
  if name == "os_args" {
    self.needs_os_args = true
    return "_lyric_os_args(_argc, _argv)"
  }
  if name == "os_exit" {
    if len(args) > 0 {
      return f"exit({self.emit_value(args[0])})"
    }
    return "exit(0)"
  }
  if name == "os_getwd" {
    return "lyric_getwd()"
  }
  if name == "list_dir" {
    if len(args) > 0 {
      return f"lyric_list_dir({self.emit_value(args[0])})"
    }
  }
  if name == "file_exists" {
    if len(args) > 0 {
      return f"lyric_file_exists({self.emit_value(args[0])})"
    }
  }
  if name == "mkdtemp" {
    if len(args) > 0 {
      return f"lyric_mkdtemp({self.emit_value(args[0])})"
    }
  }
  if name == "itoa" {
    if len(args) > 0 {
      return f"lyric_itoa({self.emit_value(args[0])})"
    }
  }
  if name == "atoi" {
    if len(args) > 0 {
      return f"lyric_atoi({self.emit_value(args[0])})"
    }
  }
  if name == "char_to_string" {
    if len(args) > 0 {
      return f"lyric_char_to_string({self.emit_value(args[0])})"
    }
  }
  if name == "hash_string" {
    if len(args) > 0 {
      return f"lyric_hash_string({self.emit_value(args[0])})"
    }
  }
  if name == "exec_command" {
    self.needs_exec_cmd = true
    if len(args) >= 2 {
      return f"_lyric_exec_command({self.emit_value(args[0])}, {self.emit_value(args[1])})"
    }
  }
  if name == "path_join" {
    self.needs_path_join = true
    if len(args) > 0 {
      return f"_lyric_path_join({self.emit_value(args[0])})"
    }
  }
  if name == "path_dir" {
    if len(args) > 0 {
      return f"lyric_path_dir({self.emit_value(args[0])})"
    }
  }
  if name == "path_base" {
    if len(args) > 0 {
      return f"lyric_path_base({self.emit_value(args[0])})"
    }
  }
  if name == "path_ext" {
    if len(args) > 0 {
      return f"lyric_path_ext({self.emit_value(args[0])})"
    }
  }

  if name == "new_string_builder" {
    return "new_string_builder()"
  }
  if name == "new_dict" {
    return "/* new_dict */"
  }
  if name == "to_string" {
    if len(args) > 0 {
      return f"lyric_to_string({self.emit_value(args[0])})"
    }
  }

  // No fallback — unhandled builtins must be added explicitly above
  eprintln(f"c_backend: unhandled builtin: {name}")
  os_exit(1)
  return ""
}

// ---------------------------------------------------------------------------
// Expression emission
// ---------------------------------------------------------------------------

func CGen.emit_expr_str(self, e: LExpr?) -> string {
  if isnull(e) {
    return "/* null expr */"
  }
  match e!.kind {
    ExCall => {
      return self.emit_call_expr(e!)
    }
    ExMethodCall => {
      return self.emit_method_call_expr(e!)
    }
    ExClassAlloc => {
      return self.emit_class_alloc_expr(e!)
    }
    ExStructLit => {
      return self.emit_struct_lit_expr(e!)
    }
    ExBinOp => {
      return self.emit_bin_op_expr(e!)
    }
    ExUnOp => {
      let d = e!.un_op!
      return f"({c_un_op(d.op)}{self.emit_value(d.operand)})"
    }
    ExCast => {
      let d = e!.cast!
      // Special case: cast to error (const char*) — use emit_value_as_cstr
      if !isnull(d.target) && d.target!.kind is TyError {
        return self.emit_value_as_cstr(d.operand)
      }
      return f"(({self.c_type(d.target)}){self.emit_value(d.operand)})"
    }
    ExBuiltin => {
      return self.emit_builtin(e!.builtin)
    }
    ExStructField => {
      return self.emit_struct_field_expr(e!)
    }
    ExClassGet => {
      let d = e!.class_get!
      if self.prog!.slab_mode_soa {
        let cname = self.resolve_class_name(d.class_name, "ExClassGet")
        return f"_lyric_slab_{cname}.{lc_first(d.field)}[{self.emit_value(d.handle)}]"
      }
      return f"{self.emit_value(d.handle)}->{d.field}"
    }
    ExIndexGet => {
      let d = e!.index_get!
      return f"{self.emit_value(d.collection)}.data[{self.emit_value(d.index)}]"
    }
    ExSlice => {
      let d = e!.slice_data!
      let coll = self.emit_value(d.collection)
      let low = if !isnull(d.low) { self.emit_value(d.low) } else { "0" }
      let high = if !isnull(d.high) { self.emit_value(d.high) } else { f"{coll}.len" }
      let slice_type = self.c_type(e!.typ)
      return f"lyric_subslice({coll}, {low}, {high}, {slice_type})"
    }
    ExWrapOptional => {
      let d = e!.wrap_opt!
      if self.is_class_optional(e!.typ) {
        return self.emit_value(d.value)
      }
      let opt_name = self.opt_type_name_from_expr(e)
      return f"lyric_some({self.emit_value(d.value)}, {opt_name})"
    }
    ExUnwrapOptional => {
      let d = e!.unwrap_opt!
      let val_type = self.resolve_value_type(d.value)
      if !isnull(val_type) {
        let is_class = val_type!.kind is TyClassHandle
        let is_opt_class = self.is_class_optional(val_type)
        if is_class || is_opt_class {
          return self.emit_value(d.value)
        }
      }
      return f"lyric_unwrap({self.emit_value(d.value)})"
    }
    ExIsNull => {
      let d = e!.is_null!
      let val_type = self.resolve_value_type(d.value)
      if !isnull(val_type) {
        let is_class = val_type!.kind is TyClassHandle
        let is_any = val_type!.kind is TyAny
        let is_opt_class = (
          val_type!.kind is TyOptional
          && !isnull(val_type!.elem) && val_type!.elem!.kind is TyClassHandle
        )
        let is_error = val_type!.kind is TyError
        if is_class || is_any || is_opt_class || is_error {
          if self.prog!.slab_mode_soa && (is_class || is_opt_class) {
            return f"({self.emit_value(d.value)} == 0)"
          }
          return f"({self.emit_value(d.value)} == NULL)"
        }
      }
      return f"lyric_isnull({self.emit_value(d.value)})"
    }
    ExVariantConstruct => {
      return self.emit_variant_construct_expr(e!)
    }
    ExVariantTag => {
      let d = e!.variant_tag!
      let val = self.emit_value(d.value)
      let vt = self.resolve_value_type(d.value)
      if !isnull(vt) && vt!.kind is TyTaggedUnion {
        let sn = vt!.name
        let entry = self.simple_enums!.get(sym(sn))
        if !isnull(entry) {
          return val
        }
      }
      return f"{val}.tag"
    }
    ExVariantData => {
      let d = e!.variant_data!
      let variant_lower = c_safe_name(str_to_lower(d.variant))
      if d.field != "" {
        return f"{self.emit_value(d.value)}.data.{variant_lower}.{d.field}"
      }
      return f"{self.emit_value(d.value)}.data.{variant_lower}"
    }
    ExFuncLit => {
      return self.emit_func_lit_expr(e!)
    }
    ExFormat => {
      return self.emit_format(e!.format)
    }
    ExExtractValue => {
      let d = e!.extract_value!
      return f"{self.emit_value(d.value)}.value"
    }
    ExExtractError => {
      let d = e!.extract_error!
      return f"{self.emit_value(d.value)}.error"
    }
    ExMakeResult => {
      let d = e!.make_result!
      let result_name = self.result_type_name_from_expr(e)
      if d.err!.kind is ValLitNull || (d.err!.kind is ValLitString && d.err!.str_val == "") {
        // Check if ErrorResult wraps Optional — wrap value in lyric_some
        let ret_elem = if !isnull(e!.typ) { e!.typ!.elem } else { null }
        let val_type = if !isnull(d.value) { d.value!.typ } else { null }
        if !isnull(ret_elem) && ret_elem!.kind is TyOptional && !self.is_class_optional(ret_elem) && (isnull(val_type) || !(val_type!.kind is TyOptional)) && !(d.value!.kind is ValLitNull) {
          let opt_name = self.opt_type_name(ret_elem!.elem)
          return f"lyric_ok(lyric_some({self.emit_value(d.value)}, {opt_name}), {result_name})"
        }
        return f"lyric_ok({self.emit_value(d.value)}, {result_name})"
      }
      return f"lyric_err({self.emit_value_as_cstr(d.err)}, {result_name})"
    }
    ExMakeSlice => {
      let slice_type = self.slice_type_name_from_type(e!.typ)
      let d = e!.builtin!
      if len(d.args) == 0 {
        return f"lyric_slice_empty({slice_type})"
      }
      // Non-empty slice literal
      let elem_type = if !isnull(e!.typ) && !isnull(e!.typ!.elem) { self.c_type(e!.typ!.elem) } else { "void*" }
      let sb = new_string_builder()
      sb.write(f"lyric_slice_lit({slice_type}, {elem_type}, ")
      let mut i = 0
      while i < len(d.args) {
        if i > 0 { sb.write(", ") }
        sb.write(self.emit_value(d.args[i]))
        i = i + 1
      }
      sb.write(")")
      return sb.to_string()
    }
    ExMakeMap => {
      return "NULL /* make_map: not supported in C backend */"
    }
    ExMakeChannel => {
      let d = e!.make_channel!
      let suffix = self.chan_suffix(d.elem_type)
      let buf_size = if !isnull(d.buf_size) { self.emit_value(d.buf_size) } else { "0" }
      return f"lyric_chan_make_{suffix}({buf_size})"
    }
    ExFuncRef => {
      let d = e!.func_ref!
      return d.name
    }
    ExEnvGet => {
      let d = e!.env_get!
      return f"{self.emit_value(d.env)}.{d.field}"
    }
    ExSlabGet => {
      let d = e!.slab_get!
      let ref = self.emit_value(d.handle)
      if self.prog!.slab_mode_soa {
        let cname = self.resolve_class_name(d.class_name, "ExSlabGet")
        return f"_lyric_slab_{cname}.{lc_first(d.field)}[{ref}]"
      }
      return f"{ref}->{d.field}"
    }
    ExSlabAlloc => {
      let d = e!.slab_alloc!
      let cname = self.resolve_class_name(d.class_name, "ExSlabAlloc")
      if len(d.fields) == 0 {
        return f"_lyric_slab_alloc_{cname}()"
      }
      // Has inline field inits (from slab_rewrite_expr path): emit compound stmt expr
      let mut class_fields: [LField] = []
      let c_entry = self.class_by_name!.get(sym(cname))
      if !isnull(c_entry) {
        class_fields = c_entry!.value.fields
      }
      let sb = new_string_builder()
      if self.prog!.slab_mode_soa {
        sb.write(f"({{ uint32_t _p = _lyric_slab_alloc_{cname}(); ")
      } else {
        sb.write(f"({{ {cname}* _p = _lyric_slab_alloc_{cname}(); ")
      }
      let mut j = 0
      while j < len(d.fields) {
        let f = d.fields[j]
        let mut val = self.emit_value(f.value)
        let field_type = self.find_class_field(class_fields, f.name)
        if !isnull(field_type) && field_type!.kind is TyOptional {
          if !isnull(field_type!.elem) && field_type!.elem!.kind is TyClassHandle {
            if !isnull(f.value) && f.value!.kind is ValLitNull {
              if self.prog!.slab_mode_soa { val = "0" } else { val = "NULL" }
            }
          } else if !isnull(f.value) && f.value!.kind is ValLitNull {
            let opt_n = self.opt_type_name(field_type!.elem)
            val = f"lyric_none({opt_n})"
          } else if isnull(f.value) || isnull(f.value!.typ) || !(f.value!.typ!.kind is TyOptional) {
            let opt_n = self.opt_type_name(field_type!.elem)
            val = f"lyric_some({val}, {opt_n})"
          }
        }
        if self.prog!.slab_mode_soa {
          sb.write(f"_lyric_slab_{cname}.{lc_first(f.name)}[_p] = {val}; ")
        } else {
          sb.write(f"_p->{lc_first(f.name)} = {val}; ")
        }
        j = j + 1
      }
      sb.write("_p; })")
      return sb.to_string()
    }
    _ => {
      return "/* unknown expr */"
    }
  }
}

func CGen.emit_call_expr(self, e: LExpr?) -> string {
  let d = e!.call!
  let name = c_safe_name(d.func_name)
  // Map known stdlib functions
  if name == "println" || name == "fmt.Println" {
    return self.emit_println(d.args)
  }
  if name == "fmt.Printf" {
    return self.emit_printf(d.args)
  }
  if name == "fmt.Sprintf" {
    return self.emit_sprintf(d.args)
  }
  if name == "fmt.Errorf" || name == "errors.New" {
    if len(d.args) > 0 {
      return self.emit_value_as_cstr(d.args[0])
    }
  }
  if name == "fmt.Print" {
    return self.emit_fprint("stdout", d.args, false)
  }
  if name == "panic" && len(d.args) > 0 {
    return f"lyric_panic({self.emit_value(d.args[0])})"
  }
  // Check if calling a generator function
  if self.is_gen_func_by_name(name) {
    let args_str = self.emit_args(d.args)
    return f"{name}_init({args_str})"
  }
  // Known C library mappings
  if name == "atoi" {
    return f"lyric_atoi({self.emit_args(d.args)})"
  }
  if name == "parse_float" {
    return f"lyric_parse_float({self.emit_args(d.args)})"
  }
  if name == "itoa" || name == "strconv.Itoa" {
    return f"lyric_itoa({self.emit_args(d.args)})"
  }
  if name == "strings.ToUpper" {
    if len(d.args) > 0 {
      return f"lyric_toupper({self.emit_value(d.args[0])})"
    }
  }
  if name == "strings.ToLower" {
    if len(d.args) > 0 {
      return f"lyric_tolower({self.emit_value(d.args[0])})"
    }
  }
  if name == "strings.Contains" {
    if len(d.args) >= 2 {
      return f"(strstr({self.emit_value(d.args[0])}, {self.emit_value(d.args[1])}) != NULL)"
    }
  }
  if name == "char_to_string" {
    return f"lyric_char_to_string({self.emit_args(d.args)})"
  }
  if name == "hash_string" {
    if len(d.args) > 0 {
      return f"lyric_hash_string({self.emit_value(d.args[0])})"
    }
    return "0"
  }
  if name == "new_string_builder" {
    return "lyric_new_string_builder()"
  }
  let args_str = self.emit_args_boxed_mut(d.func_name, d.args, d.mut_args)
  return f"{name}({args_str})"
}

func CGen.emit_method_call_expr(self, e: LExpr?) -> string {
  let d = e!.method_call!
  let recv = self.emit_value(d.receiver)
  let args_str = self.emit_args_with_mut(d.args, d.mut_args)

  // Check for interface vtable dispatch
  let recv_type = self.resolve_value_type(d.receiver)
  if !isnull(recv_type) && recv_type!.kind is TyAny && recv_type!.name != "" {
    let iface_entry = self.iface_by_name!.get(sym(recv_type!.name))
    if !isnull(iface_entry) {
      if args_str != "" {
        return f"{recv}._vtable->{d.method}({recv}._data, {args_str})"
      }
      return f"{recv}._vtable->{d.method}({recv}._data)"
    }
  }

  // Resolve class name
  let mut class_name = ""
  if !isnull(d.receiver) && !isnull(d.receiver!.typ) {
    class_name = d.receiver!.typ!.name
    if d.receiver!.typ!.kind is TyOptional && !isnull(d.receiver!.typ!.elem) && d.receiver!.typ!.elem!.kind is TyClassHandle {
      class_name = d.receiver!.typ!.elem!.name
    }
  }
  if !isnull(self.prog!.class_renames) {
    let ren = self.prog!.class_renames!.get(sym(class_name))
    if !isnull(ren) {
      class_name = ren!.value
    }
  }
  if class_name == "" {
    let resolved = self.resolve_value_type(d.receiver)
    if !isnull(resolved) {
      class_name = resolved!.name
      if resolved!.kind is TyOptional && !isnull(resolved!.elem) {
        class_name = resolved!.elem!.name
      }
    }
  }
  if class_name == "" {
    // Search for matching method via reverse index
    let recv_entry = self.method_to_receiver!.get(sym(d.method))
    if !isnull(recv_entry) {
      class_name = recv_entry!.value
    }
  }
  let method_name = f"{class_name}_{d.method}"
  if args_str != "" {
    return f"{method_name}({recv}, {args_str})"
  }
  return f"{method_name}({recv})"
}

func CGen.emit_class_alloc_expr(self, e: LExpr?) -> string {
  let d = e!.class_alloc!
  let name = d.class_name
  // Look up class fields for optional wrapping
  let mut class_fields: [LField] = []
  let c_entry = self.class_by_name!.get(sym(name))
  if !isnull(c_entry) {
    class_fields = c_entry!.value.fields
  }
  let sb = new_string_builder()
  sb.write(f"({{ {name}* _p = malloc(sizeof({name})); *_p = ({name}){{")
  let mut j = 0
  while j < len(d.fields) {
    if j > 0 {
      sb.write(", ")
    }
    let f = d.fields[j]
    let mut val = self.emit_value(f.value)
    // Auto-wrap for optional fields
    let field_type = self.find_class_field(class_fields, f.name)
    if !isnull(field_type) && field_type!.kind is TyOptional {
      if !isnull(field_type!.elem) && field_type!.elem!.kind is TyClassHandle {
        if !isnull(f.value) && f.value!.kind is ValLitNull {
          val = "NULL"
        }
      } else if !isnull(f.value) && f.value!.kind is ValLitNull {
        let opt_n = self.opt_type_name(field_type!.elem)
        val = f"lyric_none({opt_n})"
      } else if isnull(f.value) || isnull(f.value!.typ) || !(f.value!.typ!.kind is TyOptional) {
        let opt_n = self.opt_type_name(field_type!.elem)
        val = f"lyric_some({val}, {opt_n})"
      }
    }
    sb.write(f".{lc_first(f.name)} = {val}")
    j = j + 1
  }
  sb.write("}; _p; })")
  return sb.to_string()
}

func CGen.emit_struct_lit_expr(self, e: LExpr?) -> string {
  let d = e!.struct_lit!
  // Check if this is a slice literal
  if !isnull(e!.typ) && e!.typ!.kind is TySlice {
    let slice_type = self.slice_type_name(e!.typ!.elem)
    let elem_type = self.c_type(e!.typ!.elem)
    if len(d.fields) == 0 {
      return f"lyric_slice_empty({slice_type})"
    }
    let sb = new_string_builder()
    sb.write(f"lyric_slice_lit({slice_type}, {elem_type}, ")
    let mut i = 0
    while i < len(d.fields) {
      if i > 0 {
        sb.write(", ")
      }
      sb.write(self.emit_value(d.fields[i].value))
      i = i + 1
    }
    sb.write(")")
    return sb.to_string()
  }
  let sb = new_string_builder()
  let is_class = !isnull(e!.typ) && e!.typ!.kind is TyClassHandle
  if is_class {
    let name = e!.typ!.name
    sb.write(f"({{ {name}* _p = malloc(sizeof({name})); *_p = ({name}){{")
  } else if !isnull(e!.typ) && e!.typ!.kind is TyMutex {
    return "(pthread_mutex_t)PTHREAD_MUTEX_INITIALIZER"
  } else {
    let type_name = if !isnull(e!.typ) { self.c_type(e!.typ) } else { "/* struct */" }
    sb.write(f"({type_name}){{")
  }
  let mut i = 0
  while i < len(d.fields) {
    if i > 0 {
      sb.write(", ")
    }
    let fname = if is_class { lc_first(d.fields[i].name) } else { d.fields[i].name }
    if fname == "" {
      // Positional field — no designator
      sb.write(self.emit_value(d.fields[i].value))
    } else {
      sb.write(f".{fname} = {self.emit_value(d.fields[i].value)}")
    }
    i = i + 1
  }
  if is_class {
    sb.write("}; _p; })")
  } else {
    sb.write("}")
  }
  return sb.to_string()
}

func CGen.emit_bin_op_expr(self, e: LExpr?) -> string {
  let d = e!.bin_op!
  let left = self.emit_value(d.left)
  let right = self.emit_value(d.right)
  // String equality/concat
  if !isnull(d.left) && !isnull(d.left!.typ) && d.left!.typ!.kind is TyString {
    if d.op is BinEq {
      return f"lyric_str_eq({left}, {right})"
    }
    if d.op is BinNe {
      return f"(!lyric_str_eq({left}, {right}))"
    }
    if d.op is BinAdd {
      return f"lyric_str_concat({left}, {right})"
    }
  }
  // Slice concatenation
  if !isnull(d.left) && !isnull(d.left!.typ) && d.left!.typ!.kind is TySlice {
    if d.op is BinAdd {
      let slice_type = self.c_type(d.left!.typ)
      return f"lyric_slice_concat({left}, {right}, {slice_type})"
    }
  }
  // Optional null comparison
  if (d.op is BinEq || d.op is BinNe) && (!isnull(d.right) && d.right!.kind is ValLitNull || !isnull(d.left) && d.left!.kind is ValLitNull) {
    let mut opt_val = left
    let mut opt_type: LType? = null
    if !isnull(d.right) && d.right!.kind is ValLitNull {
      opt_type = self.resolve_value_type(d.left)
    } else {
      opt_val = right
      opt_type = self.resolve_value_type(d.right)
    }
    if !isnull(opt_type) && opt_type!.kind is TyOptional && !self.is_class_optional(opt_type) {
      if d.op is BinNe {
        return f"{opt_val}.has"
      }
      return f"(!{opt_val}.has)"
    }
  }
  return f"({left} {c_bin_op(d.op)} {right})"
}

func CGen.emit_struct_field_expr(self, e: LExpr?) -> string {
  let d = e!.struct_field!
  let recv_type = self.resolve_value_type(d.receiver)
  // Unwrap optional to get the inner type for class handle check
  let mut inner_type = recv_type
  if !isnull(inner_type) && inner_type!.kind is TyOptional && !isnull(inner_type!.elem) {
    inner_type = inner_type!.elem
  }
  if !isnull(inner_type) && inner_type!.kind is TyClassHandle {
    let recv = self.emit_value(d.receiver)
    if self.prog!.slab_mode_soa {
      let cname = inner_type!.name
      return f"_lyric_slab_{cname}.{lc_first(d.field)}[{recv}]"
    }
    return f"{recv}->{d.field}"
  }
  if !isnull(recv_type) && recv_type!.kind is TyErrorResult {
    let field = d.field
    if field == "_0" {
      return f"{self.emit_value(d.receiver)}.value"
    }
    if field == "_1" {
      return f"{self.emit_value(d.receiver)}.error"
    }
  }
  return f"{self.emit_value(d.receiver)}.{d.field}"
}

func CGen.emit_variant_construct_expr(self, e: LExpr?) -> string {
  let d = e!.variant_construct!
  let mut enum_name = d.enum_name
  if enum_name == "" {
    enum_name = "Enum"
  }
  // Simple enums
  let entry = self.simple_enums!.get(sym(enum_name))
  if !isnull(entry) {
    return f"{enum_name}_{d.variant}"
  }
  if len(d.fields) == 0 {
    return f"({enum_name}){{.tag = {enum_name}_{d.variant}}}"
  }
  let sb = new_string_builder()
  let variant_lower = c_safe_name(str_to_lower(d.variant))
  sb.write(f"({enum_name}){{.tag = {enum_name}_{d.variant}, .data.{variant_lower} = {{")
  let mut i = 0
  while i < len(d.fields) {
    if i > 0 {
      sb.write(", ")
    }
    sb.write(self.emit_value(d.fields[i]))
    i = i + 1
  }
  sb.write("}}")
  return sb.to_string()
}

func CGen.emit_func_lit_expr(self, e: LExpr?) -> string {
  let d = e!.func_lit!
  let sb = new_string_builder()
  let mut i = 0
  while i < len(d.params) {
    if i > 0 {
      sb.write(", ")
    }
    sb.write(self.c_field_decl(d.params[i].typ, d.params[i].name))
    i = i + 1
  }
  let mut ret_type = "void"
  if !isnull(e!.typ) && e!.typ!.kind is TyFuncPtr && !isnull(e!.typ!.ret) {
    ret_type = self.c_type(e!.typ!.ret)
  } else if !isnull(d.return_type) {
    ret_type = self.c_type(d.return_type)
  }
  self.lambda_id = self.lambda_id + 1
  let lambda_name = f"_lambda_{self.lambda_id}"
  let param_str = if sb.len() > 0 { sb.to_string() } else { "void" }

  // Emit body into temp buffer
  let saved_buf = self.buf
  self.buf = new_string_builder()
  self.indent = self.indent + 1
  self.emit_stmts(d.body)
  self.indent = self.indent - 1
  let body_str = self.buf!.to_string()
  self.buf = saved_buf

  append(self.lambdas, CLambda {
    name: lambda_name,
    ret_type: ret_type,
    params: param_str,
    body_str: body_str,
  })
  return lambda_name
}

func CGen.is_gen_func_by_name(self, name: string) -> bool {
  let entry = self.gen_funcs!.get(sym(name))
  return !isnull(entry)
}

func CGen.emit_args_boxed(self, func_name: string, args: [LValue?]) -> string {
  let fn_entry = self.func_by_name!.get(sym(func_name))
  let sb = new_string_builder()
  let mut i = 0
  while i < len(args) {
    if i > 0 {
      sb.write(", ")
    }
    let mut arg_str = self.emit_value(args[i])
    // Check if arg needs boxing (concrete → interface)
    if !isnull(fn_entry) && i < len(fn_entry!.value.params) {
      let pt = fn_entry!.value.params[i].typ
      if !isnull(pt) && pt!.kind is TyUnion {
        // Union param: wrap concrete arg in lyric_union_*()
        let arg_type = self.resolve_value_type(args[i])
        if !isnull(arg_type) && !(arg_type!.kind is TyUnion) {
          arg_str = self.c_wrap_union(arg_str, arg_type)
        }
      } else if !isnull(pt) && pt!.kind is TyAny && pt!.name != "" {
        let iface_e = self.iface_by_name!.get(sym(pt!.name))
        if !isnull(iface_e) {
          let concrete_class = self.resolve_concrete_class(args[i])
          if concrete_class != "" {
            let data_val = if self.prog!.slab_mode_soa { f"(void*)(uintptr_t){arg_str}" } else { arg_str }
            arg_str = f"({pt!.name}){{._data = {data_val}, ._vtable = &{concrete_class}_as_{pt!.name}}}"
          }

        }
      }
    }
    sb.write(arg_str)
    i = i + 1
  }
  return sb.to_string()
}

func CGen.emit_args_boxed_mut(self, func_name: string, args: [LValue?], mut_args: [bool]) -> string {
  let fn_entry = self.func_by_name!.get(sym(func_name))
  let sb = new_string_builder()
  let mut i = 0
  while i < len(args) {
    if i > 0 {
      sb.write(", ")
    }
    let mut arg_str = self.emit_value(args[i])
    // Check if arg is mut — prefix with &
    if len(mut_args) > i && mut_args[i] {
      arg_str = "&" + arg_str
    } else if !isnull(fn_entry) && i < len(fn_entry!.value.params) {
      // Check if arg needs boxing (concrete → interface or concrete → union)
      let pt = fn_entry!.value.params[i].typ
      if !isnull(pt) && pt!.kind is TyUnion {
        let arg_type = self.resolve_value_type(args[i])
        if !isnull(arg_type) && !(arg_type!.kind is TyUnion) {
          arg_str = self.c_wrap_union(arg_str, arg_type)
        }
      } else if !isnull(pt) && pt!.kind is TyAny && pt!.name != "" {
        let iface_e = self.iface_by_name!.get(sym(pt!.name))
        if !isnull(iface_e) {
          let concrete_class = self.resolve_concrete_class(args[i])
          if concrete_class != "" {
            let data_val = if self.prog!.slab_mode_soa { f"(void*)(uintptr_t){arg_str}" } else { arg_str }
            arg_str = f"({pt!.name}){{._data = {data_val}, ._vtable = &{concrete_class}_as_{pt!.name}}}"
          }
        }

      }
    }
    sb.write(arg_str)
    i = i + 1
  }
  return sb.to_string()
}

func CGen.resolve_concrete_class(self, v: LValue?) -> string {
  let t = self.resolve_value_type(v)
  if isnull(t) {
    return ""
  }
  if t!.kind is TyClassHandle {
    return t!.name
  }
  if t!.kind is TyOptional && !isnull(t!.elem) && t!.elem!.kind is TyClassHandle {
    return t!.elem!.name
  }
  return ""
}

// ---------------------------------------------------------------------------
// Side effect emission
// ---------------------------------------------------------------------------

func CGen.emit_side_effect(self, e: LExpr?) -> string {
  if isnull(e) {
    return ""
  }
  if e!.kind is ExBuiltin {
    let d = e!.builtin!
    if (d.name == "append" || d.name == "slice_push") && len(d.args) >= 2 {
      let slice_arg = self.emit_value(d.args[0])
      let elem_arg = self.emit_value(d.args[1])
      let slice_type = self.slice_type_name_from_value(d.args[0])
      self.line(f"lyric_push(&{slice_arg}, {elem_arg}, {slice_type});")
      return ""
    }
    if d.name == "push_bytes" && len(d.args) >= 2 {
      let dst_arg = self.emit_value(d.args[0])
      let src_arg = self.emit_value(d.args[1])
      self.line(f"lyric_push_bytes(&{dst_arg}, {src_arg});")
      return ""
    }
    if d.name == "slice_extend" && len(d.args) >= 2 {
      let dst_arg = self.emit_value(d.args[0])
      let src_arg = self.emit_value(d.args[1])
      let slice_type = self.slice_type_name_from_value(d.args[0])
      self.line(f"lyric_extend(&{dst_arg}, {src_arg}, {slice_type});")
      return ""
    }
  }
  return self.emit_expr_str(e)
}

// ---------------------------------------------------------------------------
// Multi-assign emission
// ---------------------------------------------------------------------------

func CGen.emit_multi_assign(self, names: [string], types: [LType?], expr: LExpr?) {
  let expr_str = self.emit_expr_str(expr)
  let mut is_error_result = !isnull(expr) && !isnull(expr!.typ) && expr!.typ!.kind is TyErrorResult
  if !is_error_result && len(types) == 2 && !isnull(types[1]) {
    let is_err = types[1]!.kind is TyError || types[1]!.kind is TyString
    if is_err {
      is_error_result = true
    }
  }
  // Also check if the call is to a function that returns ErrorResult
  if !is_error_result && !isnull(expr) && expr!.kind is ExCall && !isnull(expr!.call) {
    let call_name = expr!.call!.func_name
    let fn_entry = self.func_by_name!.get(sym(call_name))
    if !isnull(fn_entry) && !isnull(fn_entry!.value.return_type) && fn_entry!.value.return_type!.kind is TyErrorResult {
      is_error_result = true
      if !isnull(fn_entry!.value.return_type!.elem) {
        expr!.typ = fn_entry!.value.return_type
      }
    }
  }
  if len(names) == 2 && is_error_result {
    let mut elem_type: LType? = null
    if len(types) > 0 {
      elem_type = types[0]
    }
    if !isnull(expr) && !isnull(expr!.typ) && !isnull(expr!.typ!.elem) {
      elem_type = expr!.typ!.elem
    }
    if isnull(elem_type) {
      elem_type = LType { kind: TyI32, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 0, is_exported: false }
    }
    let result_type = self.result_type_name(elem_type)
    let tmp_name = f"_multi_{self.next_temp()}"
    self.line(f"{result_type} {tmp_name} = {expr_str};")
    if names[0] != "_" {
      let val_type = self.c_type(elem_type)
      self.line(f"{val_type} {names[0]} = {tmp_name}.value;")
      self.var_types!.set(sym(names[0]), elem_type)
    }
    if names[1] != "_" {
      self.line(f"const char* {names[1]} = {tmp_name}.error;")
      self.var_types!.set(sym(names[1]), LType { kind: TyError, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 0, is_exported: false })
    }
  } else if len(names) == 2 && !isnull(expr) && !isnull(expr!.typ) && expr!.typ!.kind is TyTuple {
    let tmp_name = f"_multi_{self.next_temp()}"
    let tuple_type = self.c_tuple_type(expr!.typ)
    self.line(f"{tuple_type} {tmp_name} = {expr_str};")
    let mut i = 0
    while i < len(names) {
      if names[i] != "_" {
        let mut field_type_str = "int"
        let mut ltyp: LType? = null
        if i < len(expr!.typ!.fields) {
          ltyp = expr!.typ!.fields[i].typ
          field_type_str = self.c_type(ltyp)
        }
        self.line(f"{field_type_str} {names[i]} = {tmp_name}._{i};")
        if !isnull(ltyp) {
          self.var_types!.set(sym(names[i]), ltyp)
        }
      }
      i = i + 1
    }
  } else {
    let sb = new_string_builder()
    let mut i = 0
    while i < len(names) {
      if i > 0 {
        sb.write(", ")
      }
      sb.write(names[i])
      i = i + 1
    }
    self.line(f"/* multi-assign: {sb.to_string()} = {expr_str} */")
  }
}

// ---------------------------------------------------------------------------
// Statement emission
// ---------------------------------------------------------------------------

func CGen.emit_stmts(self, stmts: [LStmt?]) {
  let mut i = 0
  while i < len(stmts) {
    self.emit_stmt(stmts[i])
    i = i + 1
  }
}

func CGen.emit_stmt(self, s: LStmt?) {
  if isnull(s) {
    return
  }
  match s!.kind {
    StTempDef => {
      let d = s!.temp_def!
      let ty = self.infer_expr_type(d.expr)
      self.temp_types!.set(sym(f"{d.id}"), ty)
      if !isnull(ty) && ty!.kind is TyUnit {
        self.line(f"{self.emit_expr_str(d.expr)};")
        return
      }
      let temp_name = f"_t{d.id}"
      self.line(f"{self.c_field_decl(ty, temp_name)} = {self.emit_expr_str(d.expr)};")
    }
    StVarDecl => {
      let d = s!.var_decl!
      if d.name == "_" {
        if !isnull(d.init) {
          self.line(f"(void){self.emit_value(d.init)};")
        }
        return
      }
      let name = c_safe_name(d.name)
      if self.in_generator {
        self.var_types!.set(sym(d.name), d.typ)
        if !isnull(d.init) {
          self.line(f"_gen->{name} = {self.emit_value(d.init)};")
        }
        return
      }
      let mut var_type = d.typ
      // Resolve type from init if type is any/typevar
      if !isnull(d.init) {
        let should_resolve = (
          isnull(var_type) || self.contains_type_var(var_type) || var_type!.kind is TyAny
        )
        if should_resolve {
          let resolved = self.resolve_value_type(d.init)
          if !isnull(resolved) {
            var_type = resolved
          }
        }
      }
      self.var_types!.set(sym(d.name), var_type)
      if !isnull(d.init) {
        let mut init_str = self.emit_value(d.init)
        // Wrap in union constructor if target is union and source isn't
        if !isnull(var_type) && var_type!.kind is TyUnion {
          let src_type = self.infer_lval_type(d.init)
          if !isnull(src_type) && !(src_type!.kind is TyUnion) {
            init_str = self.c_wrap_union(init_str, src_type)
          }
        }
        // Wrap in optional for bare literal values
        if !isnull(var_type) && var_type!.kind is TyOptional {
          let is_lit = (
            d.init!.kind is ValLitInt || d.init!.kind is ValLitUint
            || d.init!.kind is ValLitFloat || d.init!.kind is ValLitString
            || d.init!.kind is ValLitBool
          )
          if is_lit {
            init_str = f"lyric_some({init_str}, {self.c_type(var_type)})"
          }
        }
        self.line(f"{self.c_field_decl(var_type, name)} = {init_str};")
      } else {
        self.line(f"{self.c_field_decl(var_type, name)} = {self.zero_value(var_type)};")
      }
    }
    StAssign => {
      let d = s!.assign!
      let mut target = c_safe_name(d.target)
      if self.in_generator {
        target = f"_gen->{target}"
      } else if !isnull(self.spawn_captures) {
        let entry = self.spawn_captures!.get(sym(d.target))
        if !isnull(entry) {
          target = f"(*_ctx->{target})"
        }
      } else if !isnull(self.mut_params) {
        let mp = self.mut_params!.get(sym(d.target))
        if !isnull(mp) {
          target = f"(*{target})"
        }
      }
      self.line(f"{target} = {self.emit_value(d.value)};")
    }
    StStructSet => {
      let d = s!.struct_set!
      self.line(f"{self.emit_value(d.receiver)}.{d.field} = {self.emit_value(d.value)};")
    }
    StClassSet => {
      let d = s!.class_set!
      let ref = self.emit_value(d.handle)
      let val = self.emit_value(d.value)
      // Auto-wrap non-optional value when field type is optional struct
      let mut wrapped_val = val
      let classes = self.prog!.classes
      let mut ci = 0
      while ci < len(classes) {
        if classes[ci].name == d.class_name {
          let field_type = self.find_class_field(classes[ci].fields, d.field)
          if !isnull(field_type) && field_type!.kind is TyOptional && !self.is_class_optional(field_type) {
            let val_type = self.resolve_value_type(d.value)
            if isnull(val_type) || !(val_type!.kind is TyOptional) {
              if !isnull(d.value) && !(d.value!.kind is ValLitNull) {
                let opt_name = self.opt_type_name(field_type!.elem)
                wrapped_val = f"lyric_some({val}, {opt_name})"
              }
            }
          }
          break
        }
        ci = ci + 1
      }
      if self.prog!.slab_mode_soa {
        let cname = self.resolve_class_name(d.class_name, "StClassSet")
        self.line(f"_lyric_slab_{cname}.{lc_first(d.field)}[{ref}] = {wrapped_val};")
      } else {
        self.line(f"{ref}->{lc_first(d.field)} = {wrapped_val};")
      }
    }
    StSlabSet => {
      let d = s!.slab_set!
      let ref = self.emit_value(d.handle)
      let val = self.emit_value(d.value)
      // Auto-wrap non-optional value when field type is optional struct
      let mut wrapped_val = val
      let classes = self.prog!.classes
      let mut ci = 0
      while ci < len(classes) {
        if classes[ci].name == d.class_name {
          let field_type = self.find_class_field(classes[ci].fields, d.field)
          if !isnull(field_type) && field_type!.kind is TyOptional && !self.is_class_optional(field_type) {
            let val_type = self.resolve_value_type(d.value)
            if isnull(val_type) || !(val_type!.kind is TyOptional) {
              if !isnull(d.value) && !(d.value!.kind is ValLitNull) {
                let opt_name = self.opt_type_name(field_type!.elem)
                wrapped_val = f"lyric_some({val}, {opt_name})"
              }
            }
          }
          break
        }
        ci = ci + 1
      }
      if self.prog!.slab_mode_soa {
        let cname = self.resolve_class_name(d.class_name, "StSlabSet")
        self.line(f"_lyric_slab_{cname}.{lc_first(d.field)}[{ref}] = {wrapped_val};")
      } else {
        self.line(f"{ref}->{lc_first(d.field)} = {wrapped_val};")
      }
    }
    StSlabFree => {
      let d = s!.slab_free!
      let ref = self.emit_value(d.handle)
      let cname = self.resolve_class_name(d.class_name, "field_to_string")
      self.line(f"_lyric_slab_free_{cname}({ref});")
    }
    StSliceFree => {
      let d = s!.slice_free!
      self.line(f"if ({d.name}.cap > 0 && {d.name}.data) free({d.name}.data);")
    }
    StSliceRetain => {
      // TODO: slice RC retain — for now no-op (slices still use scope-exit free)
    }
    StRefIncr => {
      let d = s!.ref_incr!
      let ref = self.emit_value(d.handle)
      let cname = self.resolve_class_name(d.class_name, "ref_incr")
      self.line(f"/* rc_incr({cname}, {ref}) */")
    }
    StRefDecr => {
      let d = s!.ref_decr!
      let ref = self.emit_value(d.handle)
      let cname = self.resolve_class_name(d.class_name, "ref_decr")
      self.line(f"/* rc_free({cname}, {ref}) */")
    }
    StIndexSet => {
      let d = s!.index_set!
      if d.field != "" {
        // slice[i].field = val → direct lvalue chain
        self.line(f"{self.emit_value(d.collection)}.data[{self.emit_value(d.index)}].{d.field} = {self.emit_value(d.value)};")
      } else {
        self.line(f"{self.emit_value(d.collection)}.data[{self.emit_value(d.index)}] = {self.emit_value(d.value)};")
      }
    }
    StReturn => {
      self.emit_return_stmt(s!)
    }
    StIf => {
      let d = s!.if_data!
      self.line(f"if ({self.emit_value(d.cond)}) {{")
      self.indent = self.indent + 1
      self.emit_stmts(d.then_body)
      self.indent = self.indent - 1
      if len(d.else_body) > 0 {
        self.line(f"}} else {{")
        self.indent = self.indent + 1
        self.emit_stmts(d.else_body)
        self.indent = self.indent - 1
      }
      self.line("}")
    }
    StWhile => {
      let d = s!.while_data!
      self.line(f"while (1) {{")
      self.indent = self.indent + 1
      self.emit_stmts(d.cond_block)
      self.line(f"if (!({self.emit_value(d.cond_var)})) break;")
      self.emit_stmts(d.body)
      self.indent = self.indent - 1
      self.line("}")
    }
    StFor => {
      self.emit_for_stmt(s!)
    }
    StSwitch => {
      self.emit_switch_stmt(s!)
    }
    StTypeSwitch => {
      self.emit_type_switch_stmt(s!)
    }
    StBlock => {
      let d = s!.block!
      self.line(f"{{")
      self.indent = self.indent + 1
      self.emit_stmts(d.stmts)
      self.indent = self.indent - 1
      self.line("}")
    }
    StSideEffect => {
      let d = s!.side_effect!
      let expr = self.emit_side_effect(d.expr)
      if expr != "" {
        self.line(f"{expr};")
      }
    }
    StMultiAssign => {
      let d = s!.multi_assign!
      self.emit_multi_assign(d.names, d.types, d.expr)
    }
    StSend => {
      let d = s!.send_data!
      let chan_type = self.resolve_value_type(d.channel)
      let mut suffix = "void"
      if !isnull(chan_type) && chan_type!.kind is TyChannel && !isnull(chan_type!.elem) {
        suffix = self.chan_suffix(chan_type!.elem)
      }
      self.line(f"lyric_chan_send_{suffix}({self.emit_value(d.channel)}, {self.emit_value(d.value)});")
    }
    StSpawn => {
      let d = s!.spawn_data!
      self.spawn_id = self.spawn_id + 1
      let func_name = f"_spawn_{self.spawn_id}"

      // Auto-detect captured variables: used in body but not declared within it
      let used = collect_used_vars(d.body)
      let declared = collect_declared_vars(d.body)
      let mut captures: [CCapture] = []
      let used_keys = used!.keys()
      let mut ci = 0
      while ci < len(used_keys) {
        let var_name = used_keys[ci].get_name()
        let decl_entry = declared!.get(used_keys[ci])
        if isnull(decl_entry) {
          let mut ctyp = "void*"
          let vt_entry = self.var_types!.get(used_keys[ci])
          if !isnull(vt_entry) && !isnull(vt_entry!.value) {
            ctyp = self.c_type(vt_entry!.value)
          }
          append(captures, CCapture { name: var_name, typ: ctyp })
        }
        ci = ci + 1
      }

      // Emit the body into a separate buffer with capture awareness
      let saved_buf = self.buf
      let saved_indent = self.indent
      let saved_captures = self.spawn_captures
      self.buf = new_string_builder()
      self.indent = 1
      let capture_set = Dict<Sym, bool>()
      let mut ci2 = 0
      while ci2 < len(captures) {
        capture_set.set(sym(captures[ci2].name), true)
        ci2 = ci2 + 1
      }
      self.spawn_captures = capture_set
      self.emit_stmts(d.body)
      let body_str = self.buf!.to_string()
      self.buf = saved_buf
      self.indent = saved_indent
      self.spawn_captures = saved_captures

      append(self.spawn_funcs, CSpawnFunc {
        name: func_name,
        body_str: body_str,
        captures: captures,
      })

      // Emit spawn call site — pass pointers to original variables
      if len(captures) > 0 {
        self.line("{")
        self.indent = self.indent + 1
        self.line(f"{func_name}_ctx* _ctx = ({func_name}_ctx*)malloc(sizeof({func_name}_ctx));")
        let mut ci3 = 0
        while ci3 < len(captures) {
          self.line(f"_ctx->{captures[ci3].name} = &{captures[ci3].name};")
          ci3 = ci3 + 1
        }
        self.line(f"lyric_spawn({func_name}, _ctx);")
        self.indent = self.indent - 1
        self.line("}")
      } else {
        self.line(f"lyric_spawn({func_name}, NULL);")
      }
    }
    StSelect => {
      let d = s!.select_data!
      let sid = self.select_id
      self.select_id = self.select_id + 1
      let label = f"_sel_done_{sid}"
      // Select: try each case in a polling loop
      self.line("for (;;) {")
      self.indent = self.indent + 1
      let mut si = 0
      while si < len(d.cases) {
        let sc = d.cases[si]
        match sc.kind {
          SelDefault => {
            self.emit_stmts(sc.body)
            self.line(f"goto {label};")
          }
          SelRecv => {
            let chan_type = self.resolve_value_type(sc.channel)
            let mut suffix = "void"
            if !isnull(chan_type) && chan_type!.kind is TyChannel && !isnull(chan_type!.elem) {
              suffix = self.chan_suffix(chan_type!.elem)
            }
            let ch_val = self.emit_value(sc.channel)
            if sc.binding != "" {
              let elem_ctype = if !isnull(chan_type) && chan_type!.kind is TyChannel && !isnull(chan_type!.elem) {
                self.c_type(chan_type!.elem)
              } else { "int32_t" }
              self.line(f"{{ {elem_ctype} _sel_val; if (lyric_chan_tryrecv_{suffix}({ch_val}, &_sel_val)) {{")
              self.indent = self.indent + 1
              self.line(f"{elem_ctype} {c_safe_name(sc.binding)} = _sel_val;")
              self.emit_stmts(sc.body)
              self.indent = self.indent - 1
              self.line(f"goto {label}; }} }}")
            } else {
              self.line(f"if (lyric_chan_tryrecv_{suffix}({ch_val}, NULL)) {{")
              self.indent = self.indent + 1
              self.emit_stmts(sc.body)
              self.indent = self.indent - 1
              self.line(f"goto {label}; }}")
            }
          }
          SelSend => {
            let chan_type = self.resolve_value_type(sc.channel)
            let mut suffix = "void"
            if !isnull(chan_type) && chan_type!.kind is TyChannel && !isnull(chan_type!.elem) {
              suffix = self.chan_suffix(chan_type!.elem)
            }
            let ch_val = self.emit_value(sc.channel)
            let send_val = self.emit_value(sc.value)
            self.line(f"if (lyric_chan_trysend_{suffix}({ch_val}, {send_val})) {{")
            self.indent = self.indent + 1
            self.emit_stmts(sc.body)
            self.indent = self.indent - 1
            self.line(f"goto {label}; }}")
          }
        }
        si = si + 1
      }
      // If no default case, sleep briefly and retry
      self.line("usleep(100);")
      self.indent = self.indent - 1
      self.line("}")
      self.line(f"{label}:;")
    }
    StDefer => {
      let d = s!.defer_data!
      self.line("/* defer (executed inline): */")
      self.emit_stmts(d.body)
    }
    StLock => {
      let d = s!.lock_data!
      let mutex_val = self.emit_value(d.mutex)
      self.line(f"pthread_mutex_lock(&{mutex_val});")
      self.emit_stmts(d.body)
      self.line(f"pthread_mutex_unlock(&{mutex_val});")
    }
    StExpr => {
      let d = s!.expr_stmt!
      let ty_entry = self.temp_types!.get(sym(f"{d.temp_id}"))
      if !isnull(ty_entry) && !isnull(ty_entry!.value) && ty_entry!.value!.kind is TyUnit {
        return
      }
      self.line(f"_t{d.temp_id};")
    }
    StBreak => {
      self.line("break;")
    }
    StContinue => {
      self.line("continue;")
    }
    StYield => {
      let d = s!.yield_data!
      self.gen_yield_count = self.gen_yield_count + 1
      let state_num = self.gen_yield_count
      self.line(f"_gen->_value = {self.emit_value(d.value)};")
      self.line(f"_gen->_state = {state_num};")
      self.line("return true;")
      self.line(f"_gen_s{state_num}:;")
    }
    _ => {}
  }
}

func CGen.emit_return_stmt(self, s: LStmt?) {
  let d = s!.ret!
  if self.in_generator {
    self.line("return false;")
    return
  }
  if len(d.values) == 0 {
    self.line("return;")
    return
  }
  if len(d.values) == 1 {
    let val = self.emit_value(d.values[0])
    // Check if function returns ErrorResult
    if !isnull(self.current_func) && !isnull(self.current_func!.return_type) && self.current_func!.return_type!.kind is TyErrorResult {
      let result_name = self.result_type_name(self.current_func!.return_type!.elem)
      let val_type = self.resolve_value_type(d.values[0])
      if !isnull(val_type) && val_type!.kind is TyErrorResult {
        self.line(f"return {val};")
        return
      }
      // If ErrorResult wraps an Optional, wrap value in lyric_some first
      let ret_elem = self.current_func!.return_type!.elem
      if !isnull(ret_elem) && ret_elem!.kind is TyOptional && !self.is_class_optional(ret_elem) && (isnull(val_type) || !(val_type!.kind is TyOptional)) && !(d.values[0]!.kind is ValLitNull) {
        let opt_name = self.opt_type_name(ret_elem!.elem)
        self.line(f"return lyric_ok(lyric_some({val}, {opt_name}), {result_name});")
      } else {
        self.line(f"return lyric_ok({val}, {result_name});")
      }
      return
    }
    // Check optional return
    if !isnull(self.current_func) && !isnull(self.current_func!.return_type) && self.current_func!.return_type!.kind is TyOptional {
      if self.is_class_optional(self.current_func!.return_type) {
        if !isnull(d.values[0]) && d.values[0]!.kind is ValLitNull {
          if self.prog!.slab_mode_soa {
            self.line("return 0;")
          } else {
            self.line("return NULL;")
          }
        } else {
          self.line(f"return {val};")
        }
        return
      }
      let opt_name = self.opt_type_name(self.current_func!.return_type!.elem)
      if !isnull(d.values[0]) && d.values[0]!.kind is ValLitNull {
        self.line(f"return lyric_none({opt_name});")
      } else if !isnull(d.values[0]) && !isnull(d.values[0]!.typ) && d.values[0]!.typ!.kind is TyOptional {
        self.line(f"return {val};")
      } else {
        self.line(f"return lyric_some({val}, {opt_name});")
      }
      return
    }
    self.line(f"return {val};")
    return
  }
  // 2-value return for ErrorResult
  if len(d.values) == 2 && !isnull(self.current_func) && !isnull(self.current_func!.return_type) && self.current_func!.return_type!.kind is TyErrorResult {
    let result_name = self.result_type_name(self.current_func!.return_type!.elem)
    let val_str = self.emit_value(d.values[0])
    if !isnull(d.values[1]) && (d.values[1]!.kind is ValLitNull || (d.values[1]!.kind is ValLitString && d.values[1]!.str_val == "")) {
      // Wrap value in lyric_some if ErrorResult's Elem is Optional and value isn't already optional
      let ret_elem = self.current_func!.return_type!.elem
      let val_type = self.resolve_value_type(d.values[0])
      if !isnull(ret_elem) && ret_elem!.kind is TyOptional && !self.is_class_optional(ret_elem) && (isnull(val_type) || !(val_type!.kind is TyOptional)) && !(d.values[0]!.kind is ValLitNull) {
        let opt_name = self.opt_type_name(ret_elem!.elem)
        self.line(f"return lyric_ok(lyric_some({val_str}, {opt_name}), {result_name});")
      } else {
        self.line(f"return lyric_ok({val_str}, {result_name});")
      }
    } else {
      self.line(f"return lyric_err({self.emit_value_as_cstr(d.values[1])}, {result_name});")
    }
    return
  }
  // Multi-value return for tuple types — construct tuple struct
  if !isnull(self.current_func) && !isnull(self.current_func!.return_type) && self.current_func!.return_type!.kind is TyTuple {
    let tuple_name = self.c_tuple_type(self.current_func!.return_type)
    let mut fields = ""
    let mut fi = 0
    while fi < len(d.values) {
      if fi > 0 { fields = fields + ", " }
      fields = fields + f"._{fi} = {self.emit_value(d.values[fi])}"
      fi = fi + 1
    }
    self.line(f"return ({tuple_name}){{ {fields} }};")
    return
  }
  self.line(f"return {self.emit_value(d.values[0])};")
}

func CGen.emit_for_stmt(self, s: LStmt?) {
  let d = s!.for_data!
  let coll_str = self.emit_value(d.collection)

  // Generator iteration
  if !isnull(d.collection) && !isnull(d.collection!.typ) && d.collection!.typ!.kind is TyGenerator {
    let gen_base_name = self.resolve_gen_base_name(d.collection)
    let struct_name = f"{gen_base_name}_gen_t"
    self.lambda_id = self.lambda_id + 1
    let iter_var = f"_gen_iter_{self.lambda_id}"
    self.line(f"{struct_name}* {iter_var} = {coll_str};")
    self.line(f"while ({gen_base_name}_next({iter_var})) {{")
    self.indent = self.indent + 1
    self.line(f"{self.c_type(d.var_type)} {d.var_name} = {iter_var}->_value;")
    self.emit_stmts(d.body)
    self.indent = self.indent - 1
    self.line("}")
    self.line(f"free({iter_var});")
    return
  }

  if d.index_var != "" {
    self.line(f"for (int32_t {d.index_var} = 0; {d.index_var} < {coll_str}.len; {d.index_var}++) {{")
    self.indent = self.indent + 1
    self.line(f"{self.c_type(d.var_type)} {d.var_name} = {coll_str}.data[{d.index_var}];")
  } else {
    self.line(f"for (int32_t _idx = 0; _idx < {coll_str}.len; _idx++) {{")
    self.indent = self.indent + 1
    self.line(f"{self.c_type(d.var_type)} {d.var_name} = {coll_str}.data[_idx];")
  }
  self.emit_stmts(d.body)
  self.indent = self.indent - 1
  self.line("}")
}

func CGen.emit_switch_stmt(self, s: LStmt?) {
  let d = s!.switch_data!
  self.line(f"switch ({self.emit_value(d.tag)}) {{")
  let mut has_default = false
  let mut i = 0
  while i < len(d.cases) {
    let c = d.cases[i]
    if c.tag == -1 {
      self.line(f"default: {{")
      has_default = true
    } else {
      self.line(f"case {c.tag}: {{")
    }
    self.indent = self.indent + 1
    if c.binding != "" && d.enum_name != "" {
      let variant_lower = c_safe_name(str_to_lower(c.binding))
      self.line(f"{d.enum_name}_{c.binding}_Data {c.binding} = {self.emit_value(d.tag)}.data.{variant_lower};")
    }
    self.emit_stmts(c.body)
    self.line("break;")
    self.indent = self.indent - 1
    self.line("}")
    i = i + 1
  }
  if !has_default {
    self.line("default: __builtin_unreachable();")
  }
  self.line("}")
}

func CGen.emit_type_switch_stmt(self, s: LStmt?) {
  let d = s!.type_switch!
  let val = self.emit_value(d.value)
  self.line(f"switch ({val}.tag) {{")
  let mut i = 0
  while i < len(d.cases) {
    let c = d.cases[i]
    if !isnull(c.typ) {
      // Check for ad-hoc union
      let val_type = self.resolve_value_type(d.value)
      if !isnull(val_type) && val_type!.kind is TyUnion {
        self.line(f"case {self.union_tag_for_type(c.typ)}: {{")
      } else {
        let tag_const = self.resolve_tag_constant(d, c.typ)
        if tag_const != "" {
          self.line(f"case {tag_const}: {{")
        } else {
          self.line(f"case /* {self.c_type(c.typ)} */0: {{")
        }
      }
    } else {
      self.line(f"default: {{")
    }
    self.indent = self.indent + 1
    self.emit_stmts(c.body)
    self.line("break;")
    self.indent = self.indent - 1
    self.line("}")
    i = i + 1
  }
  self.line("}")
}

func CGen.resolve_tag_constant(self, d: LTypeSwitchData, case_type: LType?) -> string {
  if isnull(d.value) || isnull(d.value!.typ) {
    return ""
  }
  let enum_name = d.value!.typ!.name
  let mut variant_name = ""
  if !isnull(case_type) && case_type!.name != "" {
    variant_name = case_type!.name
    let prefix = f"{enum_name}_"
    if str_has_prefix(variant_name, prefix) {
      variant_name = variant_name[len(prefix):len(variant_name)]
      // Strip _Data suffix if present
      if str_has_suffix(variant_name, "_Data") {
        variant_name = variant_name[0:len(variant_name) - 5]
      }
    }
  }
  if variant_name == "" {
    return ""
  }
  return f"{enum_name}_{variant_name}"
}

// ---------------------------------------------------------------------------
// Generator support
// ---------------------------------------------------------------------------

func CGen.resolve_gen_base_name(self, v: LValue?) -> string {
  if !isnull(v) && v!.kind is ValTemp {
    return self.find_gen_func_name_in_stmts(self.current_func!.body, v!.temp_id)
  }
  return "unknown_gen"
}

func CGen.find_gen_func_name_in_stmts(self, stmts: [LStmt?], temp_id: i32) -> string {
  let mut i = 0
  while i < len(stmts) {
    if isnull(stmts[i]) {
      i = i + 1
      // skip null
      continue
    }
    let s = stmts[i]!
    if s.kind is StTempDef {
      let d = s.temp_def!
      if d.id == temp_id {
        return self.extract_func_name_from_expr(d.expr)
      }
    }
    if s.kind is StIf {
      let d = s.if_data!
      let r = self.find_gen_func_name_in_stmts(d.then_body, temp_id)
      if r != "unknown_gen" { return r }
      let r2 = self.find_gen_func_name_in_stmts(d.else_body, temp_id)
      if r2 != "unknown_gen" { return r2 }
    }
    if s.kind is StWhile {
      let d = s.while_data!
      let r = self.find_gen_func_name_in_stmts(d.cond_block, temp_id)
      if r != "unknown_gen" { return r }
      let r2 = self.find_gen_func_name_in_stmts(d.body, temp_id)
      if r2 != "unknown_gen" { return r2 }
    }
    if s.kind is StFor {
      let d = s.for_data!
      let r = self.find_gen_func_name_in_stmts(d.body, temp_id)
      if r != "unknown_gen" { return r }
    }
    if s.kind is StSwitch {
      let d = s.switch_data!
      let mut j = 0
      while j < len(d.cases) {
        let r = self.find_gen_func_name_in_stmts(d.cases[j].body, temp_id)
        if r != "unknown_gen" { return r }
        j = j + 1
      }
    }
    i = i + 1
  }
  return "unknown_gen"
}

func CGen.extract_func_name_from_expr(self, e: LExpr?) -> string {
  if !isnull(e) && e!.kind is ExCall {
    let d = e!.call!
    return c_safe_name(d.func_name)
  }
  return "unknown_gen"
}

// ---------------------------------------------------------------------------
// Generator declarations
// ---------------------------------------------------------------------------

func CGen.collect_gen_vars(self, stmts: [LStmt?]) -> [LVarDeclData] {
  let mut result: [LVarDeclData] = []
  let mut i = 0
  while i < len(stmts) {
    if isnull(stmts[i]) {
      i = i + 1
      // skip
      continue
    }
    let s = stmts[i]!
    if s.kind is StVarDecl {
      append(result, s.var_decl!)
    }
    if s.kind is StWhile {
      let d = s.while_data!
      let sub = self.collect_gen_vars(d.body)
      let mut j = 0
      while j < len(sub) {
        append(result, sub[j])
        j = j + 1
      }
    }
    if s.kind is StIf {
      let d = s.if_data!
      let sub1 = self.collect_gen_vars(d.then_body)
      let mut j = 0
      while j < len(sub1) {
        append(result, sub1[j])
        j = j + 1
      }
      let sub2 = self.collect_gen_vars(d.else_body)
      j = 0
      while j < len(sub2) {
        append(result, sub2[j])
        j = j + 1
      }
    }
    if s.kind is StFor {
      let d = s.for_data!
      let sub = self.collect_gen_vars(d.body)
      let mut j = 0
      while j < len(sub) {
        append(result, sub[j])
        j = j + 1
      }
    }
    if s.kind is StBlock {
      let d = s.block!
      let sub = self.collect_gen_vars(d.stmts)
      let mut j = 0
      while j < len(sub) {
        append(result, sub[j])
        j = j + 1
      }
    }
    i = i + 1
  }
  return result
}

func CGen.count_yields(self, stmts: [LStmt?]) -> i32 {
  let mut count: i32 = 0
  let mut i = 0
  while i < len(stmts) {
    if isnull(stmts[i]) {
      i = i + 1
      // skip
      continue
    }
    let s = stmts[i]!
    if s.kind is StYield { count = count + 1 }
    if s.kind is StWhile {
      count = count + self.count_yields(s.while_data!.body)
    }
    if s.kind is StIf {
      count = count + self.count_yields(s.if_data!.then_body)
      count = count + self.count_yields(s.if_data!.else_body)
    }
    if s.kind is StFor {
      count = count + self.count_yields(s.for_data!.body)
    }
    if s.kind is StBlock {
      count = count + self.count_yields(s.block!.stmts)
    }
    i = i + 1
  }
  return count
}

func CGen.emit_generator_struct_decl(self, f: LFuncDecl) {
  let base_name = self.func_name(f)
  let struct_name = f"{base_name}_gen_t"
  let elem_type = self.c_type(f.return_type!.elem)

  self.line(f"typedef struct {struct_name} {{")
  self.indent = self.indent + 1
  self.line("int _state;")
  self.line(f"{elem_type} _value;")
  let mut i = 0
  while i < len(f.params) {
    self.line(f"{self.c_type(f.params[i].typ)} {c_safe_name(f.params[i].name)};")
    i = i + 1
  }
  let vars = self.collect_gen_vars(f.body)
  let seen = Dict<Sym, bool>()
  i = 0
  while i < len(vars) {
    let entry = seen.get(sym(vars[i].name))
    if isnull(entry) {
      seen.set(sym(vars[i].name), true)
      self.line(f"{self.c_type(vars[i].typ)} {c_safe_name(vars[i].name)};")
    }
    i = i + 1
  }
  self.indent = self.indent - 1
  self.line(f"}} {struct_name};")
  self.line("")
}

func CGen.emit_generator_forward_decls(self, f: LFuncDecl) {
  let base_name = self.func_name(f)
  let struct_name = f"{base_name}_gen_t"
  let params = self.c_param_list(f)
  self.line(f"{struct_name}* {base_name}_init({params});")
  self.line(f"bool {base_name}_next({struct_name}* _gen);")
}

func CGen.emit_generator_func_decl(self, f: LFuncDecl) {
  let base_name = self.func_name(f)
  let struct_name = f"{base_name}_gen_t"
  let params = self.c_param_list(f)

  // Init function
  self.line(f"{struct_name}* {base_name}_init({params}) {{")
  self.indent = self.indent + 1
  self.line(f"{struct_name}* _gen = ({struct_name}*)malloc(sizeof({struct_name}));")
  self.line("_gen->_state = 0;")
  let mut i = 0
  while i < len(f.params) {
    let name = c_safe_name(f.params[i].name)
    self.line(f"_gen->{name} = {name};")
    i = i + 1
  }
  let vars = self.collect_gen_vars(f.body)
  let seen = Dict<Sym, bool>()
  i = 0
  while i < len(vars) {
    let entry = seen.get(sym(vars[i].name))
    if isnull(entry) {
      seen.set(sym(vars[i].name), true)
      if !isnull(vars[i].init) && !(vars[i].init!.kind is ValTemp) {
        self.line(f"_gen->{c_safe_name(vars[i].name)} = {self.emit_value(vars[i].init)};")
      } else if isnull(vars[i].init) {
        self.line(f"_gen->{c_safe_name(vars[i].name)} = 0;")
      }
    }
    i = i + 1
  }
  self.line("return _gen;")
  self.indent = self.indent - 1
  self.line("}")
  self.line("")

  // Next function
  self.line(f"bool {base_name}_next({struct_name}* _gen) {{")
  self.indent = self.indent + 1
  let n_yields = self.count_yields(f.body)
  self.line(f"switch (_gen->_state) {{")
  self.line("case 0: goto _gen_s0;")
  let mut y = 1
  while y <= n_yields {
    self.line(f"case {y}: goto _gen_s{y};")
    y = y + 1
  }
  self.line("default: return false;")
  self.line("}")
  self.in_generator = true
  self.gen_struct_name = struct_name
  self.gen_yield_count = 0
  self.line("_gen_s0:;")
  self.emit_stmts(f.body)
  self.line("return false;")
  self.in_generator = false
  self.indent = self.indent - 1
  self.line("}")
  self.line("")
}

// ---------------------------------------------------------------------------
// infer_expr_type — resolve LTyAny for well-known builtins
// ---------------------------------------------------------------------------

func CGen.infer_expr_type(self, e: LExpr?) -> LType? {
  if isnull(e) {
    return LType { kind: TyI32, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 0, is_exported: false }
  }
  let t = e!.typ
  if !isnull(t) && !(t!.kind is TyAny) && !self.contains_type_var(t) {
    return t
  }
  // Try to infer from call
  if e!.kind is ExCall {
    let d = e!.call!
    let fn_entry = self.func_by_name!.get(sym(d.func_name))
    if !isnull(fn_entry) && !isnull(fn_entry!.value.return_type) && !(fn_entry!.value.return_type!.kind is TyAny) {
      return fn_entry!.value.return_type
    }
  }
  // Try method call
  if e!.kind is ExMethodCall {
    let d = e!.method_call!
    let recv_type = self.resolve_value_type(d.receiver)
    if !isnull(recv_type) && recv_type!.name != "" {
      let mut type_name = recv_type!.name
      if !isnull(self.prog!.class_renames) {
        let ren = self.prog!.class_renames!.get(sym(type_name))
        if !isnull(ren) {
          type_name = ren!.value
        }
      }
      let key = f"{type_name}.{d.method}"
      let fn_entry = self.func_by_name!.get(sym(key))
      if !isnull(fn_entry) && !isnull(fn_entry!.value.return_type) && !(fn_entry!.value.return_type!.kind is TyAny) {
        return fn_entry!.value.return_type
      }
    }
  }
  // UnwrapOptional
  if e!.kind is ExUnwrapOptional {
    let d = e!.unwrap_opt!
    let arg_type = self.resolve_value_type(d.value)
    if !isnull(arg_type) && arg_type!.kind is TyOptional && !isnull(arg_type!.elem) {
      return arg_type!.elem
    }
    if !isnull(arg_type) && arg_type!.kind is TyClassHandle {
      return arg_type
    }
  }
  // ClassGet
  if e!.kind is ExClassGet {
    let d = e!.class_get!
    if !isnull(d.handle) && !isnull(d.handle!.typ) {
      let field_type = self.resolve_field_type(d.handle!.typ, d.field)
      if !isnull(field_type) {
        return field_type
      }
    }
  }
  // SlabGet — same as ClassGet
  if e!.kind is ExSlabGet {
    let d = e!.slab_get!
    if !isnull(d.handle) && !isnull(d.handle!.typ) {
      let field_type = self.resolve_field_type(d.handle!.typ, d.field)
      if !isnull(field_type) {
        return field_type
      }
    }
  }
  // StructField
  if e!.kind is ExStructField {
    let d = e!.struct_field!
    if !isnull(d.receiver) && !isnull(d.receiver!.typ) {
      let field_type = self.resolve_field_type(d.receiver!.typ, d.field)
      if !isnull(field_type) {
        return field_type
      }
    }
  }
  // Builtin type inference
  if e!.kind is ExBuiltin {
    let d = e!.builtin!
    if d.name == "len" || d.name == "string_len" || d.name == "slice_len" || d.name == "map_len" {
      return LType { kind: TyPlatformInt, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 0, is_exported: false }
    }
    if d.name == "isnull" || d.name == "contains" || d.name == "has_prefix" || d.name == "has_suffix" || d.name == "file_exists" {
      return LType { kind: TyBool, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 0, is_exported: false }
    }
    if d.name == "itoa" || d.name == "char_to_string" || d.name == "mkdtemp" || d.name == "os_getwd" || d.name == "string_trim" {
      return LType { kind: TyString, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 0, is_exported: false }
    }
    if d.name == "hash_string" {
      return LType { kind: TyU64, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 64, is_exported: false }
    }
    if d.name == "assert" || d.name == "assert_eq" || d.name == "println" || d.name == "print" || d.name == "eprintln" || d.name == "eprint" || d.name == "panic" || d.name == "Println" || d.name == "Print" || d.name == "fmt.Println" || d.name == "fmt.Print" || d.name == "fmt.Printf" {
      return LType { kind: TyUnit, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 0, is_exported: false }
    }
  }
  if isnull(t) {
    return LType { kind: TyI32, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 0, is_exported: false }
  }
  return t
}

// ---------------------------------------------------------------------------
// to_string functions for assert_eq
// ---------------------------------------------------------------------------

func CGen.to_string_expr(self, v: LValue?) -> string {
  let t = self.resolve_value_type(v)
  let val = self.emit_value(v)
  if isnull(t) {
    return "LYRIC_STR(\"<unknown>\")"
  }
  match t!.kind {
    TyBool => { return f"({val} ? LYRIC_STR(\"true\") : LYRIC_STR(\"false\"))" }
    TyI8 | TyI16 | TyI32 | TyPlatformInt => { return "lyric_sprintf(\"%d\", " + val + ")" }
    TyI64 => { return "lyric_sprintf(\"%lld\", (long long)" + val + ")" }
    TyU8 | TyU16 | TyU32 | TyPlatformUint => { return "lyric_sprintf(\"%u\", (unsigned)" + val + ")" }
    TyU64 => { return "lyric_sprintf(\"%llu\", (unsigned long long)" + val + ")" }
    TyF32 => { return "lyric_sprintf(\"%g\", (double)" + val + ")" }
    TyF64 => { return "lyric_sprintf(\"%g\", " + val + ")" }
    TyString => { return "lyric_sprintf(\"\\\"%.*s\\\"\", (int)" + val + ".len, (const char*)" + val + ".data)" }
    TyTaggedUnion => { return f"{t!.name}_to_string({val})" }
    TyStruct => { return f"{t!.name}_to_string({val})" }
    TyClassHandle => {
      let null_check = if self.prog!.slab_mode_soa { "== 0" } else { "== NULL" }
      return f"({val} {null_check} ? LYRIC_STR(\"null\") : {t!.name}_to_string({val}))"
    }
    _ => { return f"LYRIC_STR(\"<{t!.name}>\")" }
  }
}

func CGen.eq_expr(self, a: LValue?, b: LValue?) -> string {
  let t = self.resolve_value_type(a)
  let va = self.emit_value(a)
  let vb = self.emit_value(b)
  if !isnull(t) && t!.kind is TyString {
    return f"lyric_str_eq({va}, {vb})"
  }
  if !isnull(t) && t!.kind is TyTaggedUnion {
    let sn = t!.name
    let entry = self.simple_enums!.get(sym(sn))
    if !isnull(entry) {
      return f"({va} == {vb})"
    }
    return f"({va}.tag == {vb}.tag)"
  }
  return f"({va} == {vb})"
}

func CGen.resolve_type_name(self, name: string) -> string {
  if !isnull(self.prog!.class_renames) {
    let ren = self.prog!.class_renames!.get(sym(name))
    if !isnull(ren) {
      return ren!.value
    }
  }
  return name
}

func CGen.field_to_string_expr(self, t: LType?, val: string) -> string {
  if isnull(t) {
    return "LYRIC_STR(\"?\")"
  }
  match t!.kind {
    TyBool => { return f"({val} ? LYRIC_STR(\"true\") : LYRIC_STR(\"false\"))" }
    TyI8 | TyI16 | TyI32 | TyPlatformInt => { return "lyric_sprintf(\"%d\", " + val + ")" }
    TyI64 => { return "lyric_sprintf(\"%lld\", (long long)" + val + ")" }
    TyU8 | TyU16 | TyU32 | TyPlatformUint => { return "lyric_sprintf(\"%u\", (unsigned)" + val + ")" }
    TyU64 => { return "lyric_sprintf(\"%llu\", (unsigned long long)" + val + ")" }
    TyF32 => { return "lyric_sprintf(\"%g\", (double)" + val + ")" }
    TyF64 => { return "lyric_sprintf(\"%g\", " + val + ")" }
    TyString => { return "lyric_sprintf(\"\\\"%.*s\\\"\", (int)" + val + ".len, (const char*)" + val + ".data)" }
    TyTaggedUnion => {
      let name = self.resolve_type_name(t!.name)
      return f"{name}_to_string({val})"
    }
    TyStruct => {
      let name = self.resolve_type_name(t!.name)
      return f"{name}_to_string({val})"
    }
    TyClassHandle => {
      let name = self.resolve_type_name(t!.name)
      let null_check = if self.prog!.slab_mode_soa { "== 0" } else { "== NULL" }
      return f"({val} {null_check} ? LYRIC_STR(\"null\") : {name}_to_string({val}))"
    }
    _ => { return f"LYRIC_STR(\"<{t!.name}>\")" }
  }
}

// ---------------------------------------------------------------------------
// Dedup helper
// ---------------------------------------------------------------------------

func dedup_structs(items: [LStructDecl]) -> [LStructDecl] {
  let seen = Dict<Sym, bool>()
  let mut result: [LStructDecl] = []
  let mut i = 0
  while i < len(items) {
    let entry = seen.get(sym(items[i].name))
    if isnull(entry) {
      seen.set(sym(items[i].name), true)
      append(result, items[i])
    }
    i = i + 1
  }
  return result
}

func dedup_classes(items: [LClassDecl]) -> [LClassDecl] {
  let seen = Dict<Sym, bool>()
  let mut result: [LClassDecl] = []
  let mut i = 0
  while i < len(items) {
    let entry = seen.get(sym(items[i].name))
    if isnull(entry) {
      seen.set(sym(items[i].name), true)
      append(result, items[i])
    }
    i = i + 1
  }
  return result
}

func dedup_enums(items: [LEnumDecl]) -> [LEnumDecl] {
  let seen = Dict<Sym, bool>()
  let mut result: [LEnumDecl] = []
  let mut i = 0
  while i < len(items) {
    let entry = seen.get(sym(items[i].name))
    if isnull(entry) {
      seen.set(sym(items[i].name), true)
      append(result, items[i])
    }
    i = i + 1
  }
  return result
}

func dedup_funcs(items: [LFuncDecl]) -> [LFuncDecl] {
  let seen = Dict<Sym, bool>()
  let mut result: [LFuncDecl] = []
  let mut i = 0
  while i < len(items) {
    let mut key = items[i].name
    if items[i].receiver != "" {
      key = f"{items[i].receiver}.{items[i].name}"
    }
    let entry = seen.get(sym(key))
    if isnull(entry) {
      seen.set(sym(key), true)
      append(result, items[i])
    }
    i = i + 1
  }
  return result
}

// ---------------------------------------------------------------------------
// Topo sort structs
// ---------------------------------------------------------------------------

func struct_type_name(t: LType?) -> string {
  if isnull(t) { return "" }
  if t!.kind is TyStruct { return t!.name }
  return ""
}

func topo_sort_structs(structs: [LStructDecl]) -> [LStructDecl] {
  if len(structs) <= 1 {
    return structs
  }
  let name_idx = Dict<Sym, i32>()
  let mut i = 0
  while i < len(structs) {
    name_idx.set(sym(structs[i].name), i as i32)
    i = i + 1
  }
  // Build reverse adjacency + in-degree
  let n = len(structs)
  // Use flat arrays for adjacency lists: rev_deps_start[i], rev_deps_count[i]
  // Simple approach: just do insertion sort by dependency
  let mut in_deg: [i32] = []
  let mut rev_deps_flat: [i32] = []
  let mut rev_deps_offsets: [i32] = []
  // Simpler: use quadratic approach — small N
  i = 0
  while i < n {
    append(in_deg, 0)
    i = i + 1
  }
  // Count deps
  i = 0
  while i < n {
    let mut j = 0
    while j < len(structs[i].fields) {
      let dep_name = struct_type_name(structs[i].fields[j].typ)
      if dep_name != "" {
        let dep_entry = name_idx.get(sym(dep_name))
        if !isnull(dep_entry) && dep_entry!.value != i as i32 {
          in_deg[i] = in_deg[i] + 1
        }
      }
      j = j + 1
    }
    i = i + 1
  }
  // Kahn's: find nodes with in_deg 0
  let mut queue: [i32] = []
  i = 0
  while i < n {
    if in_deg[i] == 0 {
      append(queue, i as i32)
    }
    i = i + 1
  }
  let mut result: [LStructDecl] = []
  let added = Dict<Sym, bool>()
  let mut qi = 0
  while qi < len(queue) {
    let idx = queue[qi]
    append(result, structs[idx])
    added.set(sym(structs[idx].name), true)
    // Find dependents
    let mut j = 0
    while j < n {
      let mut k = 0
      while k < len(structs[j as i32].fields) {
        let dep_name = struct_type_name(structs[j as i32].fields[k].typ)
        if dep_name == structs[idx].name {
          in_deg[j] = in_deg[j] - 1
          if in_deg[j] == 0 {
            append(queue, j as i32)
          }
        }
        k = k + 1
      }
      j = j + 1
    }
    qi = qi + 1
  }
  // Append remaining (cycles)
  if len(result) < n {
    i = 0
    while i < n {
      let entry = added.get(sym(structs[i].name))
      if isnull(entry) {
        append(result, structs[i])
      }
      i = i + 1
    }
  }
  return result
}

// ---------------------------------------------------------------------------
// String lowercase helper
// ---------------------------------------------------------------------------

func str_to_lower(s: string) -> string {
  let sb = new_string_builder()
  let mut i = 0
  while i < len(s) {
    let c = s[i]
    if c >= 'A' as u8 && c <= 'Z' as u8 {
      sb.write_byte((c as i32 + 32) as u8)
    } else {
      sb.write_byte(c)
    }
    i = i + 1
  }
  return sb.to_string()
}

// ---------------------------------------------------------------------------
// collectCompositeTypes — pre-scan to register slice/opt/result types
// ---------------------------------------------------------------------------

func CGen.collect_composite_types(self) {
  // Walk all struct/class/enum field types
  let structs = self.prog!.structs
  let mut i = 0
  while i < len(structs) {
    let mut j = 0
    while j < len(structs[i].fields) {
      self.walk_type_for_composite(structs[i].fields[j].typ)
      j = j + 1
    }
    i = i + 1
  }
  let classes = self.prog!.classes
  i = 0
  while i < len(classes) {
    let mut j = 0
    while j < len(classes[i].fields) {
      self.walk_type_for_composite(classes[i].fields[j].typ)
      j = j + 1
    }
    i = i + 1
  }
  let enums = self.prog!.enums
  i = 0
  while i < len(enums) {
    let mut j = 0
    while j < len(enums[i].variants) {
      let mut k = 0
      while k < len(enums[i].variants[j].fields) {
        self.walk_type_for_composite(enums[i].variants[j].fields[k].typ)
        k = k + 1
      }
      j = j + 1
    }
    i = i + 1
  }
  let funcs = self.prog!.functions
  i = 0
  while i < len(funcs) {
    self.walk_type_for_composite(funcs[i].return_type)
    let mut j = 0
    while j < len(funcs[i].params) {
      self.walk_type_for_composite(funcs[i].params[j].typ)
      j = j + 1
    }
    // Walk function body for types used in expressions and locals
    self.walk_stmts_for_composite(funcs[i].body)
    i = i + 1
  }
}

func CGen.walk_type_for_composite(self, t: LType?) {
  if isnull(t) { return }
  if self.contains_type_var(t) { return }
  match t!.kind {
    TySlice => { self.slice_type_name(t!.elem) }
    TyOptional => {
      if isnull(t!.elem) || !(t!.elem!.kind is TyClassHandle) {
        self.opt_type_name(t!.elem)
      }
    }
    TyErrorResult => { self.result_type_name(t!.elem) }
    TyChannel => {
      if !isnull(t!.elem) {
        self.chan_suffix(t!.elem)
      }
    }
    TyTuple => { self.c_tuple_type(t) }
    _ => {}
  }
  self.walk_type_for_composite(t!.elem)
  self.walk_type_for_composite(t!.key)
  self.walk_type_for_composite(t!.ret)
  let mut i = 0
  while i < len(t!.params) {
    self.walk_type_for_composite(t!.params[i])
    i = i + 1
  }
  i = 0
  while i < len(t!.type_args) {
    self.walk_type_for_composite(t!.type_args[i])
    i = i + 1
  }
  i = 0
  while i < len(t!.fields) {
    self.walk_type_for_composite(t!.fields[i].typ)
    i = i + 1
  }
}

func CGen.walk_val_for_composite(self, v: LValue?) {
  if !isnull(v) {
    self.walk_type_for_composite(v!.typ)
  }
}

func CGen.walk_expr_for_composite(self, e: LExpr?) {
  if isnull(e) { return }
  self.walk_type_for_composite(e!.typ)
  match e!.kind {
    ExCall => {
      if !isnull(e!.call) {
        let mut i = 0
        while i < len(e!.call!.args) {
          self.walk_val_for_composite(e!.call!.args[i])
          i = i + 1
        }
      }
    }
    ExBuiltin => {
      if !isnull(e!.builtin) {
        let mut i = 0
        while i < len(e!.builtin!.args) {
          self.walk_val_for_composite(e!.builtin!.args[i])
          i = i + 1
        }
      }
    }
    ExMakeChannel => {
      if !isnull(e!.make_channel) {
        self.walk_type_for_composite(e!.make_channel!.elem_type)
      }
    }
    ExMakeResult => {
      if !isnull(e!.make_result) {
        self.walk_val_for_composite(e!.make_result!.value)
        self.walk_val_for_composite(e!.make_result!.err)
      }
    }
    ExWrapOptional => {
      if !isnull(e!.wrap_opt) {
        self.walk_val_for_composite(e!.wrap_opt!.value)
      }
    }
    _ => {}
  }
}

func CGen.walk_stmts_for_composite(self, stmts: [LStmt?]) {
  let mut i = 0
  while i < len(stmts) {
    if isnull(stmts[i]) {
      i = i + 1
      continue
    }
    let s = stmts[i]!
    match s.kind {
      StTempDef => {
        if !isnull(s.temp_def) {
          self.walk_expr_for_composite(s.temp_def!.expr)
        }
      }
      StVarDecl => {
        if !isnull(s.var_decl) {
          self.walk_type_for_composite(s.var_decl!.typ)
          self.walk_val_for_composite(s.var_decl!.init)
        }
      }
      StSideEffect => {
        if !isnull(s.side_effect) {
          self.walk_expr_for_composite(s.side_effect!.expr)
        }
      }
      StMultiAssign => {
        if !isnull(s.multi_assign) {
          self.walk_expr_for_composite(s.multi_assign!.expr)
        }
      }
      StIf => {
        if !isnull(s.if_data) {
          self.walk_stmts_for_composite(s.if_data!.then_body)
          self.walk_stmts_for_composite(s.if_data!.else_body)
        }
      }
      StWhile => {
        if !isnull(s.while_data) {
          self.walk_stmts_for_composite(s.while_data!.cond_block)
          self.walk_stmts_for_composite(s.while_data!.body)
        }
      }
      StFor => {
        if !isnull(s.for_data) {
          self.walk_type_for_composite(s.for_data!.var_type)
          self.walk_stmts_for_composite(s.for_data!.body)
        }
      }
      StSwitch => {
        if !isnull(s.switch_data) {
          let mut j = 0
          while j < len(s.switch_data!.cases) {
            self.walk_stmts_for_composite(s.switch_data!.cases[j].body)
            j = j + 1
          }
        }
      }
      StTypeSwitch => {
        if !isnull(s.type_switch) {
          let mut j = 0
          while j < len(s.type_switch!.cases) {
            self.walk_stmts_for_composite(s.type_switch!.cases[j].body)
            j = j + 1
          }
        }
      }
      StBlock => {
        if !isnull(s.block) {
          self.walk_stmts_for_composite(s.block!.stmts)
        }
      }
      StSpawn => {
        if !isnull(s.spawn_data) {
          self.walk_stmts_for_composite(s.spawn_data!.body)
        }
      }
      StSend => {
        if !isnull(s.send_data) {
          self.walk_val_for_composite(s.send_data!.channel)
          self.walk_val_for_composite(s.send_data!.value)
        }
      }
      StReturn => {
        if !isnull(s.ret) {
          let mut j = 0
          while j < len(s.ret!.values) {
            self.walk_val_for_composite(s.ret!.values[j])
            j = j + 1
          }
        }
      }
      _ => {}
    }
    i = i + 1
  }
}

// ---------------------------------------------------------------------------
// Type declaration emission
// ---------------------------------------------------------------------------

func CGen.emit_struct_decl(self, s: LStructDecl) {
  self.line(f"struct {s.name} {{")
  self.indent = self.indent + 1
  if len(s.fields) == 0 {
    self.line("int _empty; /* C requires at least one field */")
  }
  let mut i = 0
  while i < len(s.fields) {
    self.line(f"{self.c_field_decl(s.fields[i].typ, s.fields[i].name)};")
    i = i + 1
  }
  self.indent = self.indent - 1
  self.line("};")
  self.line("")
}

func CGen.emit_class_decl(self, c: LClassDecl) {
  if self.prog!.slab_mode_soa {
    // SoA: no struct definition — fields are parallel arrays in slab
    // Emit typedef for documentation
    self.line(f"/* class {c.name}: SoA slab — no struct emitted */")
    self.line("")
    return
  }
  self.line(f"struct {c.name} {{")
  self.indent = self.indent + 1
  if len(c.fields) == 0 && !self.prog!.slab_mode {
    self.line("int _empty;")
  }
  let mut i = 0
  while i < len(c.fields) {
    self.line(f"{self.c_field_decl(c.fields[i].typ, c.fields[i].name)};")
    i = i + 1
  }
  if self.prog!.slab_mode {
    // Ref count for non-permanent, non-owned classes
    if !c.is_permanent {
      self.line("uint32_t _rc;")
    }
    // Free-list linkage: lyric_next at end so field offsets are unchanged
    self.line(f"struct {c.name}* lyric_next;")
  }
  self.indent = self.indent - 1
  self.line("};")
  self.line("")
}

// Emit slab allocator infrastructure for all classes
// Emit slab allocator infrastructure for all classes (AoS block-based).
// Class handles are ClassName* pointers (NULL = none).
// Each block holds LYRIC_SLAB_BLOCK objects; blocks form a linked list.
// lyric_next field in each struct serves as the free-list link.
func CGen.emit_slab_infrastructure(self, classes: [LClassDecl]) {
  if self.prog!.slab_mode_soa {
    self.emit_slab_infrastructure_soa(classes)
    return
  }
  self.line("/* Slab allocator infrastructure (AoS block-based) */")

  // Block type + slab type + global for each class
  let mut i = 0
  while i < len(classes) {
    let name = classes[i].name
    // Block struct: array of objects + next pointer + used count
    self.line(f"typedef struct LyricSlab_{name}_Block {{")
    self.indent = self.indent + 1
    self.line(f"struct {name} data[LYRIC_SLAB_BLOCK];")
    self.line(f"struct LyricSlab_{name}_Block* next;")
    self.line("int32_t used;")
    self.indent = self.indent - 1
    self.line(f"}} LyricSlab_{name}_Block;")
    // Slab control: current block + free-list head
    self.line(f"typedef struct {{ LyricSlab_{name}_Block* cur; {name}* free; }} LyricSlab_{name};")
    self.line(f"static LyricSlab_{name} _lyric_slab_{name} = {{0}};")

    self.line("")
    i = i + 1
  }

  // Alloc function for each class (returns ClassName*)
  i = 0
  while i < len(classes) {
    let name = classes[i].name
    self.line(f"static {name}* _lyric_slab_alloc_{name}(void) {{")
    self.indent = self.indent + 1
    // Check free list first
    self.line(f"if (_lyric_slab_{name}.free) {{")
    self.indent = self.indent + 1
    self.line(f"{name}* p = _lyric_slab_{name}.free;")
    self.line(f"_lyric_slab_{name}.free = p->lyric_next;")
    self.line(f"memset(p, 0, sizeof({name}));")
    if !classes[i].is_permanent {
      self.line("p->_rc = 1;")
    }
    self.line("return p;")
    self.indent = self.indent - 1
    self.line("}")
    // Allocate new block if current is full or missing
    self.line(f"if (!_lyric_slab_{name}.cur || _lyric_slab_{name}.cur->used == LYRIC_SLAB_BLOCK) {{")
    self.indent = self.indent + 1
    self.line(f"LyricSlab_{name}_Block* b = (LyricSlab_{name}_Block*)calloc(1, sizeof(LyricSlab_{name}_Block));")
    self.line(f"b->next = _lyric_slab_{name}.cur;")
    self.line(f"_lyric_slab_{name}.cur = b;")
    self.indent = self.indent - 1
    self.line("}")
    // Allocate from current block (calloc zeroed the block, so _rc is 0)
    self.line(f"{name}* p = &_lyric_slab_{name}.cur->data[_lyric_slab_{name}.cur->used++];")
    if !classes[i].is_permanent {
      self.line("p->_rc = 1;")
    }
    self.line("return p;")
    self.indent = self.indent - 1
    self.line("}")
    self.line("")
    i = i + 1
  }

  // Free function for each class (takes ClassName*)
  i = 0
  while i < len(classes) {
    let name = classes[i].name
    self.line(f"static void _lyric_slab_free_{name}({name}* p) {{")
    self.indent = self.indent + 1
    self.line("if (!p) return;")
    self.line(f"p->lyric_next = _lyric_slab_{name}.free;")
    self.line(f"_lyric_slab_{name}.free = p;")
    self.indent = self.indent - 1
    self.line("}")
    self.line("")
    i = i + 1
  }
}

// SoA slab: parallel arrays per field, uint32_t handles, realloc growth
func CGen.emit_slab_infrastructure_soa(self, classes: [LClassDecl]) {
  self.line("/* Slab allocator infrastructure (SoA parallel-array) */")

  // Slab struct + global for each class
  let mut i = 0
  while i < len(classes) {
    let name = classes[i].name
    self.line(f"typedef struct {{")
    self.indent = self.indent + 1
    let mut j = 0
    while j < len(classes[i].fields) {
      let f = classes[i].fields[j]
      self.line(f"{self.c_type(f.typ)}* {lc_first(f.name)};")
      j = j + 1
    }
    self.line("uint32_t* lyric_next;")
    if !classes[i].is_permanent {
      self.line("uint32_t* _rc;")
    }
    self.line("uint32_t used;")
    self.line("uint32_t cap;")
    self.line("uint32_t free_head;")
    self.indent = self.indent - 1
    self.line(f"}} LyricSlab_{name};")
    self.line(f"static LyricSlab_{name} _lyric_slab_{name} = {{ .used = 1 }};")

    self.line("")
    i = i + 1
  }

  // Alloc function for each class (returns uint32_t handle)
  i = 0
  while i < len(classes) {
    let name = classes[i].name
    self.line(f"static {name} _lyric_slab_alloc_{name}(void) {{")
    self.indent = self.indent + 1
    self.line("uint32_t h;")
    // Check free list first
    self.line(f"if (_lyric_slab_{name}.free_head) {{")
    self.indent = self.indent + 1
    self.line(f"h = _lyric_slab_{name}.free_head;")
    self.line(f"_lyric_slab_{name}.free_head = _lyric_slab_{name}.lyric_next[h];")
    self.indent = self.indent - 1
    self.line("} else {")
    self.indent = self.indent + 1
    // Grow if needed
    self.line(f"if (_lyric_slab_{name}.used >= _lyric_slab_{name}.cap) {{")
    self.indent = self.indent + 1
    self.line(f"uint32_t new_cap = _lyric_slab_{name}.cap ? _lyric_slab_{name}.cap * 2 : 64;")
    let mut j = 0
    while j < len(classes[i].fields) {
      let f = classes[i].fields[j]
      let ct = self.c_type(f.typ)
      self.line(f"_lyric_slab_{name}.{lc_first(f.name)} = ({ct}*)realloc(_lyric_slab_{name}.{lc_first(f.name)}, new_cap * sizeof({ct}));")
      j = j + 1
    }
    self.line(f"_lyric_slab_{name}.lyric_next = (uint32_t*)realloc(_lyric_slab_{name}.lyric_next, new_cap * sizeof(uint32_t));")
    if !classes[i].is_permanent {
      self.line(f"_lyric_slab_{name}._rc = (uint32_t*)realloc(_lyric_slab_{name}._rc, new_cap * sizeof(uint32_t));")
    }    self.line(f"_lyric_slab_{name}.cap = new_cap;")
    self.indent = self.indent - 1
    self.line("}")
    self.line(f"h = _lyric_slab_{name}.used++;")
    self.indent = self.indent - 1
    self.line("}")
    // Zero all fields at the allocated slot
    j = 0
    while j < len(classes[i].fields) {
      let f = classes[i].fields[j]
      let ct = self.c_type(f.typ)
      let zv = self.zero_value(f.typ)
      if zv == "{0}" {
        self.line(f"memset(&_lyric_slab_{name}.{lc_first(f.name)}[h], 0, sizeof({ct}));")
      } else {
        self.line(f"_lyric_slab_{name}.{lc_first(f.name)}[h] = {zv};")
      }
      j = j + 1
    }
    if !classes[i].is_permanent {
      self.line(f"_lyric_slab_{name}._rc[h] = 1;")
    }
    self.line("return h;")
    self.indent = self.indent - 1
    self.line("}")
    self.line("")
    self.line("")
    i = i + 1
  }

  // Free function for each class
  i = 0
  while i < len(classes) {
    let name = classes[i].name
    self.line(f"static void _lyric_slab_free_{name}({name} h) {{")
    self.indent = self.indent + 1
    self.line("if (!h) return;")
    self.line(f"_lyric_slab_{name}.lyric_next[h] = _lyric_slab_{name}.free_head;")
    self.line(f"_lyric_slab_{name}.free_head = h;")
    self.indent = self.indent - 1
    self.line("}")
    self.line("")
    i = i + 1
  }
}

func CGen.emit_simple_enum_decl(self, e: LEnumDecl) {
  self.simple_enums!.set(sym(e.name), true)
  self.line(f"typedef enum {{")
  self.indent = self.indent + 1
  let mut i = 0
  while i < len(e.variants) {
    let comma = if i < len(e.variants) - 1 { "," } else { "" }
    self.line(f"{e.name}_{e.variants[i].name} = {e.variants[i].tag}{comma}")
    i = i + 1
  }
  self.indent = self.indent - 1
  self.line(f"}} {e.name};")
  self.line("")
}

func CGen.emit_enum_decl(self, e: LEnumDecl) {
  // Tag enum
  self.line(f"enum {e.name}_Tag {{")
  self.indent = self.indent + 1
  let mut i = 0
  while i < len(e.variants) {
    let comma = if i < len(e.variants) - 1 { "," } else { "" }
    self.line(f"{e.name}_{e.variants[i].name} = {e.variants[i].tag}{comma}")
    i = i + 1
  }
  self.indent = self.indent - 1
  self.line("};")
  self.line("")

  // Variant data structs
  i = 0
  while i < len(e.variants) {
    if len(e.variants[i].fields) > 0 {
      self.line(f"typedef struct {{")
      self.indent = self.indent + 1
      let mut j = 0
      while j < len(e.variants[i].fields) {
        self.line(f"{self.c_field_decl(e.variants[i].fields[j].typ, e.variants[i].fields[j].name)};")
        j = j + 1
      }
      self.indent = self.indent - 1
      self.line(f"}} {e.name}_{e.variants[i].name}_Data;")
      self.line("")
    }
    i = i + 1
  }

  // Tagged union struct
  self.line(f"struct {e.name} {{")
  self.indent = self.indent + 1
  self.line(f"enum {e.name}_Tag tag;")
  self.line(f"union {{")
  self.indent = self.indent + 1
  i = 0
  while i < len(e.variants) {
    if len(e.variants[i].fields) > 0 {
      let variant_lower = c_safe_name(str_to_lower(e.variants[i].name))
      self.line(f"{e.name}_{e.variants[i].name}_Data {variant_lower};")
    }
    i = i + 1
  }
  self.indent = self.indent - 1
  self.line("} data;")
  self.indent = self.indent - 1
  self.line("};")
  self.line("")
}

// ---------------------------------------------------------------------------
// Function declarations
// ---------------------------------------------------------------------------

func CGen.emit_func_forward_decl(self, f: LFuncDecl) {
  let name = self.func_name(f)
  if name == "main" { return }
  if self.is_generator_func(f) {
    self.emit_generator_forward_decls(f)
    return
  }
  let ret_type = self.c_return_type(f.return_type)
  let params = self.c_param_list(f)
  self.line(f"{ret_type} {name}({params});")
}

func CGen.emit_func_decl(self, f: LFuncDecl) {
  self.current_func = f
  if self.is_generator_func(f) {
    self.emit_generator_func_decl(f)
    return
  }
  self.temp_types = Dict<Sym, LType?>()
  self.var_types = Dict<Sym, LType?>()
  let ret_type = self.c_return_type(f.return_type)
  let params = self.c_param_list(f)
  let name = self.func_name(f)

  if name == "main" {
    self.line(f"int main(int _argc, char** _argv) {{")
  } else {
    self.line(f"{ret_type} {name}({params}) {{")
  }
  self.indent = self.indent + 1
  // Register param types
  let mut i = 0
  while i < len(f.params) {
    if !isnull(f.params[i].typ) {
      self.var_types!.set(sym(f.params[i].name), f.params[i].typ)
    }
    i = i + 1
  }
  self.emit_stmts(f.body)
  if name == "main" {
    self.line("return 0;")
  }
  self.indent = self.indent - 1
  self.line("}")
  self.line("")
}

// ---------------------------------------------------------------------------
// to_string function emission
// ---------------------------------------------------------------------------

func CGen.emit_to_string_functions(self) {
  let has_to_string = Dict<Sym, bool>()
  let funcs = self.prog!.functions
  let mut i = 0
  while i < len(funcs) {
    if funcs[i].name == "to_string" && funcs[i].receiver != "" {
      has_to_string.set(sym(funcs[i].receiver), true)
    }
    i = i + 1
  }

  // Forward declarations
  let enums = self.prog!.enums
  i = 0
  while i < len(enums) {
    let name = enums[i].name
    if isnull(has_to_string.get(sym(name))) {
      self.line(f"static lyric_string {name}_to_string({name} v);")
    }
    i = i + 1
  }
  let structs = self.prog!.structs
  i = 0
  while i < len(structs) {
    let name = structs[i].name
    if isnull(has_to_string.get(sym(name))) {
      self.line(f"static lyric_string {name}_to_string({name} v);")
    }
    i = i + 1
  }
  let classes = self.prog!.classes
  i = 0
  while i < len(classes) {
    let name = classes[i].name
    if isnull(has_to_string.get(sym(name))) {
      if self.prog!.slab_mode_soa {
        self.line(f"static lyric_string {name}_to_string({name} v);")
      } else {
        self.line(f"static lyric_string {name}_to_string({name}* v);")
      }
    }
    i = i + 1
  }
  self.line("")

  // Enum to_string
  i = 0
  while i < len(enums) {
    let name = enums[i].name
    if !isnull(has_to_string.get(sym(name))) {
      i = i + 1
      // skip
      continue
    }
    self.line(f"static lyric_string {name}_to_string({name} v) {{")
    self.indent = self.indent + 1
    let is_simple = !isnull(self.simple_enums!.get(sym(name)))
    if is_simple {
      self.line(f"switch (v) {{")
    } else {
      self.line(f"switch (v.tag) {{")
    }
    self.indent = self.indent + 1
    let mut j = 0
    while j < len(enums[i].variants) {
      self.line(f"case {name}_{enums[i].variants[j].name}: return LYRIC_STR(\"{enums[i].variants[j].name}\");")
      j = j + 1
    }
    self.line(f"default: return LYRIC_STR(\"<unknown {name}>\");")
    self.indent = self.indent - 1
    self.line("}")
    self.indent = self.indent - 1
    self.line("}")
    self.line("")
    i = i + 1
  }

  // Struct to_string
  i = 0
  while i < len(structs) {
    let name = structs[i].name
    if !isnull(has_to_string.get(sym(name))) {
      i = i + 1
      // skip
      continue
    }
    self.line(f"static lyric_string {name}_to_string({name} v) {{")
    self.indent = self.indent + 1
    self.emit_field_dump_to_string(name, structs[i].fields, "v.", "")
    self.indent = self.indent - 1
    self.line("}")
    self.line("")
    i = i + 1
  }

  // Class to_string
  i = 0
  while i < len(classes) {
    let name = classes[i].name
    if !isnull(has_to_string.get(sym(name))) {
      i = i + 1
      // skip
      continue
    }
    if self.prog!.slab_mode_soa {
      self.line(f"static lyric_string {name}_to_string({name} v) {{")
      self.indent = self.indent + 1
      self.emit_field_dump_to_string(name, classes[i].fields, f"_lyric_slab_{name}.", "[v]")
    } else if self.prog!.slab_mode {
      self.line(f"static lyric_string {name}_to_string({name}* v) {{")
      self.indent = self.indent + 1
      self.emit_field_dump_to_string(name, classes[i].fields, "v->", "")
    } else {
      self.line(f"static lyric_string {name}_to_string({name}* v) {{")
      self.indent = self.indent + 1
      self.emit_field_dump_to_string(name, classes[i].fields, "v->", "")
    }
    self.indent = self.indent - 1
    self.line("}")
    self.line("")
    i = i + 1
  }
}

func CGen.emit_field_dump_to_string(self, type_name: string, fields: [LField], prefix: string, suffix: string) {
  if len(fields) == 0 {
    self.line(f"return LYRIC_STR(\"{type_name}{{}}\");")
    return
  }
  self.line(f"lyric_string _result = LYRIC_STR(\"{type_name}{{\");")
  let mut i = 0
  while i < len(fields) {
    if i > 0 {
      self.line("_result = lyric_str_concat(_result, LYRIC_STR(\", \"));")
    }
    self.line(f"_result = lyric_str_concat(_result, LYRIC_STR(\"{fields[i].name}: \"));")
    let field_val = f"{prefix}{c_safe_name(fields[i].name)}{suffix}"
    let field_str = self.field_to_string_expr(fields[i].typ, field_val)
    self.line(f"_result = lyric_str_concat(_result, {field_str});")
    i = i + 1
  }
  self.line("_result = lyric_str_concat(_result, LYRIC_STR(\"}\"));")
  self.line("return _result;")
}

// ---------------------------------------------------------------------------
// OS helpers emission
// ---------------------------------------------------------------------------

func CGen.emit_os_helpers(self) {
  let slice_type = self.slice_type_name(LType { kind: TyString, name: "", elem: null, key: null, fields: [], params: [], ret: null, variants: [], type_args: [], bits: 0, is_exported: false })

  if self.needs_os_args {
    self.line(f"static inline {slice_type} _lyric_os_args(int argc, char** argv) {{")
    self.indent = self.indent + 1
    self.line(f"{slice_type} result = {{0}};")
    self.line("result.data = (lyric_string*)malloc(sizeof(lyric_string) * argc);")
    self.line("result.len = (int32_t)argc;")
    self.line("result.cap = (int32_t)argc;")
    self.line("for (int i = 0; i < argc; i++) result.data[i] = lyric_str_from_cstr(argv[i]);")
    self.line("return result;")
    self.indent = self.indent - 1
    self.line("}")
    self.line("")
  }
  if self.needs_exec_cmd {
    self.line(f"static inline lyric_str_bool_t _lyric_exec_command(lyric_string program, {slice_type} args) {{")
    self.indent = self.indent + 1
    self.line("size_t total = program.len + 1;")
    self.line("for (int i = 0; i < args.len; i++) total += args.data[i].len + 3;")
    self.line("char* cmd = (char*)malloc(total + 16);")
    self.line("memcpy(cmd, program.data, program.len); cmd[program.len] = '\\0';")
    self.line(f"for (int i = 0; i < args.len; i++) {{ strcat(cmd, \" \"); strncat(cmd, (const char*)args.data[i].data, args.data[i].len); }}")
    self.line("strcat(cmd, \" 2>&1\");")
    self.line("FILE* fp = popen(cmd, \"r\");")
    self.line("free(cmd);")
    self.line(f"if (!fp) {{ lyric_str_bool_t r = {{LYRIC_STR_EMPTY, false}}; return r; }}")
    self.line("size_t cap = 4096, len = 0;")
    self.line("uint8_t* out = (uint8_t*)malloc(cap);")
    self.line("size_t n;")
    self.line(f"while ((n = fread(out + len, 1, cap - len - 1, fp)) > 0) {{")
    self.indent = self.indent + 1
    self.line("len += n;")
    self.line(f"if (len + 1 >= cap) {{ cap *= 2; out = (uint8_t*)realloc(out, cap); }}")
    self.indent = self.indent - 1
    self.line("}")
    self.line("out[len] = '\\0';")
    self.line("int status = pclose(fp);")
    self.line(f"lyric_str_bool_t r = {{{{.data = out, .len = (int32_t)len, .cap = (int32_t)len}}, status == 0}};")
    self.line("return r;")
    self.indent = self.indent - 1
    self.line("}")
    self.line("")
  }
  if self.needs_path_join {
    self.line(f"static inline lyric_string _lyric_path_join({slice_type} parts) {{")
    self.indent = self.indent + 1
    self.line("if (parts.len == 0) return LYRIC_STR_EMPTY;")
    self.line("int32_t total = 0;")
    self.line("for (int i = 0; i < parts.len; i++) total += parts.data[i].len + 1;")
    self.line("uint8_t* out = (uint8_t*)malloc(total + 1);")
    self.line("int32_t pos = 0;")
    self.line(f"for (int i = 0; i < parts.len; i++) {{")
    self.indent = self.indent + 1
    self.line(f"if (i > 0) {{ out[pos++] = '/'; }}")
    self.line("memcpy(out + pos, parts.data[i].data, parts.data[i].len);")
    self.line("pos += parts.data[i].len;")
    self.indent = self.indent - 1
    self.line("}")
    self.line("out[pos] = '\\0';")
    self.line(f"return (lyric_string){{.data = out, .len = pos, .cap = pos}};")
    self.indent = self.indent - 1
    self.line("}")
    self.line("")
  }
}

// ---------------------------------------------------------------------------
// Spawn capture collection helpers
// ---------------------------------------------------------------------------

func collect_val_vars(v: LValue?, used: Dict<Sym, bool>?) {
  if isnull(v) { return }
  if v!.kind is ValVar {
    used!.set(sym(v!.name), true)
  }
}

func collect_expr_vars(e: LExpr?, used: Dict<Sym, bool>?) {
  if isnull(e) { return }
  match e!.kind {
    ExCall => {
      let d = e!.call!
      let mut i = 0
      while i < len(d.args) {
        collect_val_vars(d.args[i], used)
        i = i + 1
      }
    }
    ExMethodCall => {
      let d = e!.method_call!
      collect_val_vars(d.receiver, used)
      let mut i = 0
      while i < len(d.args) {
        collect_val_vars(d.args[i], used)
        i = i + 1
      }
    }
    ExBuiltin => {
      let d = e!.builtin!
      let mut i = 0
      while i < len(d.args) {
        collect_val_vars(d.args[i], used)
        i = i + 1
      }
    }
    ExBinOp => {
      let d = e!.bin_op!
      collect_val_vars(d.left, used)
      collect_val_vars(d.right, used)
    }
    ExUnOp => {
      let d = e!.un_op!
      collect_val_vars(d.operand, used)
    }
    ExCast => {
      let d = e!.cast!
      collect_val_vars(d.operand, used)
    }
    ExStructField => {
      let d = e!.struct_field!
      collect_val_vars(d.receiver, used)
    }
    ExClassGet => {
      let d = e!.class_get!
      collect_val_vars(d.handle, used)
    }
    ExSlabGet => {
      let d = e!.slab_get!
      collect_val_vars(d.handle, used)
    }
    ExSlabAlloc => {
      // No vars to collect — alloc takes no arguments
    }
    ExIndexGet => {
      let d = e!.index_get!
      collect_val_vars(d.collection, used)
      collect_val_vars(d.index, used)
    }
    ExWrapOptional => {
      let d = e!.wrap_opt!
      collect_val_vars(d.value, used)
    }
    ExMakeChannel => {
      let d = e!.make_channel!
      collect_val_vars(d.buf_size, used)
    }
    ExIsNull => {
      let d = e!.is_null!
      collect_val_vars(d.value, used)
    }
    ExUnwrapOptional => {
      let d = e!.unwrap_opt!
      collect_val_vars(d.value, used)
    }
    _ => {}
  }
}

func collect_used_vars_stmts(stmts: [LStmt?], used: Dict<Sym, bool>?) {
  let mut i = 0
  while i < len(stmts) {
    let s = stmts[i]
    if isnull(s) {
      i = i + 1
      continue
    }
    match s!.kind {
      StTempDef => {
        let d = s!.temp_def!
        collect_expr_vars(d.expr, used)
      }
      StVarDecl => {
        let d = s!.var_decl!
        collect_val_vars(d.init, used)
      }
      StAssign => {
        let d = s!.assign!
        used!.set(sym(d.target), true)
        collect_val_vars(d.value, used)
      }
      StSideEffect => {
        let d = s!.side_effect!
        collect_expr_vars(d.expr, used)
      }
      StSend => {
        let d = s!.send_data!
        collect_val_vars(d.channel, used)
        collect_val_vars(d.value, used)
      }
      StIf => {
        let d = s!.if_data!
        collect_used_vars_stmts(d.then_body, used)
        collect_used_vars_stmts(d.else_body, used)
      }
      StWhile => {
        let d = s!.while_data!
        collect_used_vars_stmts(d.cond_block, used)
        collect_used_vars_stmts(d.body, used)
      }
      StFor => {
        let d = s!.for_data!
        collect_val_vars(d.collection, used)
        collect_used_vars_stmts(d.body, used)
      }
      StBlock => {
        let d = s!.block!
        collect_used_vars_stmts(d.stmts, used)
      }
      StLock => {
        let d = s!.lock_data!
        collect_val_vars(d.mutex, used)
        collect_used_vars_stmts(d.body, used)
      }
      StReturn => {
        let d = s!.ret!
        let mut j = 0
        while j < len(d.values) {
          collect_val_vars(d.values[j], used)
          j = j + 1
        }
      }
      StStructSet => {
        let d = s!.struct_set!
        collect_val_vars(d.receiver, used)
        collect_val_vars(d.value, used)
      }
      StClassSet => {
        let d = s!.class_set!
        collect_val_vars(d.handle, used)
        collect_val_vars(d.value, used)
      }
      StSlabSet => {
        let d = s!.slab_set!
        collect_val_vars(d.handle, used)
        collect_val_vars(d.value, used)
      }
      StSlabFree => {
        let d = s!.slab_free!
        collect_val_vars(d.handle, used)
      }
      StSliceFree => {
        // slice_free references a variable by name — mark it as used
        let d = s!.slice_free!
        used.set(`d.name`, true)
      }
      StIndexSet => {
        let d = s!.index_set!
        collect_val_vars(d.collection, used)
        collect_val_vars(d.index, used)
        collect_val_vars(d.value, used)
      }
      _ => {}
    }
    i = i + 1
  }
}

func collect_used_vars(stmts: [LStmt?]) -> Dict<Sym, bool>? {
  let used = Dict<Sym, bool>()
  collect_used_vars_stmts(stmts, used)
  return used
}

func collect_declared_vars_stmts(stmts: [LStmt?], decl: Dict<Sym, bool>?) {
  let mut i = 0
  while i < len(stmts) {
    let s = stmts[i]
    if isnull(s) {
      i = i + 1
      continue
    }
    match s!.kind {
      StVarDecl => {
        let d = s!.var_decl!
        decl!.set(sym(d.name), true)
      }
      StIf => {
        let d = s!.if_data!
        collect_declared_vars_stmts(d.then_body, decl)
        collect_declared_vars_stmts(d.else_body, decl)
      }
      StWhile => {
        let d = s!.while_data!
        collect_declared_vars_stmts(d.body, decl)
      }
      StFor => {
        let d = s!.for_data!
        decl!.set(sym(d.var_name), true)
        collect_declared_vars_stmts(d.body, decl)
      }
      StBlock => {
        let d = s!.block!
        collect_declared_vars_stmts(d.stmts, decl)
      }
      StLock => {
        let d = s!.lock_data!
        collect_declared_vars_stmts(d.body, decl)
      }
      _ => {}
    }
    i = i + 1
  }
}

func collect_declared_vars(stmts: [LStmt?]) -> Dict<Sym, bool>? {
  let decl = Dict<Sym, bool>()
  collect_declared_vars_stmts(stmts, decl)
  return decl
}

// ---------------------------------------------------------------------------
// Top-level generation — EmitC entry point
// ---------------------------------------------------------------------------

pub func emit_c(prog: LProgram?) -> string {
  let g = new_cgen(prog)!

  g.line("/* Generated by Lyric compiler — C backend */")
  g.line("#include <stdio.h>")
  g.line("#include <stdlib.h>")
  g.line("#include <stdint.h>")
  g.line("#include <stdbool.h>")
  g.line("#include <string.h>")
  g.line("#include <stdarg.h>")
  g.line("#include <setjmp.h>")
  g.line("#include \"lyric_runtime.h\"")
  g.line("")
  // Test assertion macros
  g.line("static jmp_buf _lyric_test_jmp;")
  g.line("static int _lyric_test_failed;")
  g.line(f"#define lyric_assert(cond, msg, file, line) do {{ \\")
  g.line(f"    if (!(cond)) {{ \\")
  g.line("        fprintf(stderr, \"  assert failed at %s:%d\\n    %.*s\\n\", file, line, (int)(msg).len, (const char*)(msg).data); \\")
  g.line("        _lyric_test_failed = 1; \\")
  g.line("        longjmp(_lyric_test_jmp, 1); \\")
  g.line("    } \\")
  g.line("} while(0)")
  g.line(f"#define lyric_assert_eq(eq, actual_str, expected_str, msg, file, line) do {{ \\")
  g.line(f"    if (!(eq)) {{ \\")
  g.line("        lyric_string _a = (actual_str); \\")
  g.line("        lyric_string _e = (expected_str); \\")
  g.line("        fprintf(stderr, \"  assert_eq failed at %s:%d\\n    %.*s\\n    expected: %.*s\\n    got:      %.*s\\n\", \\")
  g.line("            file, line, (int)(msg).len, (const char*)(msg).data, \\")
  g.line("            (int)_e.len, (const char*)_e.data, (int)_a.len, (const char*)_a.data); \\")
  g.line("        _lyric_test_failed = 1; \\")
  g.line("        longjmp(_lyric_test_jmp, 1); \\")
  g.line("    } \\")
  g.line("} while(0)")
  g.line("")

  // Pre-scan composite types
  g.collect_composite_types()

  // Pre-scan function signatures to discover tuple types
  let pre_funcs = g.prog!.functions
  let mut pi = 0
  while pi < len(pre_funcs) {
    if len(pre_funcs[pi].type_params) == 0 {
      if !isnull(pre_funcs[pi].return_type) && pre_funcs[pi].return_type!.kind is TyTuple {
        g.c_tuple_type(pre_funcs[pi].return_type)
      }
      let mut pj = 0
      while pj < len(pre_funcs[pi].params) {
        if !isnull(pre_funcs[pi].params[pj].typ) && pre_funcs[pi].params[pj].typ!.kind is TyTuple {
          g.c_tuple_type(pre_funcs[pi].params[pj].typ)
        }
        pj = pj + 1
      }
    }
    pi = pi + 1
  }

  // Deduplicate
  let prog_ref = g.prog!
  prog_ref.structs = dedup_structs(prog_ref.structs)
  prog_ref.classes = dedup_classes(prog_ref.classes)
  prog_ref.enums = dedup_enums(prog_ref.enums)
  prog_ref.functions = dedup_funcs(prog_ref.functions)

  // Forward-declare structs, classes, enums
  let structs = prog_ref.structs
  let mut i = 0
  while i < len(structs) {
    g.line(f"typedef struct {structs[i].name} {structs[i].name};")
    i = i + 1
  }
  let classes = prog_ref.classes
  i = 0
  while i < len(classes) {
    if prog_ref.slab_mode_soa {
      g.line(f"typedef uint32_t {classes[i].name};")
    } else {
      g.line(f"typedef struct {classes[i].name} {classes[i].name};")
    }
    i = i + 1
  }
  let enums = prog_ref.enums
  i = 0
  while i < len(enums) {
    if is_simple_enum(enums[i]) {
      g.emit_simple_enum_decl(enums[i])
    } else {
      g.line(f"typedef struct {enums[i].name} {enums[i].name};")
    }
    i = i + 1
  }
  if len(structs) + len(classes) + len(enums) > 0 {
    g.line("")
  }

  // Emit slice typedefs
  let slice_keys = g.slice_types!.keys()
  i = 0
  while i < len(slice_keys) {
    let entry = g.slice_types!.get(slice_keys[i])
    if !isnull(entry) {
      let name = entry!.value
      if name != "LyricSlice_uint8_t" && name != "LyricSlice_lyric_string" {
        g.line(f"LYRIC_SLICE_DEF({slice_keys[i].get_name()}, {name})")
      }
    }
    i = i + 1
  }
  if len(slice_keys) > 0 {
    g.line("")
  }

  // Pre-collect optional types from struct/class fields
  i = 0
  while i < len(structs) {
    let mut j = 0
    while j < len(structs[i].fields) {
      let ft = structs[i].fields[j].typ
      if !isnull(ft) && ft!.kind is TyOptional {
        g.opt_type_name(ft!.elem)
      }
      j = j + 1
    }
    i = i + 1
  }
  i = 0
  while i < len(classes) {
    let mut j = 0
    while j < len(classes[i].fields) {
      let ft = classes[i].fields[j].typ
      if !isnull(ft) && ft!.kind is TyOptional {
        g.opt_type_name(ft!.elem)
      }
      j = j + 1
    }
    i = i + 1
  }

  // Emit enum definitions
  i = 0
  while i < len(enums) {
    if !is_simple_enum(enums[i]) {
      g.emit_enum_decl(enums[i])
    }
    i = i + 1
  }

  // Emit struct definitions (topo sorted)
  prog_ref.structs = topo_sort_structs(prog_ref.structs)
  let sorted_structs = prog_ref.structs
  i = 0
  while i < len(sorted_structs) {
    g.emit_struct_decl(sorted_structs[i])
    i = i + 1
  }

  // Emit tuple typedefs (after struct definitions so struct types are known)
  let tuple_keys = g.tuple_types!.keys()
  let mut tk = 0
  while tk < len(tuple_keys) {
    let tuple_entry = g.tuple_types!.get(tuple_keys[tk])
    if !isnull(tuple_entry) {
      let tname = tuple_entry!.value
      let field_types = str_split(tuple_keys[tk].get_name(), ",")
      g.line(f"typedef struct {tname} {{")
      g.indent = g.indent + 1
      let mut ti = 0
      while ti < len(field_types) {
        if field_types[ti] != "" {
          g.line(f"{field_types[ti]} _{ti};")
        }
        ti = ti + 1
      }
      // Empty struct needs a dummy field in C
      if len(field_types) == 0 || (len(field_types) == 1 && field_types[0] == "") {
        g.line("char _dummy;")
      }
      g.indent = g.indent - 1
      g.line(f"}} {tname};")
    }
    tk = tk + 1
  }
  if len(tuple_keys) > 0 {
    g.line("")
  }

  // Emit opt/result typedefs
  let opt_keys = g.opt_types!.keys()
  i = 0
  while i < len(opt_keys) {
    let entry = g.opt_types!.get(opt_keys[i])
    if !isnull(entry) {
      g.line(f"LYRIC_OPT_DEF({opt_keys[i].get_name()}, {entry!.value})")
    }
    i = i + 1
  }
  let result_keys = g.result_types!.keys()
  i = 0
  while i < len(result_keys) {
    let entry = g.result_types!.get(result_keys[i])
    if !isnull(entry) {
      g.line(f"LYRIC_RESULT_DEF({result_keys[i].get_name()}, {entry!.value})")
    }
    i = i + 1
  }
  if len(opt_keys) + len(result_keys) > 0 {
    g.line("")
  }

  // Emit class definitions
  i = 0
  while i < len(classes) {
    g.emit_class_decl(classes[i])
    i = i + 1
  }

  // Emit slab infrastructure (after class struct definitions)
  if prog_ref.slab_mode {
    g.emit_slab_infrastructure(classes)
  }

  // Emit channel type definitions
  let chan_keys = g.chan_types!.keys()
  i = 0
  while i < len(chan_keys) {
    let entry = g.chan_types!.get(chan_keys[i])
    if !isnull(entry) {
      g.line(f"LYRIC_CHAN_DEF({chan_keys[i].get_name()}, LyricChan_{entry!.value})")
    }
    i = i + 1
  }

  // Emit generator struct declarations
  let funcs = prog_ref.functions
  i = 0
  while i < len(funcs) {
    if g.is_generator_func(funcs[i]) {
      g.emit_generator_struct_decl(funcs[i])
    }
    i = i + 1
  }

  // Emit interface vtables
  let ifaces = prog_ref.interfaces
  i = 0
  while i < len(ifaces) {
    if len(ifaces[i].type_params) > 0 {
      i = i + 1
      continue
    }
    let if_name = ifaces[i].name
    g.line(f"typedef struct {if_name}_vtable {{")
    g.indent = g.indent + 1
    let mut j = 0
    while j < len(ifaces[i].methods) {
      let m = ifaces[i].methods[j]
      let ret_type = g.c_type(m.return_type)
      let sb = new_string_builder()
      sb.write("void*")
      let mut k = 0
      while k < len(m.params) {
        sb.write(", ")
        sb.write(g.c_type(m.params[k].typ))
        k = k + 1
      }
      g.line(f"{ret_type} (*{m.name})({sb.to_string()});")
      j = j + 1
    }
    g.indent = g.indent - 1
    g.line(f"}} {if_name}_vtable;")
    g.line(f"typedef struct {if_name} {{")
    g.indent = g.indent + 1
    g.line("void* _data;")
    g.line(f"const {if_name}_vtable* _vtable;")
    g.indent = g.indent - 1
    g.line(f"}} {if_name};")
    g.line("")
    i = i + 1
  }

  // Static vtable instances
  i = 0
  while i < len(classes) {
    let class_name = classes[i].name
    let mut j = 0
    while j < len(classes[i].implements) {
      let iface_name = classes[i].implements[j]
      let iface_entry = g.iface_by_name!.get(sym(iface_name))
      if !isnull(iface_entry) {
        g.line(f"static const {iface_name}_vtable {class_name}_as_{iface_name};")
      }
      j = j + 1
    }
    i = i + 1
  }

  // Type aliases
  let type_defs = prog_ref.type_defs
  i = 0
  while i < len(type_defs) {
    g.line(f"typedef {g.c_type(type_defs[i].typ)} {type_defs[i].name};")
    g.line("")
    i = i + 1
  }

  // Auto-generated to_string functions
  g.emit_to_string_functions()

  // Forward-declare all functions
  i = 0
  while i < len(funcs) {
    if g.func_name(funcs[i]) != "main" && len(funcs[i].type_params) == 0 {
      g.emit_func_forward_decl(funcs[i])
    }
    i = i + 1
  }
  if len(funcs) > 0 {
    g.line("")
  }

  // Emit globals
  let globals = prog_ref.globals
  i = 0
  while i < len(globals) {
    let c_t = g.c_type(globals[i].typ)
    if !isnull(globals[i].init) {
      g.line(f"static {c_t} {c_safe_name(globals[i].name)} = {g.emit_value(globals[i].init)};")
    } else {
      g.line(f"static {c_t} {c_safe_name(globals[i].name)};")
    }
    i = i + 1
  }
  if len(globals) > 0 {
    g.line("")
  }

  // Emit vtable definitions
  i = 0
  while i < len(classes) {
    let class_name = classes[i].name
    let mut j = 0
    while j < len(classes[i].implements) {
      let iface_name = classes[i].implements[j]
      let iface_entry = g.iface_by_name!.get(sym(iface_name))
      if !isnull(iface_entry) {
        let iface = iface_entry!.value
        g.line(f"static const {iface_name}_vtable {class_name}_as_{iface_name} = {{")
        g.indent = g.indent + 1
        let mut k = 0
        while k < len(iface.methods) {
          let m = iface.methods[k]
          let ret_type = g.c_type(m.return_type)
          let sb = new_string_builder()
          sb.write("void*")
          let mut p = 0
          while p < len(m.params) {
            sb.write(", ")
            sb.write(g.c_type(m.params[p].typ))
            p = p + 1
          }
          let cast_type = f"{ret_type}(*)({sb.to_string()})"
          g.line(f".{m.name} = ({cast_type}){class_name}_{m.name},")
          k = k + 1
        }
        g.indent = g.indent - 1
        g.line("};")
      }
      j = j + 1
    }
    i = i + 1
  }
  if len(classes) > 0 {
    g.line("")
  }

  // Emit function bodies — collect into separate buffer first for lambda hoisting
  let saved_buf = g.buf
  g.buf = new_string_builder()
  i = 0
  while i < len(funcs) {
    if len(funcs[i].type_params) == 0 {
      g.emit_func_decl(funcs[i])
    }
    i = i + 1
  }
  let func_code = g.buf!.to_string()
  g.buf = saved_buf

  // Emit hoisted lambdas
  i = 0
  while i < len(g.lambdas) {
    let lam = g.lambdas[i]
    g.line(f"static {lam.ret_type} {lam.name}({lam.params}) {{")
    g.write_raw(lam.body_str)
    g.line("}")
    g.line("")
    i = i + 1
  }

  // Emit channel implementations (before spawn funcs — spawn bodies may use channels)
  i = 0
  while i < len(chan_keys) {
    let entry = g.chan_types!.get(chan_keys[i])
    if !isnull(entry) {
      let chan_name = f"LyricChan_{entry!.value}"
      g.line(f"LYRIC_CHAN_IMPL({chan_keys[i].get_name()}, {chan_name}, {entry!.value})")
      g.line("")
    }
    i = i + 1
  }

  // Emit hoisted spawn wrapper functions
  i = 0
  while i < len(g.spawn_funcs) {
    let sf = g.spawn_funcs[i]
    if len(sf.captures) > 0 {
      g.line(f"typedef struct {sf.name}_ctx {{")
      g.indent = g.indent + 1
      let mut ci = 0
      while ci < len(sf.captures) {
        g.line(f"{sf.captures[ci].typ}* {sf.captures[ci].name};")
        ci = ci + 1
      }
      g.indent = g.indent - 1
      g.line(f"}} {sf.name}_ctx;")
    }
    g.line(f"static void* {sf.name}(void* _arg) {{")
    g.indent = g.indent + 1
    if len(sf.captures) > 0 {
      g.line(f"{sf.name}_ctx* _ctx = ({sf.name}_ctx*)_arg;")
    }
    g.write_raw(sf.body_str)
    if len(sf.captures) > 0 {
      g.line("free(_ctx);")
    }
    g.line("return NULL;")
    g.indent = g.indent - 1
    g.line("}")
    g.line("")
    i = i + 1
  }

  // Emit OS helpers
  g.emit_os_helpers()

  // Emit function bodies
  g.write_raw(func_code)

  return g.buf!.to_string()
}

// ---------------------------------------------------------------------------
// Test runner emission
// ---------------------------------------------------------------------------

pub func emit_test_runner(test_funcs: [string]) -> string {
  let sb = new_string_builder()
  sb.write("\n// --- Test runner (generated by lyric test) ---\n")
  sb.write(f"int main(int _argc, char** _argv) {{\n")
  sb.write("    int _passed = 0, _failed_count = 0, _total = 0;\n")
  sb.write(f"    struct {{ const char* name; void (*fn)(void); }} _tests[] = {{\n")
  let mut i = 0
  while i < len(test_funcs) {
    sb.write(f"        {{\"{test_funcs[i]}\", {test_funcs[i]}}},\n")
    i = i + 1
  }
  sb.write("    };\n")
  sb.write(f"    _total = {len(test_funcs)};\n")
  sb.write(f"    for (int _i = 0; _i < _total; _i++) {{\n")
  sb.write("        _lyric_test_failed = 0;\n")
  sb.write(f"        if (setjmp(_lyric_test_jmp) == 0) {{\n")
  sb.write("            _tests[_i].fn();\n")
  sb.write("        }\n")
  sb.write(f"        if (_lyric_test_failed) {{\n")
  sb.write("            printf(\"FAIL  %s\\n\", _tests[_i].name);\n")
  sb.write("            _failed_count++;\n")
  sb.write(f"        }} else {{\n")
  sb.write("            printf(\"PASS  %s\\n\", _tests[_i].name);\n")
  sb.write("            _passed++;\n")
  sb.write("        }\n")
  sb.write("    }\n")
  sb.write("    printf(\"\\n%d tests, %d passed, %d failed\\n\", _total, _passed, _failed_count);\n")
  sb.write("    return _failed_count > 0 ? 1 : 0;\n")
  sb.write("}\n")
  return sb.to_string()
}
