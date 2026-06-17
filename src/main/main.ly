// main.ly — Lyric Lyric compiler CLI
// Commands: compile, test, fmt, help
// Ports cmd/lyric/main.go (compile, test) and cmd/lyric/fmt.go

let USAGE: string = "Usage: lyric <command> [arguments]\n\nCommands:\n  compile  <file.ly> [...] [-o out]   Compile .ly files to C\n  test     <file.ly> [...]             Compile, discover test_* functions, run tests\n  fmt      <file.lyric> [...]          Format .lyric files\n  help                                Show this message\n"

// ---------------------------------------------------------------------------
// Command resolution — unique prefix matching
// ---------------------------------------------------------------------------

func resolve_command(prefix: string) -> (string, bool) {
  if prefix == "-h" || prefix == "--help" {
    return ("help", true)
  }
  let commands: [string] = ["compile", "test", "fmt", "help"]
  // Exact match first
  let mut i = 0
  while i < len(commands) {
    if commands[i] == prefix {
      return (commands[i], true)
    }
    i = i + 1
  }
  // Prefix match
  let mut matches: [string] = []
  i = 0
  while i < len(commands) {
    if len(prefix) <= len(commands[i]) {
      if commands[i][0:len(prefix)] == prefix {
        matches = append(matches, commands[i])
      }
    }
    i = i + 1
  }
  if len(matches) == 1 {
    return (matches[0], true)
  }
  if len(matches) > 1 {
    eprintln(f"ambiguous command \"{prefix}\": matches {str_join(matches, ", ")}")
  } else {
    eprintln(f"unknown command: {prefix}")
  }
  return ("", false)
}

func str_join(parts: [string], sep: string) -> string {
  let sb = new_string_builder()
  let mut i = 0
  while i < len(parts) {
    if i > 0 { sb.write(sep) }
    sb.write(parts[i])
    i = i + 1
  }
  return sb.to_string()
}

// ---------------------------------------------------------------------------
// Shared: parse, merge, desugar, check, lower pipeline
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// LIR dump — diagnostic tool for debugging pipeline issues
// Usage: lyric compile <files> --lir-dump /tmp/lir.txt
// ---------------------------------------------------------------------------

func dump_type_str(t: LType?) -> string {
  if isnull(t) { return "null" }
  let k = t!.kind
  if k is TyI8    { return "i8" }
  if k is TyI16   { return "i16" }
  if k is TyI32   { return "i32" }
  if k is TyI64   { return "i64" }
  if k is TyU8    { return "u8" }
  if k is TyU16   { return "u16" }
  if k is TyU32   { return "u32" }
  if k is TyU64   { return "u64" }
  if k is TyF32   { return "f32" }
  if k is TyF64   { return "f64" }
  if k is TyBool  { return "bool" }
  if k is TyString { return "string" }
  if k is TyUnit  { return "unit" }
  if k is TyError { return "error" }
  if k is TyAny   { return "any" }
  if k is TyPlatformInt  { return "int" }
  if k is TyPlatformUint { return "uint" }
  if k is TyMutex { return "lock" }
  if k is TyTypeVar { return f"TypeVar({t!.name})" }
  if k is TyStruct { return f"Struct({t!.name})" }
  if k is TyClassHandle {
    if len(t!.type_args) > 0 {
      return f"Class({t!.name}[{itoa(len(t!.type_args))} args])"
    }
    return f"Class({t!.name})"
  }
  if k is TyOptional {
    return f"Opt({dump_type_str(t!.elem)})"
  }
  if k is TySlice { return f"[{dump_type_str(t!.elem)}]" }
  if k is TyMap   { return f"map[{dump_type_str(t!.key)}]{dump_type_str(t!.elem)}" }
  if k is TyTuple { return f"tuple({itoa(len(t!.fields))} fields)" }
  if k is TyTaggedUnion { return f"Enum({t!.name})" }
  if k is TyChannel   { return f"chan({dump_type_str(t!.elem)})" }
  if k is TyGenerator { return f"gen({dump_type_str(t!.elem)})" }
  if k is TyFuncPtr   { return f"fn({itoa(len(t!.params))} params)" }
  if k is TyErrorResult { return f"result({dump_type_str(t!.elem)})" }
  if k is TyUnion { return f"union({itoa(len(t!.variants))} variants)" }
  return f"LType({t!.name})"
}

func dump_value_str(v: LValue?) -> string {
  if isnull(v) { return "null" }
  let k = v!.kind
  if k is ValVar    { return f"var({v!.name})" }
  if k is ValTemp   { return f"%{itoa(v!.temp_id)}" }
  if k is ValGlobal { return f"global({v!.name})" }
  if k is ValLitInt    { return f"int({itoa(v!.int_val as i32)})" }
  if k is ValLitUint   { return f"uint({itoa(v!.uint_val as i32)})" }
  if k is ValLitFloat  { return f"float({v!.float_val as i64})" }
  if k is ValLitString { return f"str(\"{v!.str_val}\")" }
  if k is ValLitBool   {
    if v!.bool_val { return "true" } else { return "false" }
  }
  if k is ValLitNull   { return "null" }
  if k is ValIndexRef  { return f"idx({dump_value_str(v!.collection)}[{dump_value_str(v!.index)}])" }
  if k is ValClassFieldRef { return f"fieldref({dump_value_str(v!.collection)}.{v!.name})" }
  return "LValue(?)"
}

