// memory.ly — Memory management pass
//
// Runs after monomorphization, before C backend emission.
// Rewrites class allocations to slab-based equivalents.
// After this pass, class allocs use ExSlabAlloc; field inits use StSlabSet.
// Class handles remain ClassName* pointers (AoS slab, pointer-stable blocks).
//
// Design: ClassName* — NULL = none, valid = non-null pointer into slab block.
// The C backend emits slab infrastructure based on LProgram.classes + slab_mode.

// ==========================================================================
// Escape Analysis
// ==========================================================================
//
// Determines which function parameters can cause a slice's backing array to
// escape (be stored in a struct/class field). Used to decide which locally-
// created slices are safe to free at scope exit.
//
// Algorithm: fixed-point iteration.
//   Pass 1: Mark params that directly store into struct/class fields.
//   Pass 2+: Mark params passed to another function's escaping param position.
//   Repeat until no changes.

// Per-function escape info: which param indices have escaping slice data.
// Key is "funcname:paramindex" (Sym), value is bool.
// Uses flat key encoding because Dict<Sym, [bool]> crashes the old compiler.
func compute_escape_map(prog: LProgram) -> Dict<Sym, bool> {
  let mut escape_map = Dict<Sym, bool>()

  // Build a name→param-names set for quick lookup of "is this a param?"
  // Also build param_index: name→param-name→index
  let mut param_names = Dict<Sym, bool>()
  let mut fi = 0
  while fi < len(prog.functions) {
    let f = prog.functions[fi]
    let mut pi = 0
    while pi < len(f.params) {
      // Store "funcname:paramname" → true for param lookup
      param_names.set(sym(f.name + ":" + f.params[pi].name), true)
      pi = pi + 1
    }
    fi = fi + 1
  }

  // Fixed-point iteration
  let mut changed = true
  while changed {
    changed = false
    fi = 0
    while fi < len(prog.functions) {
      let f = prog.functions[fi]
      if scan_escapes_in_stmts(f.body, f.name, f.params, param_names, mut escape_map) {
        changed = true
      }
      fi = fi + 1
    }
  }

  return escape_map
}

// Build the escape key for a function param
func escape_key(func_name: string, param_name: string) -> Sym {
  return sym(func_name + ":" + param_name)
}

// Build the escape key for a function param by index
func escape_key_idx(func_name: string, idx: i32) -> Sym {
  return sym(f"{func_name}:{idx}")
}

