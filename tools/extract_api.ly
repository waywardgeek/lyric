// extract_api.ly — Extract the public API from Lyric source files.
//
// Usage: extract_api <file.ly> [...]
//
// Outputs a JSON object to stdout matching the PackageInfo structure:
// {
//   "name": "module_name",
//   "structs": { "Name": { "fields": {"f": "type"}, "methods": {...} } },
//   "interfaces": { "Name": { "methods": {...} } },
//   "functions": { "name": { "params": [{"name":"p","type":"T"}], "returns": ["T"] } },
//   "typedefs": { "EnumName": "enum { Variant1, Variant2 }" },
//   "enums": { "Name": { "variants": [{"name":"V","fields":[...]}] } }
// }

lyric extract_api {

// ---------------------------------------------------------------------------
// JSON helpers — manual JSON building via StringBuilder
// ---------------------------------------------------------------------------

func json_escape(s: string) -> string {
  let sb = new_string_builder()
  let mut i = 0
  while i < len(s) {
    let c = s[i:i+1]
    if c == "\"" {
      sb.write("\\\"")
    } else if c == "\\" {
      sb.write("\\\\")
    } else if c == "\n" {
      sb.write("\\n")
    } else if c == "\t" {
      sb.write("\\t")
    } else {
      sb.write(c)
    }
    i = i + 1
  }
  return sb.to_string()
}

func type_expr_to_string(te: TypeExpr?) -> string {
  if te == null {
    return "any"
  }
  match te!.kind {
    Named(name, args) => {
      if len(args) == 0 {
        return name.name
      }
      let sb = new_string_builder()
      sb.write(name.name)
      sb.write("<")
      let mut i = 0
      while i < len(args) {
        if i > 0 { sb.write(", ") }
        sb.write(type_expr_to_string(args[i]))
        i = i + 1
      }
      sb.write(">")
      return sb.to_string()
    }
    Optional(inner) => {
      return type_expr_to_string(inner) + "?"
    }
    Sequence(elem) => {
      return "[" + type_expr_to_string(elem) + "]"
    }
    Map(key, value) => {
      return "Dict<" + type_expr_to_string(key) + ", " + type_expr_to_string(value) + ">"
    }
    Tuple(fields) => {
      let sb = new_string_builder()
      sb.write("(")
      let mut i = 0
      while i < len(fields) {
        if i > 0 { sb.write(", ") }
        if fields[i].name != null {
          sb.write(fields[i].name!.name)
          sb.write(": ")
        }
        sb.write(type_expr_to_string(fields[i].type_expr))
        i = i + 1
      }
      sb.write(")")
      return sb.to_string()
    }
    Func(params, ret) => {
      let sb = new_string_builder()
      sb.write("fn(")
      let mut i = 0
      while i < len(params) {
        if i > 0 { sb.write(", ") }
        sb.write(type_expr_to_string(params[i]))
        i = i + 1
      }
      sb.write(") -> ")
      sb.write(type_expr_to_string(ret))
      return sb.to_string()
    }
    Channel(elem) => {
      return "chan " + type_expr_to_string(elem)
    }
    Generator(elem) => {
      return "gen " + type_expr_to_string(elem)
    }
    Union(variants) => {
      let sb = new_string_builder()
      let mut i = 0
      while i < len(variants) {
        if i > 0 { sb.write(" | ") }
        sb.write(type_expr_to_string(variants[i]))
        i = i + 1
      }
      return sb.to_string()
    }
    Lock => { return "lock" }
    Unit => { return "()" }
  }
  return "any"
}

// ---------------------------------------------------------------------------
// JSON emitters
// ---------------------------------------------------------------------------

func emit_func_json(sb: StringBuilder, fn_: FuncDecl) {
  sb.write("{\"params\":[")
  let params = fn_.param_children()
  let mut first = true
  let mut pi = 0
  while pi < len(params) {
    let p = params[pi]
    if p.is_self {
      pi = pi + 1
      continue
    }
    if !first { sb.write(",") }
    first = false
    sb.write("{\"name\":\"")
    if p.name != null {
      sb.write(json_escape(p.name!.name))
    }
    sb.write("\",\"type\":\"")
    let mut type_str = type_expr_to_string(p.type_expr)
    if p.is_mut {
      type_str = "mut " + type_str
    }
    sb.write(json_escape(type_str))
    sb.write("\"}")
    pi = pi + 1
  }
  sb.write("],\"returns\":[")
  if fn_.return_type != null {
    sb.write("\"")
    sb.write(json_escape(type_expr_to_string(fn_.return_type)))
    sb.write("\"")
  }
  sb.write("]}")
}

func emit_struct_json(sb: StringBuilder, name: string, fields: [Field], methods: [FuncDecl]) {
  sb.write("\"")
  sb.write(json_escape(name))
  sb.write("\":{\"fields\":{")
  let mut fi = 0
  while fi < len(fields) {
    if fi > 0 { sb.write(",") }
    sb.write("\"")
    if fields[fi].name != null {
      sb.write(json_escape(fields[fi].name!.name))
    }
    sb.write("\":\"")
    sb.write(json_escape(type_expr_to_string(fields[fi].type_expr)))
    sb.write("\"")
    fi = fi + 1
  }
  sb.write("},\"methods\":{")
  let mut mi = 0
  let mut first_method = true
  while mi < len(methods) {
    if methods[mi].is_public || true {
      if !first_method { sb.write(",") }
      first_method = false
      sb.write("\"")
      if methods[mi].name != null {
        sb.write(json_escape(methods[mi].name!.name))
      }
      sb.write("\":")
      emit_func_json(sb, methods[mi])
    }
    mi = mi + 1
  }
  sb.write("}}")
}

func emit_enum_json(sb: StringBuilder, name: string, decl: EnumDecl) {
  sb.write("\"")
  sb.write(json_escape(name))
  sb.write("\":\"enum { ")
  let variants = decl.ev_children()
  let mut vi = 0
  while vi < len(variants) {
    if vi > 0 { sb.write(", ") }
    let v = variants[vi]
    if v.name != null {
      sb.write(v.name!.name)
    }
    let vfields = v.evf_children()
    if len(vfields) > 0 {
      sb.write("(")
      let mut fi = 0
      while fi < len(vfields) {
        if fi > 0 { sb.write(", ") }
        if vfields[fi].name != null {
          sb.write(vfields[fi].name!.name)
          sb.write(": ")
        }
        sb.write(type_expr_to_string(vfields[fi].type_expr))
        fi = fi + 1
      }
      sb.write(")")
    }
    vi = vi + 1
  }
  sb.write(" }\"")
}

func emit_interface_json(sb: StringBuilder, name: string, decl: InterfaceDecl) {
  sb.write("\"")
  sb.write(json_escape(name))
  sb.write("\":{\"methods\":{")
  let methods = decl.im_children()
  let mut mi = 0
  let mut first = true
  while mi < len(methods) {
    if !first { sb.write(",") }
    first = false
    sb.write("\"")
    if methods[mi].name != null {
      sb.write(json_escape(methods[mi].name!.name))
    }
    sb.write("\":")
    emit_func_json(sb, methods[mi])
    mi = mi + 1
  }
  sb.write("}}")
}

// ---------------------------------------------------------------------------
// Main extraction logic
// ---------------------------------------------------------------------------

func extract_file(file: File?) -> string {
  if file == null {
    return "{\"name\":\"\",\"structs\":{},\"interfaces\":{},\"functions\":{},\"typedefs\":{}}"
  }

  let sb = new_string_builder()

  // Determine module name from first lyric block
  let blocks = file!.fb_children()
  let mut module_name = ""
  if len(blocks) > 0 {
    if blocks[0].name != null {
      module_name = blocks[0].name!.name
    }
  }

  sb.write("{\"name\":\"")
  sb.write(json_escape(module_name))
  sb.write("\",\"structs\":{")

  // Collect all public structs and classes
  let mut first_struct = true

  // Collect extension methods: receiver_type → [FuncDecl]
  let ext_methods = Dict<Sym, [FuncDecl]>()

  let mut bi = 0
  while bi < len(blocks) {
    let block = blocks[bi]
    let fns = block.fd_children()
    let mut fi = 0
    while fi < len(fns) {
      let fn_ = fns[fi]
      if fn_.receiver_type != null && fn_.name != null {
        let recv = fn_.receiver_type!.name
        let existing = ext_methods.get(sym(recv))
        if existing != null {
          let mut list = existing!.value
          list = append(list, fn_)
          ext_methods.set(sym(recv), list)
        } else {
          let list: [FuncDecl] = [fn_]
          ext_methods.set(sym(recv), list)
        }
      }
      fi = fi + 1
    }
    bi = bi + 1
  }

  bi = 0
  while bi < len(blocks) {
    let block = blocks[bi]

    // Structs
    let structs = block.sd_children()
    let mut si = 0
    while si < len(structs) {
      let s = structs[si]
      if s.name != null {
        if !first_struct { sb.write(",") }
        first_struct = false
        let fields = s.sf_children()
        let mut all_fields: [Field] = []
        let mut ffi = 0
        while ffi < len(fields) {
          all_fields = append(all_fields, fields[ffi])
          ffi = ffi + 1
        }
        // Extension methods for this struct
        let mut ext_list: [FuncDecl] = []
        let ext_entry = ext_methods.get(sym(s.name!.name))
        if ext_entry != null {
          ext_list = ext_entry!.value
        }
        emit_struct_json(sb, s.name!.name, all_fields, ext_list)
      }
      si = si + 1
    }

    // Classes (emit as structs with methods)
    let classes = block.cd_children()
    let mut ci = 0
    while ci < len(classes) {
      let cls = classes[ci]
      if cls.name != null {
        if !first_struct { sb.write(",") }
        first_struct = false
        let fields = cls.cf_children()
        let methods = cls.cm_children()
        let mut all_fields: [Field] = []
        let mut ffi = 0
        while ffi < len(fields) {
          all_fields = append(all_fields, fields[ffi])
          ffi = ffi + 1
        }
        let mut all_methods: [FuncDecl] = []
        let mut mi = 0
        while mi < len(methods) {
          all_methods = append(all_methods, methods[mi])
          mi = mi + 1
        }
        // Extension methods for this class
        let ext_entry = ext_methods.get(sym(cls.name!.name))
        if ext_entry != null {
          let ext_list = ext_entry!.value
          let mut ei = 0
          while ei < len(ext_list) {
            all_methods = append(all_methods, ext_list[ei])
            ei = ei + 1
          }
        }
        emit_struct_json(sb, cls.name!.name, all_fields, all_methods)
      }
      ci = ci + 1
    }

    bi = bi + 1
  }

  sb.write("},\"interfaces\":{")

  // Interfaces
  let mut first_iface = true
  bi = 0
  while bi < len(blocks) {
    let block = blocks[bi]
    let ifaces = block.id_children()
    let mut ii = 0
    while ii < len(ifaces) {
      let iface = ifaces[ii]
      if iface.name != null {
        if !first_iface { sb.write(",") }
        first_iface = false
        emit_interface_json(sb, iface.name!.name, iface)
      }
      ii = ii + 1
    }
    bi = bi + 1
  }

  sb.write("},\"functions\":{")

  // Free functions (not extension methods)
  let mut first_func = true
  bi = 0
  while bi < len(blocks) {
    let block = blocks[bi]
    let fns = block.fd_children()
    let mut fi = 0
    while fi < len(fns) {
      let fn_ = fns[fi]
      if fn_.name != null && fn_.receiver_type == null {
        if !first_func { sb.write(",") }
        first_func = false
        sb.write("\"")
        sb.write(json_escape(fn_.name!.name))
        sb.write("\":")
        emit_func_json(sb, fn_)
      }
      fi = fi + 1
    }
    bi = bi + 1
  }

  sb.write("},\"typedefs\":{")

  // Enums as typedefs
  let mut first_enum = true
  bi = 0
  while bi < len(blocks) {
    let block = blocks[bi]
    let enums = block.ed_children()
    let mut ei = 0
    while ei < len(enums) {
      let e = enums[ei]
      if e.name != null {
        if !first_enum { sb.write(",") }
        first_enum = false
        emit_enum_json(sb, e.name!.name, e)
      }
      ei = ei + 1
    }
    bi = bi + 1
  }

  sb.write("}}")
  return sb.to_string()
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

func main() {
  let args = os_args()
  if len(args) < 2 {
    eprintln("Usage: extract_api <file.ly> [...]")
    exit(1)
  }

  // Parse all input files and merge into one
  let mut files: [File?] = []
  let mut i = 1
  while i < len(args) {
    let result = read_file(args[i])
    let src = result._0
    let err = result._1
    if err != null {
      eprintln(f"error: cannot read {args[i]}")
      exit(1)
    }
    let parse_result = parse_file(src, args[i])
    let file = parse_result._0
    if file == null {
      eprintln(f"error: cannot parse {args[i]}")
      exit(1)
    }
    files = append(files, file)
    i = i + 1
  }

  let merged = merge_files(files)
  let json = extract_file(merged)
  println(json)
}

}