func dump_expr_to_sb(sb: StringBuilder, e: LExpr?) {
  if isnull(e) {
    sb.write("null")
    return
  }
  let k = e!.kind
  sb.write(f"typ={dump_type_str(e!.typ)} ")
  if k is ExBinOp {
    if !isnull(e!.bin_op) {
      sb.write(f"ExBinOp left={dump_value_str(e!.bin_op!.left)} right={dump_value_str(e!.bin_op!.right)}")
    }
  } else if k is ExUnOp {
    if !isnull(e!.un_op) {
      sb.write(f"ExUnOp operand={dump_value_str(e!.un_op!.operand)}")
    }
  } else if k is ExCast {
    if !isnull(e!.cast) {
      sb.write(f"ExCast target={dump_type_str(e!.cast!.target)} operand={dump_value_str(e!.cast!.operand)}")
    }
  } else if k is ExStructField {
    if !isnull(e!.struct_field) {
      sb.write(f"ExStructField recv={dump_value_str(e!.struct_field!.receiver)} .{e!.struct_field!.field}")
    }
  } else if k is ExClassGet {
    if !isnull(e!.class_get) {
      sb.write(f"ExClassGet class={e!.class_get!.class_name} handle={dump_value_str(e!.class_get!.handle)} .{e!.class_get!.field}")
    }
  } else if k is ExIndexGet {
    if !isnull(e!.index_get) {
      sb.write(f"ExIndexGet coll={dump_value_str(e!.index_get!.collection)} idx={dump_value_str(e!.index_get!.index)}")
    }
  } else if k is ExSlice {
    if !isnull(e!.slice_data) {
      sb.write(f"ExSlice coll={dump_value_str(e!.slice_data!.collection)} lo={dump_value_str(e!.slice_data!.low)} hi={dump_value_str(e!.slice_data!.high)}")
    }
  } else if k is ExCall {
    if !isnull(e!.call) {
      sb.write(f"ExCall {e!.call!.func_name}(")
      let mut j = 0
      while j < len(e!.call!.args) {
        if j > 0 { sb.write(", ") }
        sb.write(dump_value_str(e!.call!.args[j]))
        j = j + 1
      }
      sb.write(")")
      if len(e!.call!.type_args) > 0 {
        sb.write(f" type_args=[{itoa(len(e!.call!.type_args))}]")
      }
    }
  } else if k is ExMethodCall {
    if !isnull(e!.method_call) {
      sb.write(f"ExMethodCall recv={dump_value_str(e!.method_call!.receiver)}.{e!.method_call!.method}(")
      let mut j = 0
      while j < len(e!.method_call!.args) {
        if j > 0 { sb.write(", ") }
        sb.write(dump_value_str(e!.method_call!.args[j]))
        j = j + 1
      }
      sb.write(")")
      if len(e!.method_call!.type_args) > 0 {
        sb.write(f" type_args=[{itoa(len(e!.method_call!.type_args))}]")
      }
    }
  } else if k is ExBuiltin {
    if !isnull(e!.builtin) {
      sb.write(f"ExBuiltin {e!.builtin!.name}(")
      let mut j = 0
      while j < len(e!.builtin!.args) {
        if j > 0 { sb.write(", ") }
        sb.write(dump_value_str(e!.builtin!.args[j]))
        j = j + 1
      }
      sb.write(")")
    }
  } else if k is ExStructLit {
    if !isnull(e!.struct_lit) {
      sb.write(f"ExStructLit fields=[")
      let mut j = 0
      while j < len(e!.struct_lit!.fields) {
        if j > 0 { sb.write(", ") }
        sb.write(f"{e!.struct_lit!.fields[j].name}={dump_value_str(e!.struct_lit!.fields[j].value)}")
        j = j + 1
      }
      sb.write("]")
    }
  } else if k is ExClassAlloc {
    if !isnull(e!.class_alloc) {
      let ca = e!.class_alloc!
      sb.write(f"ExClassAlloc({ca.class_name}) type_args=[{itoa(len(ca.type_args))}] fields=[")
      let mut j = 0
      while j < len(ca.fields) {
        if j > 0 { sb.write(", ") }
        sb.write(f"{ca.fields[j].name}={dump_value_str(ca.fields[j].value)}")
        j = j + 1
      }
      sb.write("]")
      // Dump each type_arg
      if len(ca.type_args) > 0 {
        sb.write(" type_args_detail=[")
        let mut j2 = 0
        while j2 < len(ca.type_args) {
          if j2 > 0 { sb.write(", ") }
          sb.write(dump_type_str(ca.type_args[j2]))
          j2 = j2 + 1
        }
        sb.write("]")
      }
    }
  } else if k is ExMakeSlice {
    sb.write(f"ExMakeSlice typ={dump_type_str(e!.typ)}")
  } else if k is ExMakeMap {
    sb.write(f"ExMakeMap typ={dump_type_str(e!.typ)}")
  } else if k is ExMakeChannel {
    if !isnull(e!.make_channel) {
      sb.write(f"ExMakeChannel elem={dump_type_str(e!.make_channel!.elem_type)} buf={dump_value_str(e!.make_channel!.buf_size)}")
    }
  } else if k is ExWrapOptional {
    if !isnull(e!.wrap_opt) {
      sb.write(f"ExWrapOptional value={dump_value_str(e!.wrap_opt!.value)}")
    }
  } else if k is ExUnwrapOptional {
    if !isnull(e!.unwrap_opt) {
      sb.write(f"ExUnwrapOptional value={dump_value_str(e!.unwrap_opt!.value)}")
    }
  } else if k is ExIsNull {
    if !isnull(e!.is_null) {
      sb.write(f"ExIsNull value={dump_value_str(e!.is_null!.value)}")
    }
  } else if k is ExVariantConstruct {
    if !isnull(e!.variant_construct) {
      let vc = e!.variant_construct!
      sb.write(f"ExVariantConstruct {vc.enum_name}.{vc.variant} tag={itoa(vc.tag)} fields=[")
      let mut j = 0
      while j < len(vc.fields) {
        if j > 0 { sb.write(", ") }
        sb.write(dump_value_str(vc.fields[j]))
        j = j + 1
      }
      sb.write("]")
    }
  } else if k is ExVariantTag {
    if !isnull(e!.variant_tag) {
      sb.write(f"ExVariantTag value={dump_value_str(e!.variant_tag!.value)}")
    }
  } else if k is ExVariantData {
    if !isnull(e!.variant_data) {
      let vd = e!.variant_data!
      sb.write(f"ExVariantData enum={vd.enum_name} variant={vd.variant} field={vd.field} value={dump_value_str(vd.value)}")
    }
  } else if k is ExExtractValue {
    if !isnull(e!.extract_value) {
      sb.write(f"ExExtractValue value={dump_value_str(e!.extract_value!.value)}")
    }
  } else if k is ExExtractError {
    if !isnull(e!.extract_error) {
      sb.write(f"ExExtractError value={dump_value_str(e!.extract_error!.value)}")
    }
  } else if k is ExMakeResult {
    if !isnull(e!.make_result) {
      sb.write(f"ExMakeResult value={dump_value_str(e!.make_result!.value)} err={dump_value_str(e!.make_result!.err)}")
    }
  } else if k is ExEnvGet {
    if !isnull(e!.env_get) {
      sb.write(f"ExEnvGet env={dump_value_str(e!.env_get!.env)} .{e!.env_get!.field}")
    }
  } else if k is ExFuncRef {
    if !isnull(e!.func_ref) {
      sb.write(f"ExFuncRef {e!.func_ref!.name}")
    }
  } else if k is ExFuncLit {
    sb.write("ExFuncLit(...)")
  } else if k is ExFormat {
    if !isnull(e!.format) {
      sb.write(f"ExFormat({itoa(len(e!.format!.parts))} parts)")
    }
  } else if k is ExSlabGet {
    if !isnull(e!.slab_get) {
      sb.write(f"ExSlabGet class={e!.slab_get!.class_name} handle={dump_value_str(e!.slab_get!.handle)} .{e!.slab_get!.field}")
    }
  } else if k is ExSlabAlloc {
    if !isnull(e!.slab_alloc) {
      let sa = e!.slab_alloc!
      sb.write(f"ExSlabAlloc({sa.class_name}) fields=[")
      let mut j = 0
      while j < len(sa.fields) {
        if j > 0 { sb.write(", ") }
        sb.write(f"{sa.fields[j].name}={dump_value_str(sa.fields[j].value)}")
        j = j + 1
      }
      sb.write("]")
    }
  } else {
    sb.write(f"LExpr(?)")
  }
}