// Scan a statement list for escape patterns. Returns true if any new escapes found.
func scan_escapes_in_stmts(stmts: [LStmt?], func_name: string, params: [LParam], param_names: Dict<Sym, bool>, mut escape_map: Dict<Sym, bool>) -> bool {
  let mut changed = false
  let mut i = 0
  while i < len(stmts) {
    if !isnull(stmts[i]) {
      if scan_escapes_in_stmt(stmts[i]!, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
    i = i + 1
  }
  return changed
}

// Scan a single statement for escape patterns.
func scan_escapes_in_stmt(s: LStmt, func_name: string, params: [LParam], param_names: Dict<Sym, bool>, mut escape_map: Dict<Sym, bool>) -> bool {
  let mut changed = false

  // Direct escape: slice param stored in struct literal field
  if s.kind is StTempDef {
    if !isnull(s.temp_def) && !isnull(s.temp_def!.expr) {
      if scan_escapes_in_expr(s.temp_def!.expr!, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }

  // Direct escape: param stored in class/slab field
  if s.kind is StSlabSet {
    if !isnull(s.slab_set) && !isnull(s.slab_set!.value) {
      if mark_value_escape(s.slab_set!.value!, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }
  if s.kind is StClassSet {
    if !isnull(s.class_set) && !isnull(s.class_set!.value) {
      if mark_value_escape(s.class_set!.value!, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }
  if s.kind is StStructSet {
    if !isnull(s.struct_set) && !isnull(s.struct_set!.value) {
      if mark_value_escape(s.struct_set!.value!, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }

  // Transitive escape: param passed to function where that arg position escapes
  if s.kind is StSideEffect {
    if !isnull(s.side_effect) && !isnull(s.side_effect!.expr) {
      if scan_escapes_in_expr(s.side_effect!.expr!, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }

  // Recurse into sub-blocks
  if s.kind is StIf {
    if !isnull(s.if_data) {
      let d = s.if_data!
      if scan_escapes_in_stmts(d.then_body, func_name, params, param_names, mut escape_map) {
        changed = true
      }
      if scan_escapes_in_stmts(d.else_body, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }
  if s.kind is StWhile {
    if !isnull(s.while_data) {
      if scan_escapes_in_stmts(s.while_data!.body, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }
  if s.kind is StFor {
    if !isnull(s.for_data) {
      if scan_escapes_in_stmts(s.for_data!.body, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }
  if s.kind is StBlock {
    if !isnull(s.block) {
      if scan_escapes_in_stmts(s.block!.stmts, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }
  if s.kind is StSwitch {
    if !isnull(s.switch_data) {
      let mut ci = 0
      while ci < len(s.switch_data!.cases) {
        if scan_escapes_in_stmts(s.switch_data!.cases[ci].body, func_name, params, param_names, mut escape_map) {
          changed = true
        }
        ci = ci + 1
      }
    }
  }
  if s.kind is StTypeSwitch {
    if !isnull(s.type_switch) {
      let mut ci = 0
      while ci < len(s.type_switch!.cases) {
        if scan_escapes_in_stmts(s.type_switch!.cases[ci].body, func_name, params, param_names, mut escape_map) {
          changed = true
        }
        ci = ci + 1
      }
    }
  }
  if s.kind is StSpawn {
    if !isnull(s.spawn_data) {
      if scan_escapes_in_stmts(s.spawn_data!.body, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }
  if s.kind is StSelect {
    if !isnull(s.select_data) {
      let mut ci = 0
      while ci < len(s.select_data!.cases) {
        if scan_escapes_in_stmts(s.select_data!.cases[ci].body, func_name, params, param_names, mut escape_map) {
          changed = true
        }
        ci = ci + 1
      }
    }
  }
  if s.kind is StLock {
    if !isnull(s.lock_data) {
      if scan_escapes_in_stmts(s.lock_data!.body, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }
  if s.kind is StDefer {
    if !isnull(s.defer_data) {
      if scan_escapes_in_stmts(s.defer_data!.body, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }

  return changed
}

// Scan an expression for escape patterns (struct literals + function calls).
func scan_escapes_in_expr(e: LExpr, func_name: string, params: [LParam], param_names: Dict<Sym, bool>, mut escape_map: Dict<Sym, bool>) -> bool {
  let mut changed = false

  // Struct literal: if any field value is a param, that param escapes
  if e.kind is ExStructLit {
    if !isnull(e.struct_lit) {
      let mut i = 0
      while i < len(e.struct_lit!.fields) {
        if !isnull(e.struct_lit!.fields[i].value) {
          if mark_value_escape(e.struct_lit!.fields[i].value!, func_name, params, param_names, mut escape_map) {
            changed = true
          }
        }
        i = i + 1
      }
    }
  }

  // Class alloc: if any field value is a param, that param escapes
  if e.kind is ExClassAlloc {
    if !isnull(e.class_alloc) {
      let mut i = 0
      while i < len(e.class_alloc!.fields) {
        if !isnull(e.class_alloc!.fields[i].value) {
          if mark_value_escape(e.class_alloc!.fields[i].value!, func_name, params, param_names, mut escape_map) {
            changed = true
          }
        }
        i = i + 1
      }
    }
  }
  if e.kind is ExSlabAlloc {
    if !isnull(e.slab_alloc) {
      let mut i = 0
      while i < len(e.slab_alloc!.fields) {
        if !isnull(e.slab_alloc!.fields[i].value) {
          if mark_value_escape(e.slab_alloc!.fields[i].value!, func_name, params, param_names, mut escape_map) {
            changed = true
          }
        }
        i = i + 1
      }
    }
  }

  // Enum variant construction: if any field value is a param, that param escapes
  if e.kind is ExVariantConstruct {
    if !isnull(e.variant_construct) {
      let mut i = 0
      while i < len(e.variant_construct!.fields) {
        if !isnull(e.variant_construct!.fields[i]) {
          if mark_value_escape(e.variant_construct!.fields[i]!, func_name, params, param_names, mut escape_map) {
            changed = true
          }
        }
        i = i + 1
      }
    }
  }

  // Function call: if arg is a param and callee's param position escapes, mark it
  if e.kind is ExCall {
    if !isnull(e.call) {
      if check_call_escapes(e.call!.func_name, e.call!.args, 0, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }
  if e.kind is ExMethodCall {
    if !isnull(e.method_call) {
      if check_call_escapes(e.method_call!.method, e.method_call!.args, 1, func_name, params, param_names, mut escape_map) {
        changed = true
      }
    }
  }

  return changed
}

// Resolve the full function name for a method call by combining receiver type + method.
// E.g., receiver type "SymTable" + method "set_st_buckets" → "SymTable.set_st_buckets"
func resolve_method_callee_name(d: LMethodCallData) -> string {
  if !isnull(d.receiver) && !isnull(d.receiver!.typ) {
    let mut class_name = d.receiver!.typ!.name
    if d.receiver!.typ!.kind is TyOptional && !isnull(d.receiver!.typ!.elem) {
      class_name = d.receiver!.typ!.elem!.name
    }
    if class_name != "" {
      return class_name + "." + d.method
    }
  }
  return d.method
}

// Check if a value is a param variable and mark it as escaping.
func mark_value_escape(v: LValue, func_name: string, params: [LParam], param_names: Dict<Sym, bool>, mut escape_map: Dict<Sym, bool>) -> bool {
  if v.kind is ValVar {
    // Check if this variable is a parameter of the current function
    let is_param = param_names.get(sym(func_name + ":" + v.name))
    if !isnull(is_param) {
      // Find param index
      let mut pi = 0
      while pi < len(params) {
        if params[pi].name == v.name {
          let key = escape_key_idx(func_name, pi)
          let already = escape_map.get(key)
          if isnull(already) {
            escape_map.set(key, true)
            return true
          }
          return false
        }
        pi = pi + 1
      }
    }
  }
  return false
}

// Check if any arg passed to a callee is a param, and the callee's corresponding
// param position is marked as escaping. If so, mark the caller's param as escaping.
func check_call_escapes(callee_name: string, args: [LValue?], param_offset: i32, func_name: string, params: [LParam], param_names: Dict<Sym, bool>, mut escape_map: Dict<Sym, bool>) -> bool {
  let mut changed = false
  let mut ai = 0
  while ai < len(args) {
    // Check if callee's param at this position escapes
    // param_offset is 1 for method calls (self is param 0 but not in args)
    let callee_key = escape_key_idx(callee_name, ai + param_offset)
    let callee_escapes = escape_map.get(callee_key)
    if !isnull(callee_escapes) {
      // This position escapes — if our arg is a param, mark it
      if !isnull(args[ai]) {
        if mark_value_escape(args[ai]!, func_name, params, param_names, mut escape_map) {
          changed = true
        }
      }
    }
    ai = ai + 1
  }
  return changed
}

// Check if a variable is passed to any function at an escaping param position.
func var_escapes_via_call(var_name: string, stmts: [LStmt?], escape_map: Dict<Sym, bool>) -> bool {
  let mut i = 0
  while i < len(stmts) {
    if !isnull(stmts[i]) {
      if var_escapes_in_stmt(var_name, stmts[i]!, escape_map) {
        return true
      }
    }
    i = i + 1
  }
  return false
}

func var_escapes_in_stmt(var_name: string, s: LStmt, escape_map: Dict<Sym, bool>) -> bool {
  // Check expressions in temp defs
  if s.kind is StTempDef {
    if !isnull(s.temp_def) && !isnull(s.temp_def!.expr) {
      if var_escapes_in_expr(var_name, s.temp_def!.expr!, escape_map) {
        return true
      }
    }
  }
  // Check side-effect expressions
  if s.kind is StSideEffect {
    if !isnull(s.side_effect) && !isnull(s.side_effect!.expr) {
      if var_escapes_in_expr(var_name, s.side_effect!.expr!, escape_map) {
        return true
      }
    }
  }
  // Direct field stores
  // Assignment to another variable: slice header is shallow-copied, data pointer shared.
  // If the target is returned or stored in a field, our slice data escapes.
  // Conservative: treat any assignment of our var as an escape.
  if s.kind is StAssign {
    if !isnull(s.assign) && !isnull(s.assign!.value) {
      if is_var_value(var_name, s.assign!.value!) {
        return true
      }
    }
  }
  if s.kind is StVarDecl {
    if !isnull(s.var_decl) && !isnull(s.var_decl!.init) {
      if is_var_value(var_name, s.var_decl!.init!) {
        return true
      }
    }
  }
  if s.kind is StSlabSet {
    if !isnull(s.slab_set) && !isnull(s.slab_set!.value) {
      if is_var_value(var_name, s.slab_set!.value!) {
        return true
      }
    }
  }
  if s.kind is StClassSet {
    if !isnull(s.class_set) && !isnull(s.class_set!.value) {
      if is_var_value(var_name, s.class_set!.value!) {
        return true
      }
    }
  }
  if s.kind is StStructSet {
    if !isnull(s.struct_set) && !isnull(s.struct_set!.value) {
      if is_var_value(var_name, s.struct_set!.value!) {
        return true
      }
    }
  }
  // Recurse into sub-blocks
  if s.kind is StIf {
    if !isnull(s.if_data) {
      if var_escapes_via_call(var_name, s.if_data!.then_body, escape_map) {
        return true
      }
      if var_escapes_via_call(var_name, s.if_data!.else_body, escape_map) {
        return true
      }
    }
  }
  if s.kind is StWhile {
    if !isnull(s.while_data) {
      if var_escapes_via_call(var_name, s.while_data!.body, escape_map) {
        return true
      }
    }
  }
  if s.kind is StFor {
    if !isnull(s.for_data) {
      if var_escapes_via_call(var_name, s.for_data!.body, escape_map) {
        return true
      }
    }
  }
  if s.kind is StBlock {
    if !isnull(s.block) {
      if var_escapes_via_call(var_name, s.block!.stmts, escape_map) {
        return true
      }
    }
  }
  if s.kind is StSwitch {
    if !isnull(s.switch_data) {
      let mut ci = 0
      while ci < len(s.switch_data!.cases) {
        if var_escapes_via_call(var_name, s.switch_data!.cases[ci].body, escape_map) {
          return true
        }
        ci = ci + 1
      }
    }
  }
  if s.kind is StTypeSwitch {
    if !isnull(s.type_switch) {
      let mut ci = 0
      while ci < len(s.type_switch!.cases) {
        if var_escapes_via_call(var_name, s.type_switch!.cases[ci].body, escape_map) {
          return true
        }
        ci = ci + 1
      }
    }
  }
  return false
}

func var_escapes_in_expr(var_name: string, e: LExpr, escape_map: Dict<Sym, bool>) -> bool {
  // Struct/class literal fields
  if e.kind is ExStructLit {
    if !isnull(e.struct_lit) {
      let mut i = 0
      while i < len(e.struct_lit!.fields) {
        if !isnull(e.struct_lit!.fields[i].value) {
          if is_var_value(var_name, e.struct_lit!.fields[i].value!) {
            return true
          }
        }
        i = i + 1
      }
    }
  }
  if e.kind is ExClassAlloc {
    if !isnull(e.class_alloc) {
      let mut i = 0
      while i < len(e.class_alloc!.fields) {
        if !isnull(e.class_alloc!.fields[i].value) {
          if is_var_value(var_name, e.class_alloc!.fields[i].value!) {
            return true
          }
        }
        i = i + 1
      }
    }
  }
  if e.kind is ExSlabAlloc {
    if !isnull(e.slab_alloc) {
      let mut i = 0
      while i < len(e.slab_alloc!.fields) {
        if !isnull(e.slab_alloc!.fields[i].value) {
          if is_var_value(var_name, e.slab_alloc!.fields[i].value!) {
            return true
          }
        }
        i = i + 1
      }
    }
  }
  // Enum variant construction: fields stored in variant escape
  if e.kind is ExVariantConstruct {
    if !isnull(e.variant_construct) {
      let mut i = 0
      while i < len(e.variant_construct!.fields) {
        if !isnull(e.variant_construct!.fields[i]) {
          if is_var_value(var_name, e.variant_construct!.fields[i]!) {
            return true
          }
        }
        i = i + 1
      }
    }
  }
  // Function call: check if our var is at an escaping param position
  if e.kind is ExCall {
    if !isnull(e.call) {
      if var_at_escaping_position(var_name, e.call!.func_name, e.call!.args, 0, escape_map) {
        return true
      }
    }
  }
  if e.kind is ExMethodCall {
    if !isnull(e.method_call) {
      if var_at_escaping_position(var_name, e.method_call!.method, e.method_call!.args, 1, escape_map) {
        return true
      }
    }
  }
  return false
}

func var_at_escaping_position(var_name: string, callee_name: string, args: [LValue?], param_offset: i32, escape_map: Dict<Sym, bool>) -> bool {
  let mut ai = 0
  while ai < len(args) {
    let callee_key = escape_key_idx(callee_name, ai + param_offset)
    let callee_escapes = escape_map.get(callee_key)
    if !isnull(callee_escapes) {
      if !isnull(args[ai]) {
        if is_var_value(var_name, args[ai]!) {
          return true
        }
      }
    }
    ai = ai + 1
  }
  return false
}


func is_var_value(var_name: string, v: LValue) -> bool {
  if v.kind is ValVar {
    return v.name == var_name
  }
  return false
}

// ==========================================================================
// Slab Rewrite + Scope-Exit Slice Free
// ==========================================================================

// Rewrite the entire program from malloc-based classes to slab-based allocation.
// Also frees locally-created slices at scope exit when safe.
func slab_rewrite(prog: LProgram) {
  prog.slab_mode = true

  // Compute escape analysis first
  let escape_map = compute_escape_map(prog)

  // Rewrite all functions
  let mut fi = 0
  while fi < len(prog.functions) {
    let f = prog.functions[fi]
    let new_body = slab_rewrite_stmts(f.body, f.body, escape_map, prog)
    prog.functions[fi].body = new_body

    // Inject slab_free(self) at end of destroy methods
    let fname = prog.functions[fi].name
    if fname == "destroy" {
      if len(prog.functions[fi].params) > 0 {
        let p = prog.functions[fi].params[0]
        if !isnull(p.typ) {
          if p.typ!.kind is TyClassHandle {
            let free_stmt = LStmt {
              kind: StSlabFree,
              slab_free: LSlabFreeData {
                class_name: p.typ!.name,
                handle: LValue {
                  kind: ValVar,
                  name: p.name,
                  temp_id: 0,
                  typ: p.typ,
                },
              },
            }
            let mut fn_body = prog.functions[fi].body
            fn_body.push(free_stmt)
            prog.functions[fi].body = fn_body
          }
        }
      }
    }
    fi = fi + 1
  }
}

// Rewrite a list of statements, returning potentially expanded list.
// Also injects StSliceFree at scope exits for locally-declared slice/string variables
// that own their backing data and don't escape.
// `all_stmts` is the full function body (for escape-via-call checking).
func slab_rewrite_stmts(stmts: [LStmt?], all_stmts: [LStmt?], escape_map: Dict<Sym, bool>, prog: LProgram) -> [LStmt?] {
  let mut result: [LStmt?] = []
  // Track slice-typed locals declared in THIS scope that OWN their data
  let mut slice_locals: [string] = []
  let mut slice_types: [LType?] = []
  // Track class-handle locals for ref counting (non-owned classes only)
  let mut class_locals: [string] = []
  let mut class_types: [LType?] = []
  // Track temp IDs that come from ExMakeSlice (fresh allocations)
  let mut fresh_temps: [i32] = []
  // Track temp IDs from ExSlabAlloc (fresh class allocations — rc=1 from alloc)
  let mut fresh_class_temps: [i32] = []
  let mut i = 0
  while i < len(stmts) {
    if isnull(stmts[i]) {
      result.push(null)
    } else {
      let s = stmts[i]!

      // Track ExMakeSlice temp defs — these produce fresh backing arrays
      // Track fresh string temps — ExFormat, ExBinOp(string+string), ExCall/ExMethodCall returning string
      if s.kind is StTempDef {
        if !isnull(s.temp_def) && !isnull(s.temp_def!.expr) {
          if s.temp_def!.expr!.kind is ExMakeSlice {
            fresh_temps.push(s.temp_def!.id)
          }
          if is_fresh_string_expr(s.temp_def!.expr!) {
            fresh_temps.push(s.temp_def!.id)
          }
          // Track fresh class allocs (ExSlabAlloc after slab rewrite, ExClassAlloc before)
          if s.temp_def!.expr!.kind is ExSlabAlloc || s.temp_def!.expr!.kind is ExClassAlloc {
            fresh_class_temps.push(s.temp_def!.id)
          }
          // Function/method calls returning class handles transfer ownership (rc=1)
          if s.temp_def!.expr!.kind is ExCall || s.temp_def!.expr!.kind is ExMethodCall {
            if !isnull(s.temp_def!.expr!.typ) && is_rc_class_type(prog, s.temp_def!.expr!.typ) {
              fresh_class_temps.push(s.temp_def!.id)
            }
          }
        }
      }

      // Track slice/string-typed variable declarations, but only if they own data
      if s.kind is StVarDecl {
        if !isnull(s.var_decl) && !isnull(s.var_decl!.typ) {
          let typ_kind = s.var_decl!.typ!.kind
          if typ_kind is TySlice || typ_kind is TyString {
            // Only free if initialized from a fresh allocation
            if is_fresh_slice_init(s.var_decl!.init, fresh_temps) {
              // Check it doesn't escape via function calls or field stores
              if !var_escapes_via_call(s.var_decl!.name, all_stmts, escape_map) {
                slice_locals.push(s.var_decl!.name)
                slice_types.push(s.var_decl!.typ)
              }
            }
          }
          // Track non-owned class handle locals for ref counting
          if is_rc_class_type(prog, s.var_decl!.typ) {
            class_locals.push(s.var_decl!.name)
            class_types.push(s.var_decl!.typ)
            // If init is NOT a fresh alloc (i.e. copying an existing handle), retain
            if !isnull(s.var_decl!.init) && !is_fresh_class_init(s.var_decl!.init, fresh_class_temps) {
              // Emit the VarDecl first, then the retain
              let expanded = slab_rewrite_one_stmt(s, all_stmts, escape_map, prog)
              let mut j = 0
              while j < len(expanded) {
                result.push(expanded[j])
                j = j + 1
              }
              let handle = LValue {
                kind: ValVar,
                name: s.var_decl!.name,
                temp_id: 0,
                typ: s.var_decl!.typ,
              }
              emit_ref_incr(handle, s.var_decl!.typ!.name, mut result)
              i = i + 1
              continue
            }
          }
        }
      }

      // Handle assignment to class-typed var: release old, retain new
      if s.kind is StAssign {
        if !isnull(s.assign) && !isnull(s.assign!.value) {
          let mut found_idx = -1
          let mut ci = 0
          while ci < len(class_locals) {
            if class_locals[ci] == s.assign!.target {
              found_idx = ci
            }
            ci = ci + 1
          }
          if found_idx >= 0 {
            let ctype = class_types[found_idx]
            // Release old value
            let old_handle = LValue {
              kind: ValVar,
              name: s.assign!.target,
              temp_id: 0,
              typ: ctype,
            }
            emit_ref_decr(old_handle, ctype!.name, mut result)
            // Emit the assignment
            let expanded = slab_rewrite_one_stmt(s, all_stmts, escape_map, prog)
            let mut j = 0
            while j < len(expanded) {
              result.push(expanded[j])
              j = j + 1
            }
            // Retain new value (unless fresh alloc)
            if !is_fresh_class_init(s.assign!.value, fresh_class_temps) {
              let new_handle = LValue {
                kind: ValVar,
                name: s.assign!.target,
                temp_id: 0,
                typ: ctype,
              }
              emit_ref_incr(new_handle, ctype!.name, mut result)
            }
            i = i + 1
            continue
          }
        }
      }

      // Also need to retain when storing a class handle into a field
      // StSlabSet (class field store): retain the value being stored
      if s.kind is StSlabSet {
        if !isnull(s.slab_set) && !isnull(s.slab_set!.value) {
          let val = s.slab_set!.value!
          if !isnull(val.typ) && is_rc_class_type(prog, val.typ) {
            emit_ref_incr(s.slab_set!.value, val.typ!.name, mut result)
          }
        }
      }
      if s.kind is StClassSet {
        if !isnull(s.class_set) && !isnull(s.class_set!.value) {
          let val = s.class_set!.value!
          if !isnull(val.typ) && is_rc_class_type(prog, val.typ) {
            emit_ref_incr(s.class_set!.value, val.typ!.name, mut result)
          }
        }
      }
      if s.kind is StStructSet {
        if !isnull(s.struct_set) && !isnull(s.struct_set!.value) {
          let val = s.struct_set!.value!
          if !isnull(val.typ) && is_rc_class_type(prog, val.typ) {
            emit_ref_incr(s.struct_set!.value, val.typ!.name, mut result)
          }
        }
      }



      // Before returns, free all in-scope owned slice locals (except the returned one)
      // and release all class handle locals (except the returned one)
      if s.kind is StReturn {
        let ret_names = get_return_var_names(s)
        emit_slice_frees(slice_locals, slice_types, ret_names, mut result)
        emit_class_releases(class_locals, class_types, ret_names, mut result)
      }

      // Process each statement; may expand to multiple
      let expanded = slab_rewrite_one_stmt(s, all_stmts, escape_map, prog)
      let mut j = 0
      while j < len(expanded) {
        result.push(expanded[j])
        j = j + 1
      }
    }
    i = i + 1
  }
  // At scope exit, free all owned slice locals and release all class locals
  let no_skip: [string] = []
  emit_slice_frees(slice_locals, slice_types, no_skip, mut result)
  emit_class_releases(class_locals, class_types, no_skip, mut result)
  return result
}

// Rewrite a single statement, returning one or more stmts.
// StTempDef with ExClassAlloc expands into alloc + field sets (StSlabSet).
func slab_rewrite_one_stmt(s: LStmt, all_stmts: [LStmt?], escape_map: Dict<Sym, bool>, prog: LProgram) -> [LStmt?] {
  let mut out: [LStmt?] = []

  // StTempDef with ExClassAlloc → slab alloc + StSlabSet for each field
  if s.kind is StTempDef {
    if !isnull(s.temp_def) && !isnull(s.temp_def!.expr) {
      let e = s.temp_def!.expr!
      if e.kind is ExClassAlloc {
        let d = e.class_alloc!
        let temp_id = s.temp_def!.id

        // Rewrite the expr to ExSlabAlloc (no fields — just returns ptr)
        e.kind = ExSlabAlloc
        e.slab_alloc = LSlabAllocData {
          class_name: d.class_name,
          fields: [],
        }
        e.class_alloc = null
        out.push(s)

        // Emit StSlabSet for each field (ptr->field = val)
        let mut fi = 0
        while fi < len(d.fields) {
          let f = d.fields[fi]
          let handle = LValue {
            kind: ValTemp,
            name: "",
            temp_id: temp_id,
            int_val: 0,
            uint_val: 0,
            float_val: 0.0,
            str_val: "",
            bool_val: false,
            typ: e.typ,
            collection: null,
            index: null,
          }
          let set_stmt = LStmt {
            kind: StSlabSet,
            slab_set: LSlabSetData {
              class_name: d.class_name,
              field: f.name,
              handle: handle,
              value: f.value,
            },
          }
          out.push(set_stmt)
          fi = fi + 1
        }
        return out
      } else {
        slab_rewrite_expr(e, all_stmts, escape_map, prog)
      }
    }
  }

  if s.kind is StVarDecl {
    if !isnull(s.var_decl) && !isnull(s.var_decl!.init) {
      slab_rewrite_value(s.var_decl!.init!)
    }
  }
  if s.kind is StAssign {
    if !isnull(s.assign) && !isnull(s.assign!.value) {
      slab_rewrite_value(s.assign!.value!)
    }
  }
  if s.kind is StReturn {
    if !isnull(s.ret) {
      let mut i = 0
      while i < len(s.ret!.values) {
        if !isnull(s.ret!.values[i]) {
          slab_rewrite_value(s.ret!.values[i]!)
        }
        i = i + 1
      }
    }
  }

  // Recurse into sub-statements that contain expressions
  if s.kind is StSideEffect {
    if !isnull(s.side_effect) && !isnull(s.side_effect!.expr) {
      slab_rewrite_expr(s.side_effect!.expr!, all_stmts, escape_map, prog)
    }
  }

  // Recurse into sub-statements that contain blocks
  // NOTE: LIfData, LWhileData, LForData, etc. are structs (value types).
  // s.if_data! gives a COPY. Must read → modify → write back.
  if s.kind is StIf {
    if !isnull(s.if_data) {
      let mut d = s.if_data!
      if !isnull(d.cond) {
        slab_rewrite_value(d.cond!)
      }
      d.then_body = slab_rewrite_stmts(d.then_body, all_stmts, escape_map, prog)
      d.else_body = slab_rewrite_stmts(d.else_body, all_stmts, escape_map, prog)
      s.if_data = d
    }
  }
  if s.kind is StWhile {
    if !isnull(s.while_data) {
      let mut d = s.while_data!
      d.cond_block = slab_rewrite_stmts(d.cond_block, all_stmts, escape_map, prog)
      if !isnull(d.cond_var) {
        slab_rewrite_value(d.cond_var!)
      }
      d.body = slab_rewrite_stmts(d.body, all_stmts, escape_map, prog)
      s.while_data = d
    }
  }
  if s.kind is StFor {
    if !isnull(s.for_data) {
      let mut d = s.for_data!
      if !isnull(d.collection) {
        slab_rewrite_value(d.collection!)
      }
      d.body = slab_rewrite_stmts(d.body, all_stmts, escape_map, prog)
      s.for_data = d
    }
  }
  if s.kind is StBlock {
    if !isnull(s.block) {
      let mut d = s.block!
      d.stmts = slab_rewrite_stmts(d.stmts, all_stmts, escape_map, prog)
      s.block = d
    }
  }
  if s.kind is StSwitch {
    if !isnull(s.switch_data) {
      let mut d = s.switch_data!
      if !isnull(d.tag) {
        slab_rewrite_value(d.tag!)
      }
      let mut ci = 0
      while ci < len(d.cases) {
        d.cases[ci].body = slab_rewrite_stmts(d.cases[ci].body, all_stmts, escape_map, prog)
        ci = ci + 1
      }
      s.switch_data = d
    }
  }
  if s.kind is StTypeSwitch {
    if !isnull(s.type_switch) {
      let mut d = s.type_switch!
      if !isnull(d.value) {
        slab_rewrite_value(d.value!)
      }
      let mut ci = 0
      while ci < len(d.cases) {
        d.cases[ci].body = slab_rewrite_stmts(d.cases[ci].body, all_stmts, escape_map, prog)
        ci = ci + 1
      }
      s.type_switch = d
    }
  }
  if s.kind is StSpawn {
    if !isnull(s.spawn_data) {
      let mut d = s.spawn_data!
      d.body = slab_rewrite_stmts(d.body, all_stmts, escape_map, prog)
      s.spawn_data = d
    }
  }
  if s.kind is StSelect {
    if !isnull(s.select_data) {
      let mut d = s.select_data!
      let mut ci = 0
      while ci < len(d.cases) {
        let c = d.cases[ci]
        if !isnull(c.channel) {
          slab_rewrite_value(c.channel!)
        }
        if !isnull(c.value) {
          slab_rewrite_value(c.value!)
        }
        d.cases[ci].body = slab_rewrite_stmts(d.cases[ci].body, all_stmts, escape_map, prog)
        ci = ci + 1
      }
      s.select_data = d
    }
  }
  if s.kind is StLock {
    if !isnull(s.lock_data) {
      let mut d = s.lock_data!
      if !isnull(d.mutex) {
        slab_rewrite_value(d.mutex!)
      }
      d.body = slab_rewrite_stmts(d.body, all_stmts, escape_map, prog)
      s.lock_data = d
    }
  }
  if s.kind is StDefer {
    if !isnull(s.defer_data) {
      s.defer_data!.body = slab_rewrite_stmts(s.defer_data!.body, all_stmts, escape_map, prog)
    }
  }
  if s.kind is StMultiAssign {
    if !isnull(s.multi_assign) && !isnull(s.multi_assign!.expr) {
      slab_rewrite_expr(s.multi_assign!.expr!, all_stmts, escape_map, prog)
    }
  }
  if s.kind is StSend {
    if !isnull(s.send_data) {
      if !isnull(s.send_data!.channel) {
        slab_rewrite_value(s.send_data!.channel!)
      }
      if !isnull(s.send_data!.value) {
        slab_rewrite_value(s.send_data!.value!)
      }
    }
  }
  if s.kind is StYield {
    if !isnull(s.yield_data) && !isnull(s.yield_data!.value) {
      slab_rewrite_value(s.yield_data!.value!)
    }
  }
  if s.kind is StStructSet {
    if !isnull(s.struct_set) {
      if !isnull(s.struct_set!.receiver) {
        slab_rewrite_value(s.struct_set!.receiver!)
      }
      if !isnull(s.struct_set!.value) {
        slab_rewrite_value(s.struct_set!.value!)
      }
    }
  }
  if s.kind is StIndexSet {
    if !isnull(s.index_set) {
      let is_data = s.index_set!
      if !isnull(is_data.collection) {
        slab_rewrite_value(is_data.collection!)
      }
      if !isnull(is_data.index) {
        slab_rewrite_value(is_data.index!)
      }
      if !isnull(is_data.value) {
        slab_rewrite_value(is_data.value!)
      }
    }
  }

  out.push(s)
  return out
}

// Rewrite a single expression, converting class allocs to slab allocs
func slab_rewrite_expr(e: LExpr, all_stmts: [LStmt?], escape_map: Dict<Sym, bool>, prog: LProgram) {
  match e.kind {
    ExClassAlloc => {
      if !isnull(e.class_alloc) {
        let d = e.class_alloc!
        e.kind = ExSlabAlloc
        e.slab_alloc = LSlabAllocData {
          class_name: d.class_name,
          fields: d.fields,
        }
        e.class_alloc = null
        // Recurse into fields
        let mut i = 0
        while i < len(e.slab_alloc!.fields) {
          if !isnull(e.slab_alloc!.fields[i].value) {
            slab_rewrite_value(e.slab_alloc!.fields[i].value!)
          }
          i = i + 1
        }
      }
    }
    ExCall => {
      if !isnull(e.call) {
        let mut i = 0
        while i < len(e.call!.args) {
          if !isnull(e.call!.args[i]) {
            slab_rewrite_value(e.call!.args[i]!)
          }
          i = i + 1
        }
      }
    }
    ExMethodCall => {
      if !isnull(e.method_call) {
        let mc = e.method_call!
        if !isnull(mc.receiver) {
          slab_rewrite_value(mc.receiver!)
        }
        let mut i = 0
        while i < len(mc.args) {
          if !isnull(mc.args[i]) {
            slab_rewrite_value(mc.args[i]!)
          }
          i = i + 1
        }
      }
    }
    ExBinOp => {
      if !isnull(e.bin_op) {
        if !isnull(e.bin_op!.left) {
          slab_rewrite_value(e.bin_op!.left!)
        }
        if !isnull(e.bin_op!.right) {
          slab_rewrite_value(e.bin_op!.right!)
        }
      }
    }
    ExUnOp => {
      if !isnull(e.un_op) {
        if !isnull(e.un_op!.operand) {
          slab_rewrite_value(e.un_op!.operand!)
        }
      }
    }
    ExCast => {
      if !isnull(e.cast) {
        if !isnull(e.cast!.operand) {
          slab_rewrite_value(e.cast!.operand!)
        }
      }
    }
    ExBuiltin => {
      if !isnull(e.builtin) {
        let mut i = 0
        while i < len(e.builtin!.args) {
          if !isnull(e.builtin!.args[i]) {
            slab_rewrite_value(e.builtin!.args[i]!)
          }
          i = i + 1
        }
      }
    }
    ExMakeSlice => {
      if !isnull(e.builtin) {
        let mut i = 0
        while i < len(e.builtin!.args) {
          if !isnull(e.builtin!.args[i]) {
            slab_rewrite_value(e.builtin!.args[i]!)
          }
          i = i + 1
        }
      }
    }
    ExMakeMap => {
      if !isnull(e.builtin) {
        let mut i = 0
        while i < len(e.builtin!.args) {
          if !isnull(e.builtin!.args[i]) {
            slab_rewrite_value(e.builtin!.args[i]!)
          }
          i = i + 1
        }
      }
    }
    ExStructLit => {
      if !isnull(e.struct_lit) {
        let mut i = 0
        while i < len(e.struct_lit!.fields) {
          if !isnull(e.struct_lit!.fields[i].value) {
            slab_rewrite_value(e.struct_lit!.fields[i].value!)
          }
          i = i + 1
        }
      }
    }
    ExStructField => {
      if !isnull(e.struct_field) {
        if !isnull(e.struct_field!.receiver) {
          slab_rewrite_value(e.struct_field!.receiver!)
        }
      }
    }
    ExIndexGet => {
      if !isnull(e.index_get) {
        if !isnull(e.index_get!.collection) {
          slab_rewrite_value(e.index_get!.collection!)
        }
        if !isnull(e.index_get!.index) {
          slab_rewrite_value(e.index_get!.index!)
        }
      }
    }
    ExSlice => {
      if !isnull(e.slice_data) {
        if !isnull(e.slice_data!.collection) {
          slab_rewrite_value(e.slice_data!.collection!)
        }
        if !isnull(e.slice_data!.low) {
          slab_rewrite_value(e.slice_data!.low!)
        }
        if !isnull(e.slice_data!.high) {
          slab_rewrite_value(e.slice_data!.high!)
        }
      }
    }
    ExWrapOptional => {
      if !isnull(e.wrap_opt) {
        if !isnull(e.wrap_opt!.value) {
          slab_rewrite_value(e.wrap_opt!.value!)
        }
      }
    }
    ExUnwrapOptional => {
      if !isnull(e.unwrap_opt) {
        if !isnull(e.unwrap_opt!.value) {
          slab_rewrite_value(e.unwrap_opt!.value!)
        }
      }
    }
    ExIsNull => {
      if !isnull(e.is_null) {
        if !isnull(e.is_null!.value) {
          slab_rewrite_value(e.is_null!.value!)
        }
      }
    }
    ExVariantConstruct => {
      if !isnull(e.variant_construct) {
        let mut i = 0
        while i < len(e.variant_construct!.fields) {
          if !isnull(e.variant_construct!.fields[i]) {
            slab_rewrite_value(e.variant_construct!.fields[i]!)
          }
          i = i + 1
        }
      }
    }
    ExVariantTag => {
      if !isnull(e.variant_tag) {
        if !isnull(e.variant_tag!.value) {
          slab_rewrite_value(e.variant_tag!.value!)
        }
      }
    }
    ExVariantData => {
      if !isnull(e.variant_data) {
        if !isnull(e.variant_data!.value) {
          slab_rewrite_value(e.variant_data!.value!)
        }
      }
    }
    ExExtractValue => {
      if !isnull(e.extract_value) {
        if !isnull(e.extract_value!.value) {
          slab_rewrite_value(e.extract_value!.value!)
        }
      }
    }
    ExExtractError => {
      if !isnull(e.extract_error) {
        if !isnull(e.extract_error!.value) {
          slab_rewrite_value(e.extract_error!.value!)
        }
      }
    }
    ExMakeResult => {
      if !isnull(e.make_result) {
        if !isnull(e.make_result!.value) {
          slab_rewrite_value(e.make_result!.value!)
        }
        if !isnull(e.make_result!.err) {
          slab_rewrite_value(e.make_result!.err!)
        }
      }
    }
    ExFormat => {
      if !isnull(e.format) {
        let mut i = 0
        while i < len(e.format!.parts) {
          if !e.format!.parts[i].is_literal {
            if !isnull(e.format!.parts[i].value) {
              slab_rewrite_value(e.format!.parts[i].value!)
            }
          }
          i = i + 1
        }
      }
    }
    ExEnvGet => {
      if !isnull(e.env_get) {
        if !isnull(e.env_get!.env) {
          slab_rewrite_value(e.env_get!.env!)
        }
      }
    }
    ExFuncRef => {
      if !isnull(e.func_ref) {
        if !isnull(e.func_ref!.env) {
          slab_rewrite_value(e.func_ref!.env!)
        }
      }
    }
    ExFuncLit => {
      if !isnull(e.func_lit) {
        e.func_lit!.body = slab_rewrite_stmts(e.func_lit!.body, e.func_lit!.body, escape_map, prog)
      }
    }
    ExMakeChannel => {
      if !isnull(e.make_channel) {
        if !isnull(e.make_channel!.buf_size) {
          slab_rewrite_value(e.make_channel!.buf_size!)
        }
      }
    }
    _ => {}
  }
}

func slab_rewrite_value(v: LValue) {
  if !isnull(v.collection) {
    slab_rewrite_value(v.collection!)
  }
  if !isnull(v.index) {
    slab_rewrite_value(v.index!)
  }
}

// ==========================================================================
// Helpers
// ==========================================================================

// Check if an expression produces a freshly-allocated string.
// Only expressions that ALWAYS allocate new backing are considered fresh.
// Function/method calls are excluded because they may return borrowed references
// to existing strings (e.g., c_safe_name returns its input unchanged sometimes).
func is_fresh_string_expr(e: LExpr) -> bool {
  // f-strings produce freshly allocated strings via sprintf
  if e.kind is ExFormat {
    return true
  }
  // String concatenation (string + string) always allocates new backing
  if e.kind is ExBinOp {
    if !isnull(e.typ) && e.typ!.kind is TyString {
      return true
    }
  }
  return false
}

// Check if a VarDecl's init value points to a fresh allocation (ExMakeSlice temp).
func is_fresh_slice_init(init: LValue?, fresh_temps: [i32]) -> bool {
  if isnull(init) {
    return false
  }
  let v = init!
  if v.kind is ValTemp {
    let mut i = 0
    while i < len(fresh_temps) {
      if fresh_temps[i] == v.temp_id {
        return true
      }
      i = i + 1
    }
  }
  return false
}

// Check if a VarDecl/Assign init value points to a fresh class alloc (ExSlabAlloc/ExClassAlloc temp).
func is_fresh_class_init(init: LValue?, fresh_class_temps: [i32]) -> bool {
  if isnull(init) {
    return false
  }
  let v = init!
  if v.kind is ValTemp {
    let mut i = 0
    while i < len(fresh_class_temps) {
      if fresh_class_temps[i] == v.temp_id {
        return true
      }
      i = i + 1
    }
  }
  // Null literal — no retain needed
  if v.kind is ValLitNull {
    return true
  }
  return false
}

// Get the variable name being returned (if it's a simple variable return).
// Returns "" if it's not a simple variable or if there are multiple return values.
func get_return_var_names(s: LStmt) -> [string] {
  let mut names: [string] = []
  if !isnull(s.ret) {
    let mut i = 0
    while i < len(s.ret!.values) {
      if !isnull(s.ret!.values[i]) {
        let v = s.ret!.values[i]!
        if v.kind is ValVar {
          names.push(v.name)
        }
      }
      i = i + 1
    }
  }
  return names
}

// Emit StSliceFree statements for all tracked slice locals, skipping skip_name.
func emit_slice_frees(names: [string], types: [LType?], skip_names: [string], mut out: [LStmt?]) {
  let mut i = 0
  while i < len(names) {
    if !should_skip(names[i], skip_names) {
      let free_stmt = LStmt {
        kind: StSliceFree,
        slice_free: LSliceFreeData {
          name: names[i],
          typ: types[i],
        },
      }
      out.push(free_stmt)
    }
    i = i + 1
  }
}

func should_skip(name: string, skip_names: [string]) -> bool {
  let mut i = 0
  while i < len(skip_names) {
    if name == skip_names[i] {
      return true
    }
    i = i + 1
  }
  return false
}

// Check if a class is owned (lifetime managed by parent destructor, no RC).
func is_owned_class(prog: LProgram, class_name: string) -> bool {
  if isnull(prog.owned_classes) { return false }
  let entry = prog.owned_classes!.get(sym(class_name))
  return !isnull(entry)
}

// Check if a class is marked permanent (never freed).
func is_permanent_class(prog: LProgram, class_name: string) -> bool {
  let mut i = 0
  while i < len(prog.classes) {
    if prog.classes[i].name == class_name {
      return prog.classes[i].is_permanent
    }
    i = i + 1
  }
  return false
}

// Check if a type is a non-owned class handle (needs RC).
func is_rc_class_type(prog: LProgram, typ: LType?) -> bool {
  if isnull(typ) { return false }
  if !(typ!.kind is TyClassHandle) { return false }
  // Fast path: resolved class_decl reference
  if !isnull(typ!.class_decl) {
    if typ!.class_decl!.is_permanent { return false }
    if typ!.class_decl!.is_owned { return false }
    return true
  }
  // Fallback: direct flag check (set by resolve_class_types)
  if typ!.is_permanent { return false }
  if typ!.is_owned { return false }
  // Final fallback: dict lookup
  if is_permanent_class(prog, typ!.name) { return false }
  return !is_owned_class(prog, typ!.name)
}

// Emit StRefIncr for a class handle value.
func emit_ref_incr(handle: LValue?, class_name: string, mut out: [LStmt?]) {
  out.push(LStmt {
    kind: StRefIncr,
    ref_incr: LRefIncrData {
      handle: handle,
      class_name: class_name,
    },
  })
}

// Emit StRefDecr for a class handle value.
func emit_ref_decr(handle: LValue?, class_name: string, mut out: [LStmt?]) {
  out.push(LStmt {
    kind: StRefDecr,
    ref_decr: LRefDecrData {
      handle: handle,
      class_name: class_name,
    },
  })
}

// Emit StRefDecr for all tracked class locals, skipping skip_names.
func emit_class_releases(names: [string], types: [LType?], skip_names: [string], mut out: [LStmt?]) {
  let mut i = 0
  while i < len(names) {
    if !should_skip(names[i], skip_names) {
      let handle = LValue {
        kind: ValVar,
        name: names[i],
        temp_id: 0,
        typ: types[i],
      }
      emit_ref_decr(handle, types[i]!.name, mut out)
    }
    i = i + 1
  }
}