func dump_stmts(sb: StringBuilder, stmts: [LStmt?], indent: string) {
  let mut i = 0
  while i < len(stmts) {
    if isnull(stmts[i]) {
      sb.write(f"{indent}(null stmt)\n")
      i = i + 1
      continue
    }
    let s = stmts[i]!
    let k = s.kind
    if k is StTempDef {
      if !isnull(s.temp_def) {
        let td = s.temp_def!
        sb.write(f"{indent}%{itoa(td.id)} = ")
        dump_expr_to_sb(sb, td.expr)
        sb.write("\n")
      }
    } else if k is StVarDecl {
      if !isnull(s.var_decl) {
        let vd = s.var_decl!
        let mut_str = if vd.mutable { "mut " } else { "" }
        sb.write(f"{indent}VarDecl {mut_str}{vd.name}: {dump_type_str(vd.typ)} = {dump_value_str(vd.init)}\n")
      }
    } else if k is StAssign {
      if !isnull(s.assign) {
        sb.write(f"{indent}Assign {s.assign!.target} = {dump_value_str(s.assign!.value)}\n")
      }
    } else if k is StStructSet {
      if !isnull(s.struct_set) {
        let ss = s.struct_set!
        sb.write(f"{indent}StructSet {dump_value_str(ss.receiver)}.{ss.field} = {dump_value_str(ss.value)}\n")
      }
    } else if k is StClassSet {
      if !isnull(s.class_set) {
        let cs = s.class_set!
        sb.write(f"{indent}ClassSet {dump_value_str(cs.handle)}({cs.class_name}).{cs.field} = {dump_value_str(cs.value)}\n")
      }
    } else if k is StIndexSet {
      if !isnull(s.index_set) {
        let is2 = s.index_set!
        let fld = if is2.field != "" { f".{is2.field}" } else { "" }
        sb.write(f"{indent}IndexSet {dump_value_str(is2.collection)}[{dump_value_str(is2.index)}]{fld} = {dump_value_str(is2.value)}\n")
      }
    } else if k is StIf {
      if !isnull(s.if_data) {
        let id = s.if_data!
        sb.write(f"{indent}If cond={dump_value_str(id.cond)} {{\n")
        dump_stmts(sb, id.then_body, f"{indent}  ")
        if len(id.else_body) > 0 {
          sb.write(f"{indent}}} else {{\n")
          dump_stmts(sb, id.else_body, f"{indent}  ")
        }
        sb.write(f"{indent}}}\n")
      }
    } else if k is StWhile {
      if !isnull(s.while_data) {
        let wd = s.while_data!
        sb.write(f"{indent}While cond={dump_value_str(wd.cond_var)} {{\n")
        dump_stmts(sb, wd.body, f"{indent}  ")
        sb.write(f"{indent}}}\n")
      }
    } else if k is StFor {
      if !isnull(s.for_data) {
        let fd = s.for_data!
        sb.write(f"{indent}For {fd.var_name} in {dump_value_str(fd.collection)} {{\n")
        dump_stmts(sb, fd.body, f"{indent}  ")
        sb.write(f"{indent}}}\n")
      }
    } else if k is StSwitch {
      if !isnull(s.switch_data) {
        let sd = s.switch_data!
        sb.write(f"{indent}Switch tag={dump_value_str(sd.tag)} enum={sd.enum_name} [{itoa(len(sd.cases))} cases]\n")
      }
    } else if k is StBlock {
      if !isnull(s.block) {
        sb.write(f"{indent}Block {{\n")
        dump_stmts(sb, s.block!.stmts, f"{indent}  ")
        sb.write(f"{indent}}}\n")
      }
    } else if k is StReturn {
      if !isnull(s.ret) {
        sb.write(f"{indent}Return [")
        let mut j = 0
        while j < len(s.ret!.values) {
          if j > 0 { sb.write(", ") }
          sb.write(dump_value_str(s.ret!.values[j]))
          j = j + 1
        }
        sb.write("]\n")
      }
    } else if k is StBreak {
      sb.write(f"{indent}Break\n")
    } else if k is StContinue {
      sb.write(f"{indent}Continue\n")
    } else if k is StExpr {
      if !isnull(s.expr_stmt) {
        sb.write(f"{indent}ExprStmt %{itoa(s.expr_stmt!.temp_id)}\n")
      }
    } else if k is StDefer {
      if !isnull(s.defer_data) {
        sb.write(f"{indent}Defer {{\n")
        dump_stmts(sb, s.defer_data!.body, f"{indent}  ")
        sb.write(f"{indent}}}\n")
      }
    } else if k is StSpawn {
      sb.write(f"{indent}Spawn {{ ... }}\n")
    } else if k is StLock {
      if !isnull(s.lock_data) {
        sb.write(f"{indent}Lock mutex={dump_value_str(s.lock_data!.mutex)} {{\n")
        dump_stmts(sb, s.lock_data!.body, f"{indent}  ")
        sb.write(f"{indent}}}\n")
      }
    } else if k is StSend {
      if !isnull(s.send_data) {
        sb.write(f"{indent}Send ch={dump_value_str(s.send_data!.channel)} value={dump_value_str(s.send_data!.value)}\n")
      }
    } else if k is StSelect {
      sb.write(f"{indent}Select {{ ... }}\n")
    } else if k is StYield {
      if !isnull(s.yield_data) {
        sb.write(f"{indent}Yield {dump_value_str(s.yield_data!.value)}\n")
      }
    } else if k is StSideEffect {
      if !isnull(s.side_effect) {
        sb.write(f"{indent}SideEffect: ")
        dump_expr_to_sb(sb, s.side_effect!.expr)
        sb.write("\n")
      }
    } else if k is StMultiAssign {
      if !isnull(s.multi_assign) {
        let ma = s.multi_assign!
        sb.write(f"{indent}MultiAssign names=[")
        let mut j = 0
        while j < len(ma.names) {
          if j > 0 { sb.write(", ") }
          sb.write(ma.names[j])
          j = j + 1
        }
        sb.write("] = ")
        dump_expr_to_sb(sb, ma.expr)
        sb.write("\n")
      }
    } else if k is StTypeSwitch {
      sb.write(f"{indent}TypeSwitch {{ ... }}\n")
    } else if k is StSlabSet {
      if !isnull(s.slab_set) {
        let ss = s.slab_set!
        sb.write(f"{indent}SlabSet {dump_value_str(ss.handle)}({ss.class_name}).{ss.field} = {dump_value_str(ss.value)}\n")
      }
    } else if k is StSlabFree {
      if !isnull(s.slab_free) {
        sb.write(f"{indent}SlabFree {dump_value_str(s.slab_free!.handle)} ({s.slab_free!.class_name})\n")
      }
    } else if k is StSliceFree {
      if !isnull(s.slice_free) {
        sb.write(f"{indent}SliceFree {s.slice_free!.name}\n")
      }
    } else {
      sb.write(f"{indent}LStmt(?)\n")
    }
    i = i + 1
  }
}

func dump_lir_func(sb: StringBuilder, f: LFuncDecl) {
  // Function signature
  let recv = if f.receiver != "" { f"{f.receiver}." } else { "" }
  sb.write(f"\nfunc {recv}{f.name}(")
  let mut i = 0
  while i < len(f.params) {
    if i > 0 { sb.write(", ") }
    let mut_str = if f.params[i].mutable { "mut " } else { "" }
    sb.write(f"{mut_str}{f.params[i].name}: {dump_type_str(f.params[i].typ)}")
    i = i + 1
  }
  sb.write(")")
  if !isnull(f.return_type) {
    sb.write(f" -> {dump_type_str(f.return_type)}")
  }
  sb.write("\n")
  dump_stmts(sb, f.body, "  ")
}

func dump_lir_to_file(prog: LProgram, path: string) {
  let sb = new_string_builder()
  sb.write(f"=== LIR Dump: {prog.package_name} ===\n")
  sb.write(f"functions: {itoa(len(prog.functions))}\n")
  sb.write(f"structs: {itoa(len(prog.structs))}\n")
  sb.write(f"classes: {itoa(len(prog.classes))}\n")
  sb.write(f"enums: {itoa(len(prog.enums))}\n")

  // Dump all struct decls
  sb.write("\n--- Struct Decls ---\n")
  let mut i = 0
  while i < len(prog.structs) {
    let s = prog.structs[i]
    sb.write(f"struct {s.name} fields=[")
    let mut j = 0
    while j < len(s.fields) {
      if j > 0 { sb.write(", ") }
      sb.write(f"{s.fields[j].name}: {dump_type_str(s.fields[j].typ)}")
      j = j + 1
    }
    sb.write("]\n")
    i = i + 1
  }

  // Dump all class decls
  sb.write("\n--- Class Decls ---\n")
  i = 0
  while i < len(prog.classes) {
    let c = prog.classes[i]
    sb.write(f"class {c.name} fields=[")
    let mut j = 0
    while j < len(c.fields) {
      if j > 0 { sb.write(", ") }
      sb.write(f"{c.fields[j].name}: {dump_type_str(c.fields[j].typ)}")
      j = j + 1
    }
    sb.write("]\n")
    i = i + 1
  }

  // Dump all enum decls
  sb.write("\n--- Enum Decls ---\n")
  i = 0
  while i < len(prog.enums) {
    let e = prog.enums[i]
    sb.write(f"enum {e.name} variants=[")
    let mut j = 0
    while j < len(e.variants) {
      if j > 0 { sb.write(", ") }
      sb.write(f"{e.variants[j].name}({itoa(e.variants[j].tag)})")
      j = j + 1
    }
    sb.write("]\n")
    i = i + 1
  }

  // Dump all functions
  sb.write("\n--- Functions ---\n")
  i = 0
  while i < len(prog.functions) {
    dump_lir_func(sb, prog.functions[i])
    i = i + 1
  }

  let ok = write_file(path, sb.to_string())
  if ok {
    eprintln(f"LIR dump written to {path}")
  } else {
    eprintln(f"error: cannot write LIR dump to {path}")
  }
}

// ---------------------------------------------------------------------------
// compile_pipeline — shared parse/check/lower/emit pipeline
// ---------------------------------------------------------------------------

func compile_pipeline(inputs: [string], output: string, module_root: string, lir_dump: string, soa: bool, detect_uaf: bool, rc_free: bool) -> bool {
  // Parse all input files
  let mut all_files: [File?] = []
  let mut i = 0
  while i < len(inputs) {
    let result = read_file(inputs[i])
    if !result._1 {
      eprintln(f"error: cannot read {inputs[i]}")
      return false
    }
    let parse_result = parse_file(result._0, inputs[i])
    if isnull(parse_result._0) {
      eprintln(parse_result._1)
      return false
    }
    all_files = append(all_files, parse_result._0)
    i = i + 1
  }

  // Merge all files
  let mut merged = merge_files(all_files)
  if isnull(merged) {
    eprintln("error: no files to compile")
    return false
  }

  // Resolve module imports if we're in a module
  if module_root != "" {
    let resolve_result = resolve_module_imports(module_root, merged)
    merged = resolve_result._0
    let resolve_err = resolve_result._1
    if resolve_err != "" {
      eprintln(f"error: {resolve_err}")
      return false
    }
  }

  // Merge stdlib
  let stdlib_dir = find_stdlib_dir()
  if stdlib_dir != "" {
    let std_file = load_stdlib(stdlib_dir)
    if !isnull(std_file) {
      merge_stdlib(merged!, std_file!)
    }
  }

  // Desugar
  desugar_all(merged!)
  eprintln("phase: check")
  let checker = check_file(merged!)
  if len(checker.errors) > 0 {
    let mut j = 0
    while j < len(checker.errors) {
      eprintln(checker.errors[j])
      j = j + 1
    }
    return false
  }

  eprintln("phase: lower")
  let lowerer = new_lowerer()
  let prog = lowerer.lower_file(merged)
  if isnull(prog) {
    eprintln("error: lowering failed")
    return false
  }
  prog!.resolve_class_types()
  prog!.resolve_class_types()

  if lir_dump != "" {
    dump_lir_to_file(prog!, lir_dump)
  }

  eprintln("phase: optimize")
  optimize(prog!)
  eprintln("phase: mono")
  monomorphize(prog)
  prog!.resolve_class_types()
  eprintln("phase: validate")
  validate_post_mono(prog)
  eprintln("phase: rewrite")
  rewrite_impl_renames(prog)
  eprintln("phase: slab")
  if soa {
    prog!.slab_mode_soa = true
  }
  if detect_uaf {
    prog!.detect_uaf = true
  }
  if rc_free {
    prog!.rc_free = true
  }
  slab_rewrite(prog!)


  let c_src = emit_c(prog)
  eprintln("phase: write")

  // Write output
  let ok = write_file(output, c_src)
  if !ok {
    eprintln(f"error: cannot write {output}")
    return false
  }
  println(f"wrote {output}")
  return true
}

// ---------------------------------------------------------------------------
// compile command
// ---------------------------------------------------------------------------

func cmd_compile(args: [string]) -> bool {
  let mut inputs: [string] = []
  let mut output = ""
  let mut lir_dump = ""
  let mut soa = false
  let mut detect_uaf = false
  let mut rc_free = false
  let mut i = 0  while i < len(args) {
    if args[i] == "-o" {
      i = i + 1
      if i < len(args) {
        output = args[i]
      }
    } else if args[i] == "--lir-dump" {
      i = i + 1
      if i < len(args) {
        lir_dump = args[i]
      }
    } else if args[i] == "--c" {
      // accepted for backwards compat
    } else if args[i] == "--soa" {
      soa = true
    } else if args[i] == "--detect-uaf" {
      detect_uaf = true
    } else if args[i] == "--rc-free" {
      rc_free = true
    } else {
      inputs = append(inputs, args[i])
    }
    i = i + 1
  }

  if len(inputs) == 0 {
    eprintln("usage: lyric compile <file.ly|dir> [...] [-o output.c]")
    return false
  }

  // Check if input is a directory with lyric.mod (module mode)
  let mut module_root = ""
  if len(inputs) == 1 {
    // Check if it's a directory
    let mod_path = path_join([inputs[0], "lyric.mod"])
    if file_exists(mod_path) {
      module_root = inputs[0]
      // Replace inputs with all .ly files in the directory
      let dir_result = list_dir(module_root)
      if len(dir_result) == 0 {
        eprintln(f"error: cannot read module directory {module_root}")
        return false
      }
      inputs = []
      let entries = dir_result
      for j in range(0, len(entries)) {
        let entry = entries[j]
        if len(entry) > 3 && entry[len(entry)-3:len(entry)] == ".ly" {
          inputs = append(inputs, path_join([module_root, entry]))
        }
      }
      if len(inputs) == 0 {
        eprintln(f"error: no .ly files in module directory {module_root}")
        return false
      }
    } else {
      // Single file — check if parent has lyric.mod
      let dir = path_dir(inputs[0])
      let parent_mod = path_join([dir, "lyric.mod"])
      if file_exists(parent_mod) {
        module_root = dir
      }
    }
  }

  if output == "" {
    // Default: first input with .c extension
    let base = path_base(inputs[0])
    let ext = path_ext(base)
    if ext != "" {
      output = base[0:len(base) - len(ext)] + ".c"
    } else {
      output = base + ".c"
    }
  }

  return compile_pipeline(inputs, output, module_root, lir_dump, soa, detect_uaf, rc_free)
}

// ---------------------------------------------------------------------------
// test command
// ---------------------------------------------------------------------------

func cmd_test(args: [string]) -> bool {
  let mut inputs: [string] = []
  let mut output = ""
  let mut lir_dump = ""
  let mut soa = false
  let mut detect_uaf = false
  let mut rc_free = false
  let mut i = 0
  while i < len(args) {
    if args[i] == "-o" {
      i = i + 1
      if i < len(args) {
        output = args[i]
      }
    } else if args[i] == "--lir-dump" {
      i = i + 1
      if i < len(args) {
        lir_dump = args[i]
      }
    } else if args[i] == "--soa" {
      soa = true
    } else if args[i] == "--detect-uaf" {
      detect_uaf = true
    } else if args[i] == "--rc-free" {
      rc_free = true
    } else {
      inputs = append(inputs, args[i])
    }
    i = i + 1
  }

  if len(inputs) == 0 {
    eprintln("usage: lyric test <file.ly> [...]")
    return false
  }

  // Parse all input files
  let mut all_files: [File?] = []
  i = 0
  while i < len(inputs) {
    let result = read_file(inputs[i])
    if !result._1 {
      eprintln(f"error: cannot read {inputs[i]}")
      return false
    }
    let parse_result = parse_file(result._0, inputs[i])
    if isnull(parse_result._0) {
      eprintln(parse_result._1)
      return false
    }
    all_files = append(all_files, parse_result._0)
    i = i + 1
  }

  // Merge
  let merged = merge_files(all_files)
  if isnull(merged) {
    eprintln("error: no files to compile")
    return false
  }

  // Stdlib
  let stdlib_dir = find_stdlib_dir()
  if stdlib_dir != "" {
    let std_file = load_stdlib(stdlib_dir)
    if !isnull(std_file) {
      merge_stdlib(merged!, std_file!)
    }
  }

  // Desugar
  desugar_all(merged!)

  let checker = check_file(merged!)
  if len(checker.errors) > 0 {
    let mut j = 0
    while j < len(checker.errors) {
      eprintln(checker.errors[j])
      j = j + 1
    }
    return false
  }

  // Lower
  let lowerer = new_lowerer()
  let prog = lowerer.lower_file(merged)
  if isnull(prog) {
    eprintln("error: lowering failed")
    return false
  }

  if lir_dump != "" {
    dump_lir_to_file(prog!, lir_dump)
  }

  // Optimize + mono
  optimize(prog!)
  monomorphize(prog)
  prog!.resolve_class_types()
  validate_post_mono(prog)
  rewrite_impl_renames(prog)
  if soa {
    prog!.slab_mode_soa = true
  }
  if detect_uaf {
    prog!.detect_uaf = true
  }
  if rc_free {
    prog!.rc_free = true
  }
  slab_rewrite(prog!)

  // Discover test_* functions
  let mut test_funcs: [string] = []
  i = 0
  while i < len(prog!.functions) {
    let f = prog!.functions[i]
    if f.receiver == "" {
      if len(f.name) >= 5 {
        if f.name[0:5] == "test_" {
          test_funcs = append(test_funcs, f.name)
        }
      }
    }
    i = i + 1
  }

  if len(test_funcs) == 0 {
    eprintln("no test_* functions found")
    return true
  }

  // Generate C source + test runner
  let c_src = emit_c(prog) + emit_test_runner(test_funcs)

  // If -o was provided, just write the C file and exit
  if output != "" {
    if !write_file(output, c_src) {
      eprintln(f"error: cannot write {output}")
      return false
    }
    println(f"wrote {output}")
    return true
  }

  // Write to temp dir
  let tmp_dir = mkdtemp("lyric-test")
  let c_file = path_join([tmp_dir, "test.c"])
  let bin_file = path_join([tmp_dir, "test"])

  if !write_file(c_file, c_src) {
    eprintln(f"error: cannot write {c_file}")
    return false
  }

  // Copy runtime header
  let runtime_dir = find_runtime_dir()
  if runtime_dir != "" {
    let rt_result = read_file(path_join([runtime_dir, "lyric_runtime.h"]))
    if rt_result._1 {
      write_file(path_join([tmp_dir, "lyric_runtime.h"]), rt_result._0)
    }
  }

  // Compile with gcc
  let gcc_result = exec_command("gcc", [
    "-std=gnu11", "-O0", "-g",
    "-o", bin_file,
    c_file,
    "-I", tmp_dir
  ])
  if !gcc_result._1 {
    eprintln(f"gcc compilation failed:\n{gcc_result._0}")
    return false
  }

  // Run tests
  let empty_args: [string] = []
  let test_result = exec_command(bin_file, empty_args)
  print(test_result._0)
  return test_result._1
}

func find_runtime_dir() -> string {
  let candidates: [string] = ["runtime", "../runtime", "../../runtime"]
  let mut i = 0
  while i < len(candidates) {
    if file_exists(path_join([candidates[i], "lyric_runtime.h"])) {
      return candidates[i]
    }
    i = i + 1
  }
  return ""
}

// ---------------------------------------------------------------------------
// fmt command — format .lyric design files
// ---------------------------------------------------------------------------

func cmd_fmt(args: [string]) -> bool {
  if len(args) == 0 {
    eprintln("usage: lyric fmt <file.lyric> [...]")
    return false
  }
  let mut i = 0
  while i < len(args) {
    if !fmt_file(args[i]) {
      return false
    }
    println(f"formatted {args[i]}")
    i = i + 1
  }
  return true
}

func fmt_file(path: string) -> bool {
  let result = read_file(path)
  if !result._1 {
    eprintln(f"error: cannot read {path}")
    return false
  }

  let split = split_at_marker(result._0, "// --- index ---")
  let zone1 = split._0
  let zones23 = split._1

  let parse_result = parse_file(zone1, path)
  if isnull(parse_result._0) {
    eprintln(parse_result._1)
    return false
  }
  let file = parse_result._0
  if isnull(file) { return false }

  let sb = new_string_builder()
  let comments = file!.fc_children()
  let mut comment_idx = 0
  let blocks = file!.fb_children()

  let mut bi = 0
  while bi < len(blocks) {
    comment_idx = emit_comments_before(sb, comments, comment_idx, blocks[bi].span.start.line, "")
    fmt_block(sb, blocks[bi], comments, comment_idx)
    // Update comment_idx after fmt_block (we can't return it from fmt_block easily,
    // so we advance past all comments consumed by the block)
    while comment_idx < len(comments) {
      if comments[comment_idx].pos.line <= blocks[bi].span.end.line {
        comment_idx = comment_idx + 1
      } else {
        break
      }
    }
    if bi < len(blocks) - 1 {
      sb.write("\n")
    }
    bi = bi + 1
  }

  // Trailing comments
  while comment_idx < len(comments) {
    sb.write(comments[comment_idx].text)
    sb.write("\n")
    comment_idx = comment_idx + 1
  }

  // Append zones 2+3
  if zones23 != "" {
    sb.write(zones23)
  }

  return write_file(path, sb.to_string())
}

func split_at_marker(text: string, marker: string) -> (string, string) {
  let idx = str_index_of(text, marker)
  if idx < 0 {
    return (text, "")
  }
  return (text[0:idx], text[idx:len(text)])
}

func emit_comments_before(
    sb: StringBuilder,
    comments: [Comment],
    idx: i32,
    before_line: i32,
    indent: string
) -> i32 {
  let mut i = idx
  while i < len(comments) {
    if comments[i].pos.line >= before_line { break }
    sb.write(indent)
    sb.write(comments[i].text)
    sb.write("\n")
    i = i + 1
  }
  return i
}

// ---------------------------------------------------------------------------
// Lyric block and declaration formatters
// ---------------------------------------------------------------------------

func fmt_block(sb: StringBuilder, block: LyricBlock, comments: [Comment], comment_idx: i32) {
  let name = if !isnull(block.name) { block.name!.get_name() } else { "unnamed" }
  sb.write(f"lyric {name} {{\n")

  // Collect all declarations with source line for ordered emission
  let imports = block.imp_children()
  let structs = block.sd_children()
  let enums = block.ed_children()
  let interfaces = block.id_children()
  let classes = block.cd_children()
  let funcs = block.fd_children()
  let relations = block.rd_children()
  let type_aliases = block.ta_children()

  let mut items: [DeclItem] = []

  let mut i = 0
  while i < len(imports) {
    items = append(items, DeclItem { line: imports[i].span.start.line, tag: 0, idx: i })
    i = i + 1
  }
  i = 0
  while i < len(structs) {
    items = append(items, DeclItem { line: structs[i].span.start.line, tag: 1, idx: i })
    i = i + 1
  }
  i = 0
  while i < len(enums) {
    items = append(items, DeclItem { line: enums[i].span.start.line, tag: 2, idx: i })
    i = i + 1
  }
  i = 0
  while i < len(interfaces) {
    items = append(items, DeclItem { line: interfaces[i].span.start.line, tag: 3, idx: i })
    i = i + 1
  }
  i = 0
  while i < len(classes) {
    items = append(items, DeclItem { line: classes[i].span.start.line, tag: 4, idx: i })
    i = i + 1
  }
  i = 0
  while i < len(funcs) {
    items = append(items, DeclItem { line: funcs[i].span.start.line, tag: 5, idx: i })
    i = i + 1
  }
  i = 0
  while i < len(relations) {
    items = append(items, DeclItem { line: relations[i].span.start.line, tag: 6, idx: i })
    i = i + 1
  }
  i = 0
  while i < len(type_aliases) {
    items = append(items, DeclItem { line: type_aliases[i].span.start.line, tag: 7, idx: i })
    i = i + 1
  }

  sort_decl_items(items)

  let mut ci = comment_idx
  i = 0
  while i < len(items) {
    let item = items[i]
    ci = emit_comments_before(sb, comments, ci, item.line, "  ")

    if item.tag == 0 { fmt_import(sb, imports[item.idx]) }
    if item.tag == 1 { fmt_struct(sb, structs[item.idx]) }
    if item.tag == 2 { fmt_enum(sb, enums[item.idx]) }
    if item.tag == 3 { fmt_interface(sb, interfaces[item.idx]) }
    if item.tag == 4 { fmt_class(sb, classes[item.idx]) }
    if item.tag == 5 { fmt_func(sb, funcs[item.idx], "  ") }
    if item.tag == 6 { fmt_relation(sb, relations[item.idx]) }
    if item.tag == 7 { fmt_type_alias(sb, type_aliases[item.idx]) }

    if i < len(items) - 1 {
      sb.write("\n")
    }
    i = i + 1
  }

  ci = emit_comments_before(sb, comments, ci, block.span.end.line + 1, "  ")

  sb.write("}\n")
}

struct DeclItem {
  line: i32
  tag: i32   // 0=import, 1=struct, 2=enum, 3=interface,
             // 4=class, 5=func, 6=relation, 7=type_alias
  idx: i32
}

func sort_decl_items(items: [DeclItem]) {
  // Insertion sort
  let mut i = 1
  while i < len(items) {
    let key = items[i]
    let mut j = i - 1
    while j >= 0 {
      if items[j].line <= key.line { break }
      items[j + 1] = items[j]
      j = j - 1
    }
    items[j + 1] = key
    i = i + 1
  }
}

// --- Declaration formatters ---

func fmt_import(sb: StringBuilder, imp: ImportDecl) {
  if !isnull(imp.alias) {
    sb.write(f"  import {imp.alias!.get_name()} from \"{imp.path}\"\n")
  } else {
    sb.write(f"  import \"{imp.path}\"\n")
  }
}

func fmt_struct(sb: StringBuilder, s: StructDecl) {
  sb.write("  ")
  if s.is_public { sb.write("pub ") }
  let name = if !isnull(s.name) { s.name!.get_name() } else { "?" }
  sb.write(f"struct {name}")
  fmt_type_params(sb, s.stp_children())
  sb.write(" {\n")
  let fields = s.sf_children()
  let mut i = 0
  while i < len(fields) {
    fmt_field(sb, fields[i])
    i = i + 1
  }
  sb.write("  }\n")
}

func fmt_enum(sb: StringBuilder, e: EnumDecl) {
  sb.write("  ")
  if e.is_public { sb.write("pub ") }
  let name = if !isnull(e.name) { e.name!.get_name() } else { "?" }
  sb.write(f"enum {name}")
  fmt_type_params(sb, e.etp_children())
  sb.write(" {")

  let variants = e.ev_children()

  // Check if all simple (no fields)
  let mut all_simple = true
  let mut i = 0
  while i < len(variants) {
    if len(variants[i].evf_children()) > 0 {
      all_simple = false
    }
    i = i + 1
  }

  if all_simple && len(variants) <= 5 {
    sb.write(" ")
    i = 0
    while i < len(variants) {
      if i > 0 { sb.write(" ") }
      let vname = if !isnull(variants[i].name) { variants[i].name!.get_name() } else { "?" }
      sb.write(vname)
      i = i + 1
    }
    sb.write(" }\n")
  } else {
    sb.write("\n")
    i = 0
    while i < len(variants) {
      let vname = if !isnull(variants[i].name) { variants[i].name!.get_name() } else { "?" }
      sb.write(f"    {vname}")
      let vfields = variants[i].evf_children()
      if len(vfields) > 0 {
        sb.write("(")
        let mut j = 0
        while j < len(vfields) {
          if j > 0 { sb.write(", ") }
          if !isnull(vfields[j].name) {
            sb.write(f"{vfields[j].name!.get_name()}: ")
          }
          fmt_type_expr(sb, vfields[j].type_expr)
          j = j + 1
        }
        sb.write(")")
      }
      sb.write("\n")
      i = i + 1
    }
    sb.write("  }\n")
  }
}

func fmt_interface(sb: StringBuilder, iface: InterfaceDecl) {
  sb.write("  ")
  if iface.is_public { sb.write("pub ") }
  let name = if !isnull(iface.name) { iface.name!.get_name() } else { "?" }
  sb.write(f"interface {name}")
  fmt_type_params(sb, iface.itp_children())
  sb.write(" {\n")

  // Implements
  let mut i = 0
  while i < len(iface.implements) {
    sb.write(f"    {iface.implements[i].get_name()}\n")
    i = i + 1
  }

  // Embeds
  let embeds = iface.ie_children()
  i = 0
  while i < len(embeds) {
    let ename = if !isnull(embeds[i].name) { embeds[i].name!.get_name() } else { "?" }
    sb.write(f"    embed {ename}")
    let eargs = embeds[i].ie_arg_children()
    if len(eargs) > 0 {
      sb.write("<")
      let mut j = 0
      while j < len(eargs) {
        if j > 0 { sb.write(", ") }
        fmt_type_expr(sb, eargs[j])
        j = j + 1
      }
      sb.write(">")
    }
    sb.write("\n")
    i = i + 1
  }

  // Fields
  let ifields = iface.ifd_children()
  i = 0
  while i < len(ifields) {
    let tp = if !isnull(ifields[i].type_param) { ifields[i].type_param!.get_name() } else { "?" }
    let fn_name = if !isnull(ifields[i].name) { ifields[i].name!.get_name() } else { "?" }
    sb.write(f"    field {tp}.{fn_name}: ")
    fmt_type_expr(sb, ifields[i].type_expr)
    sb.write("\n")
    i = i + 1
  }

  // Destructors
  let dblocks = iface.idb_children()
  i = 0
  while i < len(dblocks) {
    let dtp = if !isnull(dblocks[i].type_param) { dblocks[i].type_param!.get_name() } else { "?" }
    sb.write(f"    destructor {dtp} {{ ... }}\n")
    i = i + 1
  }

  // Methods
  let methods = iface.im_children()
  i = 0
  while i < len(methods) {
    fmt_func(sb, methods[i], "    ")
    i = i + 1
  }

  sb.write("  }\n")
}

func fmt_class(sb: StringBuilder, c: ClassDecl) {
  sb.write("  ")
  if c.is_public { sb.write("pub ") }
  let name = if !isnull(c.name) { c.name!.get_name() } else { "?" }
  sb.write(f"class {name}")
  fmt_type_params(sb, c.ctp_children())
  if len(c.implements) > 0 {
    sb.write(" implements ")
    let mut i = 0
    while i < len(c.implements) {
      if i > 0 { sb.write(", ") }
      sb.write(c.implements[i].get_name())
      i = i + 1
    }
  }
  sb.write(" {\n")
  let fields = c.cf_children()
  let mut i = 0
  while i < len(fields) {
    fmt_field(sb, fields[i])
    i = i + 1
  }
  let methods = c.cm_children()
  i = 0
  while i < len(methods) {
    fmt_func(sb, methods[i], "    ")
    i = i + 1
  }
  sb.write("  }\n")
}

func fmt_func(sb: StringBuilder, fn: FuncDecl, indent: string) {
  sb.write(indent)
  if fn.is_public { sb.write("pub ") }
  let name = if !isnull(fn.name) { fn.name!.get_name() } else { "?" }
  sb.write(f"func {name}")
  fmt_type_params(sb, fn.fp_children())
  sb.write("(")
  let params = fn.param_children()
  let mut i = 0
  while i < len(params) {
    if i > 0 { sb.write(", ") }
    if params[i].is_self {
      if params[i].is_mut {
        sb.write("mut self")
      } else {
        sb.write("self")
      }
    } else {
      let pname = if !isnull(params[i].name) { params[i].name!.get_name() } else { "_" }
      sb.write(f"{pname}: ")
      fmt_type_expr(sb, params[i].type_expr)
    }
    i = i + 1
  }
  sb.write(")")
  if !isnull(fn.return_type) {
    sb.write(" -> ")
    fmt_type_expr(sb, fn.return_type)
  }
  // Where clauses
  let wheres = fn.where_children()
  i = 0
  while i < len(wheres) {
    let wvar = if !isnull(wheres[i].variable) { wheres[i].variable!.get_name() } else { "?" }
    let wcon = if !isnull(wheres[i].constraint) { wheres[i].constraint!.get_name() } else { "?" }
    sb.write(f"\n{indent}  where {wvar}: {wcon}")
    i = i + 1
  }
  sb.write("\n")
}

func fmt_relation(sb: StringBuilder, r: RelationDecl) {
  sb.write("  ")
  if !isnull(r.hint) {
    sb.write(f"[{r.hint!.get_name()}] ")
  }
  let pname = if !isnull(r.parent.type_name) { r.parent.type_name!.get_name() } else { "?" }
  sb.write(pname)
  if !isnull(r.parent.label) {
    sb.write(f".{r.parent.label!.get_name()}")
  }
  match r.kind {
    Owns => { sb.write(" owns ") }
    Refs => { sb.write(" refs ") }
  }
  if r.is_many { sb.write("[") }
  let cname = if !isnull(r.child.type_name) { r.child.type_name!.get_name() } else { "?" }
  sb.write(cname)
  if !isnull(r.child.label) {
    sb.write(f".{r.child.label!.get_name()}")
  }
  if r.is_many { sb.write("]") }
  sb.write("\n")
}

func fmt_type_alias(sb: StringBuilder, t: TypeAliasDecl) {
  sb.write("  ")
  if t.is_public { sb.write("pub ") }
  let name = if !isnull(t.name) { t.name!.get_name() } else { "?" }
  sb.write(f"type {name} = ")
  fmt_type_expr(sb, t.type_expr)
  sb.write("\n")
}

// --- Helpers ---

func fmt_field(sb: StringBuilder, f: Field) {
  let name = if !isnull(f.name) { f.name!.get_name() } else { "?" }
  sb.write(f"    {name}: ")
  fmt_type_expr(sb, f.type_expr)
  if !isnull(f.guarded_by) {
    sb.write(f" guarded_by({f.guarded_by!.get_name()})")
  }
  sb.write("\n")
}

func fmt_type_params(sb: StringBuilder, tps: [TypeParam]) {
  if len(tps) == 0 { return }
  sb.write("<")
  let mut i = 0
  while i < len(tps) {
    if i > 0 { sb.write(", ") }
    let name = if !isnull(tps[i].name) { tps[i].name!.get_name() } else { "?" }
    sb.write(name)
    if !isnull(tps[i].constraint) {
      sb.write(f": {tps[i].constraint!.get_name()}")
    }
    i = i + 1
  }
  sb.write(">")
}

func fmt_type_expr(sb: StringBuilder, te: TypeExpr?) {
  if isnull(te) {
    sb.write("?")
    return
  }
  match te!.kind {
    Named(name, args) => {
      sb.write(name.get_name())
      if len(args) > 0 {
        sb.write("<")
        let mut i = 0
        while i < len(args) {
          if i > 0 { sb.write(", ") }
          fmt_type_expr(sb, args[i])
          i = i + 1
        }
        sb.write(">")
      }
    }
    Optional(inner) => {
      fmt_type_expr(sb, inner)
      sb.write("?")
    }
    Union(variants) => {
      let mut i = 0
      while i < len(variants) {
        if i > 0 { sb.write(" | ") }
        fmt_type_expr(sb, variants[i])
        i = i + 1
      }
    }
    Sequence(elem) => {
      sb.write("[")
      fmt_type_expr(sb, elem)
      sb.write("]")
    }
    Map(key, value) => {
      sb.write("map[")
      fmt_type_expr(sb, key)
      sb.write("]")
      fmt_type_expr(sb, value)
    }
    Tuple(fields) => {
      sb.write("(")
      let mut i = 0
      while i < len(fields) {
        if i > 0 { sb.write(", ") }
        if !isnull(fields[i].name) {
          sb.write(f"{fields[i].name!.get_name()}: ")
        }
        fmt_type_expr(sb, fields[i].type_expr)
        i = i + 1
      }
      sb.write(")")
    }
    Func(params, ret) => {
      sb.write("(")
      let mut i = 0
      while i < len(params) {
        if i > 0 { sb.write(", ") }
        fmt_type_expr(sb, params[i])
        i = i + 1
      }
      sb.write(") -> ")
      fmt_type_expr(sb, ret)
    }
    Channel(elem) => {
      sb.write("channel<")
      fmt_type_expr(sb, elem)
      sb.write(">")
    }
    Generator(elem) => {
      sb.write("generator<")
      fmt_type_expr(sb, elem)
      sb.write(">")
    }
    Lock => { sb.write("lock") }
    Unit => { sb.write("unit") }
  }
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
  let args = os_args()
  if len(args) < 2 {
    eprint(USAGE)
    os_exit(1)
  }

  let resolved = resolve_command(args[1])
  if !resolved._1 {
    eprint(f"\n{USAGE}")
    os_exit(1)
  }
  let cmd = resolved._0

  // Build sub-args
  let mut sub_args: [string] = []
  let mut i = 2
  while i < len(args) {
    sub_args = append(sub_args, args[i])
    i = i + 1
  }

  let mut ok = false
  if cmd == "compile" { ok = cmd_compile(sub_args) }
  if cmd == "test" { ok = cmd_test(sub_args) }
  if cmd == "fmt" { ok = cmd_fmt(sub_args) }
  if cmd == "help" {
    print(USAGE)
    ok = true
  }

  if !ok { os_exit(1) }
}
