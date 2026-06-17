// monomorphizer.ly — Bootstrap port of pkg/lir/monomorphize.go
// Performs LIR→LIR pass that replaces all generic declarations with specialized
// copies for each unique set of concrete type arguments found at call sites.
// After this pass, no LTyTypeVar remains in the program.

// --- Instance tracking ---
// We use Dict<Sym, string> with composite keys "name|typekey" to track instances.
// Value is the mangled name. Separate dicts for func/class/struct instances.
// Type args stored in a parallel dict keyed the same way.

class MonoPass {
  prog: LProgram?

  // Instance tracking: "name|typekey" → mangled name
  func_instances: Dict<Sym, string>?
  class_instances: Dict<Sym, string>?
  struct_instances: Dict<Sym, string>?

  // Type args: "name|typekey" → serialized types (we store the LType? list)
  // Since we can't store [LType?] in Dict, use a separate approach:
  // func_inst_types: "name|typekey|i" → LType?
  func_inst_types: Dict<Sym, LType?>?
  class_inst_types: Dict<Sym, LType?>?
  struct_inst_types: Dict<Sym, LType?>?

  // Count of type args per instance
  func_inst_count: Dict<Sym, i32>?
  class_inst_count: Dict<Sym, i32>?
  struct_inst_count: Dict<Sym, i32>?

  // Class renames: generic name → mangled name
  class_renames: Dict<Sym, string>?

  // Per-function rename map during Phase 4
  current_class_rename_map: Dict<Sym, string>?

  // Indexes
  func_by_name: Dict<Sym, LFuncDecl>?
  class_by_name: Dict<Sym, LClassDecl>?
  struct_by_name: Dict<Sym, LStructDecl>?
  methods_by_class: Dict<Sym, [LFuncDecl]>?

  // Track all generic names for instance iteration
  func_inst_names: [string]
  class_inst_names: [string]
  struct_inst_names: [string]
}

func new_mono_pass(prog: LProgram?) -> MonoPass? {
  return MonoPass {
    prog: prog,
    func_instances: Dict<Sym, string>(),
    class_instances: Dict<Sym, string>(),
    struct_instances: Dict<Sym, string>(),
    func_inst_types: Dict<Sym, LType?>(),
    class_inst_types: Dict<Sym, LType?>(),
    struct_inst_types: Dict<Sym, LType?>(),
    func_inst_count: Dict<Sym, i32>(),
    class_inst_count: Dict<Sym, i32>(),
    struct_inst_count: Dict<Sym, i32>(),
    class_renames: Dict<Sym, string>(),
    current_class_rename_map: Dict<Sym, string>(),
    func_by_name: Dict<Sym, LFuncDecl>(),
    class_by_name: Dict<Sym, LClassDecl>(),
    struct_by_name: Dict<Sym, LStructDecl>(),
    methods_by_class: Dict<Sym, [LFuncDecl]>(),
    func_inst_names: [],
    class_inst_names: [],
    struct_inst_names: [],
  }
}


// =========================================================================
// Post-monomorphization validator
// =========================================================================

// validate_post_mono checks that no generic functions, classes, or type
// variables remain after monomorphization. Panics on first violation.
func validate_post_mono(prog: LProgram?) {
  if isnull(prog) { return }

  // Check no generic functions remain
  for f in prog!.functions {
    if len(f.type_params) > 0 {
      eprintln(f"post-mono: function {f.name} still has type_params")
      os_exit(1)
    }
    for p in f.params {
      if p.typ != null {
        check_no_typevar(f.name, f"param {p.name}", p.typ!)
      }
    }
    if f.return_type != null {
      check_no_typevar(f.name, "return", f.return_type!)
    }
    check_stmts_no_typevar(f.name, f.body)
  }

  // Check no generic classes remain
  for c in prog!.classes {
    if len(c.type_params) > 0 {
      eprintln(f"post-mono: class {c.name} still has type_params")
      os_exit(1)
    }
  }

  // Check no generic structs remain
  for s in prog!.structs {
    if len(s.type_params) > 0 {
      eprintln(f"post-mono: struct {s.name} still has type_params")
      os_exit(1)
    }
  }
}

func check_no_typevar(func_name: string, context: string, t: LType) {
  match t.kind {
    TyTypeVar => {
      eprintln(f"post-mono: {func_name} {context} has unresolved type var '{t.name}'")
      os_exit(1)
    }
    TyOptional | TySlice | TyChannel | TyGenerator | TyErrorResult => {
      if t.elem != null { check_no_typevar(func_name, context, t.elem!) }
    }
    TyMap => {
      if t.key != null { check_no_typevar(func_name, context, t.key!) }
      if t.elem != null { check_no_typevar(func_name, context, t.elem!) }
    }
    TyFuncPtr => {
      if t.ret != null { check_no_typevar(func_name, context, t.ret!) }
      for p in t.params {
        if p != null { check_no_typevar(func_name, context, p!) }
      }
    }
    TyTuple => {
      for field in t.fields {
        if field.typ != null { check_no_typevar(func_name, context, field.typ!) }
      }
    }
    _ => {}
  }
  for ta in t.type_args {
    if ta != null { check_no_typevar(func_name, context, ta!) }
  }
}

func check_stmts_no_typevar(func_name: string, stmts: [LStmt?]) {
  for s in stmts {
    if isnull(s) { continue }
    match s!.kind {
      StTempDef => {
        if s!.temp_def != null && s!.temp_def!.expr != null {
          if s!.temp_def!.expr!.typ != null {
            check_no_typevar(func_name, f"_t{itoa(s!.temp_def!.id as i64)}", s!.temp_def!.expr!.typ!)
          }
        }
      }
      StVarDecl => {
        if s!.var_decl != null && s!.var_decl!.typ != null {
          check_no_typevar(func_name, s!.var_decl!.name, s!.var_decl!.typ!)
        }
      }
      StIf => {
        if s!.if_data != null {
          check_stmts_no_typevar(func_name, s!.if_data!.then_body)
          check_stmts_no_typevar(func_name, s!.if_data!.else_body)
        }
      }
      StWhile => {
        if s!.while_data != null {
          check_stmts_no_typevar(func_name, s!.while_data!.body)
        }
      }
      StFor => {
        if s!.for_data != null {
          check_stmts_no_typevar(func_name, s!.for_data!.body)
        }
      }
      StBlock => {
        if s!.block != null {
          check_stmts_no_typevar(func_name, s!.block!.stmts)
        }
      }
      StSwitch => {
        if s!.switch_data != null {
          for c in s!.switch_data!.cases {
            check_stmts_no_typevar(func_name, c.body)
          }
        }
      }
      _ => {}
    }
  }
}

// func_key returns the lookup key for a function.

func MonoPass.func_key(self, f: LFuncDecl) -> string {
  if f.receiver != "" {
    return f"{f.receiver}.{f.name}"
  }
  return f.name
}

// inst_key builds "name|typekey"
func inst_key(name: string, type_key_str: string) -> string {
  return f"{name}|{type_key_str}"
}

// record_func_instance records a function instantiation.
func MonoPass.record_func_instance(self, name: string, types: [LType?]) {
  let tk = type_key(types)
  let key = inst_key(name, tk)
  let entry = self.func_instances!.get(sym(key))
  if !isnull(entry) {
    return
  }
  let mangled = mangle_name(name, types)
  self.func_instances!.set(sym(key), mangled)
  // Store type args
  let count = len(types)
  self.func_inst_count!.set(sym(key), count as i32)
  let mut i = 0
  while i < count {
    self.func_inst_types!.set(sym(f"{key}|{i}"), types[i])
    i = i + 1
  }
  // Track name for iteration
  let has_name = has_inst_name(self.func_inst_names, name)
  if !has_name {
    append(self.func_inst_names, name)
  }
}

func MonoPass.record_class_instance(self, name: string, types: [LType?]) {
  let tk = type_key(types)
  let key = inst_key(name, tk)
  let entry = self.class_instances!.get(sym(key))
  if !isnull(entry) {
    return
  }
  let mangled = mangle_name(name, types)
  self.class_instances!.set(sym(key), mangled)
  let count = len(types)
  self.class_inst_count!.set(sym(key), count as i32)
  let mut i = 0
  while i < count {
    self.class_inst_types!.set(sym(f"{key}|{i}"), types[i])
    i = i + 1
  }
  let has_name = has_inst_name(self.class_inst_names, name)
  if !has_name {
    append(self.class_inst_names, name)
  }
}

func MonoPass.record_struct_instance(self, name: string, types: [LType?]) {
  let tk = type_key(types)
  let key = inst_key(name, tk)
  let entry = self.struct_instances!.get(sym(key))
  if !isnull(entry) {
    return
  }
  let mangled = mangle_name(name, types)
  self.struct_instances!.set(sym(key), mangled)
  let count = len(types)
  self.struct_inst_count!.set(sym(key), count as i32)
  let mut i = 0
  while i < count {
    self.struct_inst_types!.set(sym(f"{key}|{i}"), types[i])
    i = i + 1
  }
  let has_name = has_inst_name(self.struct_inst_names, name)
  if !has_name {
    append(self.struct_inst_names, name)
  }
}

func has_inst_name(names: [string], name: string) -> bool {
  let mut i = 0
  while i < len(names) {
    if names[i] == name {
      return true
    }
    i = i + 1
  }
  return false
}

// get_inst_types retrieves stored type args for an instance key.
func get_inst_types(types_dict: Dict<Sym, LType?>?, count_dict: Dict<Sym, i32>?, key: string) -> [LType?] {
  let count_entry = count_dict!.get(sym(key))
  if isnull(count_entry) {
    return []
  }
  let count = count_entry!.value
  let mut result: [LType?] = []
  let mut i: i32 = 0
  while i < count {
    let t_entry = types_dict!.get(sym(f"{key}|{i}"))
    if !isnull(t_entry) {
      append(result, t_entry!.value)
    }
    i = i + 1
  }
  return result
}

// ---------------------------------------------------------------------------
// Phase 1: Collect instantiations
// ---------------------------------------------------------------------------

func MonoPass.collect_from_type(self, t: LType?) {
  if isnull(t) {
    return
  }
  match t!.kind {
    TyClassHandle => {
      if len(t!.type_args) > 0 && !has_type_vars(t!.type_args) {
        self.record_class_instance(t!.name, t!.type_args)
      }
      let mut i = 0
      while i < len(t!.type_args) {
        self.collect_from_type(t!.type_args[i])
        i = i + 1
      }
    }
    TyStruct => {
      if len(t!.type_args) > 0 && !has_type_vars(t!.type_args) {
        self.record_struct_instance(t!.name, t!.type_args)
      }
      let mut i = 0
      while i < len(t!.type_args) {
        self.collect_from_type(t!.type_args[i])
        i = i + 1
      }
    }
    TyOptional => {
      self.collect_from_type(t!.elem)
    }
    TySlice => {
      self.collect_from_type(t!.elem)
    }
    TyFuncPtr => {
      let mut i = 0
      while i < len(t!.params) {
        self.collect_from_type(t!.params[i])
        i = i + 1
      }
      self.collect_from_type(t!.ret)
    }
    TyTaggedUnion => {
      let mut i = 0
      while i < len(t!.variants) {
        let mut j = 0
        while j < len(t!.variants[i].fields) {
          self.collect_from_type(t!.variants[i].fields[j].typ)
          j = j + 1
        }
        i = i + 1
      }
    }
    _ => {}
  }
}

func MonoPass.collect_from_stmts(self, stmts: [LStmt?]) {
  let mut i = 0
  while i < len(stmts) {
    if !isnull(stmts[i]) {
      self.collect_from_stmt(stmts[i]!)
    }
    i = i + 1
  }
}

func MonoPass.collect_from_stmt(self, s: LStmt) {
  match s.kind {
    StTempDef => {
      if !isnull(s.temp_def) {
        if !isnull(s.temp_def!.expr) {
          self.collect_from_expr(s.temp_def!.expr!)
        }
      }
    }
    StIf => {
      if !isnull(s.if_data) {
        self.collect_from_stmts(s.if_data!.then_body)
        self.collect_from_stmts(s.if_data!.else_body)
      }
    }
    StWhile => {
      if !isnull(s.while_data) {
        self.collect_from_stmts(s.while_data!.cond_block)
        self.collect_from_stmts(s.while_data!.body)
      }
    }
    StFor => {
      if !isnull(s.for_data) {
        self.collect_from_stmts(s.for_data!.body)
      }
    }
    StSwitch => {
      if !isnull(s.switch_data) {
        let mut i = 0
        while i < len(s.switch_data!.cases) {
          self.collect_from_stmts(s.switch_data!.cases[i].body)
          i = i + 1
        }
      }
    }
    StTypeSwitch => {
      if !isnull(s.type_switch) {
        let mut j = 0
        while j < len(s.type_switch!.cases) {
          self.collect_from_stmts(s.type_switch!.cases[j].body)
          j = j + 1
        }
      }
    }
    StBlock => {
      if !isnull(s.block) {
        self.collect_from_stmts(s.block!.stmts)
      }
    }
    StSideEffect => {
      if !isnull(s.side_effect) {
        if !isnull(s.side_effect!.expr) {
          self.collect_from_expr(s.side_effect!.expr!)
        }
      }
    }
    StMultiAssign => {
      if !isnull(s.multi_assign) {
        if !isnull(s.multi_assign!.expr) {
          self.collect_from_expr(s.multi_assign!.expr!)
        }
      }
    }
    StSpawn => {
      if !isnull(s.spawn_data) {
        self.collect_from_stmts(s.spawn_data!.body)
      }
    }
    StSelect => {
      if !isnull(s.select_data) {
        let mut i = 0
        while i < len(s.select_data!.cases) {
          self.collect_from_stmts(s.select_data!.cases[i].body)
          i = i + 1
        }
      }
    }
    StDefer => {
      if !isnull(s.defer_data) {
        self.collect_from_stmts(s.defer_data!.body)
      }
    }
    StLock => {
      if !isnull(s.lock_data) {
        self.collect_from_stmts(s.lock_data!.body)
      }
    }
    _ => {}
  }
}

func MonoPass.collect_from_expr(self, e: LExpr) {
  match e.kind {
    ExCall => {
      if !isnull(e.call) {
        if len(e.call!.type_args) > 0 {
          if !has_type_vars(e.call!.type_args) {
            self.record_func_instance(e.call!.func_name, e.call!.type_args)
          }
        }
      }
    }
    ExMethodCall => {
      if !isnull(e.method_call) {
        let mc = e.method_call!
        if !isnull(mc.receiver) {
          if !isnull(mc.receiver!.typ) {
            if mc.receiver!.typ!.kind is TyClassHandle {
              let recv_name = mc.receiver!.typ!.name
              let cls_entry = self.class_by_name!.get(sym(recv_name))
              if !isnull(cls_entry) {
                if len(cls_entry!.value.type_params) > 0 {
                  if len(mc.type_args) > 0 {
                    if !has_type_vars(mc.type_args) {
                      self.record_class_instance(recv_name, mc.type_args)
                    }
                  }
                }
              }
              // Check for generic function specialization via type args
              if len(mc.type_args) > 0 && !has_type_vars(mc.type_args) {
                let direct_key = recv_name + "." + mc.method
                let direct_entry = self.func_by_name!.get(sym(direct_key))
                if !isnull(direct_entry) && len(direct_entry!.value.type_params) > 0 {
                  self.record_func_instance(direct_key, mc.type_args)
                } else {
                  // Search for generic functions with type-param receivers
                  let mut fi2 = 0
                  while fi2 < len(self.prog!.functions) {
                    let f = self.prog!.functions[fi2]
                    if f.name == mc.method && len(f.type_params) > 0 && len(f.relational_constraints) > 0 {
                      let fkey = self.func_key(f)
                      self.record_func_instance(fkey, mc.type_args)
                    }
                    fi2 = fi2 + 1
                  }
                }
              }
            }
          }
        }
      }
    }
    ExClassAlloc => {
      if !isnull(e.class_alloc) {
        if len(e.class_alloc!.type_args) > 0 {
          if !has_type_vars(e.class_alloc!.type_args) {
            self.record_class_instance(e.class_alloc!.class_name, e.class_alloc!.type_args)
          }
        }
      }
    }
    ExFuncLit => {
      if !isnull(e.func_lit) {
        self.collect_from_stmts(e.func_lit!.body)
      }
    }
    _ => {}
  }
}

// ---------------------------------------------------------------------------
// Phase 2: Specialization (deep clone + type substitution)
// ---------------------------------------------------------------------------

func MonoPass.specialize_func(self, orig: LFuncDecl, subst: Dict<Sym, LType?>?, new_name: string, new_receiver: string) -> LFuncDecl {
  let mut params: [LParam] = []
  let mut i = 0
  while i < len(orig.params) {
    append(params, LParam { name: orig.params[i].name, typ: subst_type(orig.params[i].typ, subst), mutable: orig.params[i].mutable })
    i = i + 1
  }

  let body = clone_stmts(orig.body, subst)
  let ret_type = subst_type(orig.return_type, subst)

  let mut recv = ""
  if new_receiver != "" {
    recv = new_receiver
  }

  // Build class rename map
  let rename_map = self.build_class_rename_map(subst)

  let mut spec = LFuncDecl {
    name: new_name,
    type_params: [],
    params: params,
    return_type: ret_type,
    body: body,
    is_exported: orig.is_exported,
    receiver: recv,
    receiver_type_params: [],
    relational_constraints: orig.relational_constraints,
    class_rename_map: rename_map,
  }

  // Rewrite method calls based on ImplMethodRenames (per-specialization, not post-pass).
  // Matches Go compiler's rewriteImplMethodCalls in specializeFunc.
  if len(orig.relational_constraints) > 0 && !isnull(self.prog!.impl_method_renames) {
    self.rewrite_impl_method_calls(spec, orig.relational_constraints, subst)
  }

  return spec
}

// ---------------------------------------------------------------------------
// Per-specialization impl method call rewriting.
// Matches Go compiler's rewriteImplMethodCalls — uses the full @-delimited
// key (including concrete type args) to find the correct rename, avoiding
// the last-writer-wins bug in the flat post-pass.
// ---------------------------------------------------------------------------

struct ImplRenameEntry {
  class_name: string
  old_method: string
  new_method: string
}

func MonoPass.rewrite_impl_method_calls(self, fn: LFuncDecl, constraints: [LRelationalConstraint], subst: Dict<Sym, LType?>?) {
  let mut renames: [ImplRenameEntry] = []

  let mut ci = 0
  while ci < len(constraints) {
    let rc = constraints[ci]
    // Build key prefix: "InterfaceName@ConcreteArg0@ConcreteArg1@..."
    let mut prefix = rc.interface_name
    let mut ti = 0
    while ti < len(rc.type_args) {
      let ta = rc.type_args[ti]
      if !isnull(subst) {
        let ct = subst!.get(sym(ta))
        if !isnull(ct) && !isnull(ct!.value) {
          if ct!.value!.kind is TyClassHandle {
            prefix = prefix + "@" + ct!.value!.name
          } else {
            prefix = prefix + "@" + ta
          }
        } else {
          prefix = prefix + "@" + ta
        }
      } else {
        prefix = prefix + "@" + ta
      }
      ti = ti + 1
    }

    // Check all rename entries matching this prefix
    let prefix_at = prefix + "@"
    let keys = self.prog!.impl_method_renames!.keys()
    let mut ki = 0
    while ki < len(keys) {
      let key_name = keys[ki].get_name()
      if mono_starts_with(key_name, prefix_at) {
        let suffix = key_name[len(prefix_at):]
        // Parse className@methodName from suffix
        let at_pos = str_index_of(suffix, "@")
        if at_pos >= 0 {
          let class_name = suffix[0:at_pos]
          let method_name = suffix[at_pos + 1:]
          let new_name_entry = self.prog!.impl_method_renames!.get(keys[ki])
          if !isnull(new_name_entry) {
            append(renames, ImplRenameEntry {
              class_name: class_name,
              old_method: method_name,
              new_method: new_name_entry!.value,
            })
          }
        }
      }
      ki = ki + 1
    }
    ci = ci + 1
  }

  if len(renames) == 0 { return }

  rewrite_method_calls_in_stmts(fn.body, renames)
}

func rewrite_method_calls_in_stmts(stmts: [LStmt?], renames: [ImplRenameEntry]) {
  let mut i = 0
  while i < len(stmts) {
    if !isnull(stmts[i]) {
      rewrite_method_calls_in_stmt(stmts[i]!, renames)
    }
    i = i + 1
  }
}

func rewrite_method_calls_in_stmt(s: LStmt, renames: [ImplRenameEntry]) {
  match s.kind {
    StTempDef => {
      if !isnull(s.temp_def) {
        rewrite_method_calls_in_expr(s.temp_def!.expr, renames)
      }
    }
    StExpr => {
      if !isnull(s.side_effect) {
        rewrite_method_calls_in_expr(s.side_effect!.expr, renames)
      }
    }
    StSideEffect => {
      if !isnull(s.side_effect) {
        rewrite_method_calls_in_expr(s.side_effect!.expr, renames)
      }
    }
    StIf => {
      if !isnull(s.if_data) {
        rewrite_method_calls_in_stmts(s.if_data!.then_body, renames)
        rewrite_method_calls_in_stmts(s.if_data!.else_body, renames)
      }
    }
    StWhile => {
      if !isnull(s.while_data) {
        rewrite_method_calls_in_stmts(s.while_data!.cond_block, renames)
        rewrite_method_calls_in_stmts(s.while_data!.body, renames)
      }
    }
    StBlock => {
      if !isnull(s.block) {
        rewrite_method_calls_in_stmts(s.block!.stmts, renames)
      }
    }
    StFor => {
      if !isnull(s.for_data) {
        rewrite_method_calls_in_stmts(s.for_data!.body, renames)
      }
    }
    _ => {}
  }
}

func rewrite_method_calls_in_expr(e: LExpr?, renames: [ImplRenameEntry]) {
  if isnull(e) { return }
  if e!.kind is ExMethodCall {
    if !isnull(e!.method_call) {
      let mc = e!.method_call!
      if !isnull(mc.receiver) && !isnull(mc.receiver!.typ) {
        if mc.receiver!.typ!.kind is TyClassHandle {
          let recv_class = mc.receiver!.typ!.name
          let mut ri = 0
          while ri < len(renames) {
            if renames[ri].class_name == recv_class && renames[ri].old_method == mc.method {
              mc.method = renames[ri].new_method
              return
            }
            ri = ri + 1
          }
        }
      }
    }
  }
  // Also handle ExCall (plain function calls) — setters may be lowered as ExCall
  // Match by func_name against old_method, and check first arg type for class match.
  if e!.kind is ExCall {
    if !isnull(e!.call) {
      let cd = e!.call!
      let mut ri = 0
      while ri < len(renames) {
        if renames[ri].old_method == cd.func_name {
          // Verify first arg is the right class
          if len(cd.args) > 0 && !isnull(cd.args[0]) && !isnull(cd.args[0]!.typ) {
            if cd.args[0]!.typ!.kind is TyClassHandle {
              if cd.args[0]!.typ!.name == renames[ri].class_name {
                cd.func_name = renames[ri].new_method
                return
              }
            }
          }
        }
        ri = ri + 1
      }
    }
  }
}

func MonoPass.specialize_class(self, orig: LClassDecl, subst: Dict<Sym, LType?>?, new_name: string) -> LClassDecl {
  let mut fields: [LField] = []
  let mut i = 0
  while i < len(orig.fields) {
    append(fields, LField { name: orig.fields[i].name, typ: subst_type(orig.fields[i].typ, subst) })
    i = i + 1
  }
  return LClassDecl {
    name: new_name,
    type_params: [],
    fields: fields,
    is_exported: orig.is_exported,
    is_permanent: orig.is_permanent,
    implements: [],
  }
}

func MonoPass.specialize_struct(self, orig: LStructDecl, subst: Dict<Sym, LType?>?, new_name: string) -> LStructDecl {
  let mut fields: [LField] = []
  let mut i = 0
  while i < len(orig.fields) {
    append(fields, LField { name: orig.fields[i].name, typ: subst_type(orig.fields[i].typ, subst) })
    i = i + 1
  }
  return LStructDecl {
    name: new_name,
    type_params: [],
    fields: fields,
    is_exported: orig.is_exported,
  }
}

func MonoPass.build_class_rename_map(self, subst: Dict<Sym, LType?>?) -> Dict<Sym, string>? {
  let renames = Dict<Sym, string>()
  if isnull(subst) {
    return renames
  }
  // Check all generic classes
  let class_names = self.class_by_name!.keys()
  let mut i = 0
  while i < len(class_names) {
    let name = class_names[i]
    let cls_entry = self.class_by_name!.get(name)
    if !isnull(cls_entry) {
      let cls = cls_entry!.value
      if len(cls.type_params) > 0 {
        let mut types: [LType?] = []
        let mut all_resolved = true
        let mut j = 0
        while j < len(cls.type_params) {
          let concrete = subst!.get(sym(cls.type_params[j].name))
          if !isnull(concrete) {
            append(types, concrete!.value)
          } else {
            all_resolved = false
          }
          j = j + 1
        }
        if all_resolved {
          if len(types) == len(cls.type_params) {
            renames.set(name, mangle_name(name.get_name(), types))
          }
        }
      }
    }
    i = i + 1
  }
  // Check all generic structs
  let struct_names = self.struct_by_name!.keys()
  i = 0
  while i < len(struct_names) {
    let sname = struct_names[i]
    let st_entry = self.struct_by_name!.get(sname)
    if !isnull(st_entry) {
      let st = st_entry!.value
      if len(st.type_params) > 0 {
        let mut types: [LType?] = []
        let mut all_resolved = true
        let mut j = 0
        while j < len(st.type_params) {
          let concrete = subst!.get(sym(st.type_params[j].name))
          if !isnull(concrete) {
            append(types, concrete!.value)
          } else {
            all_resolved = false
          }
          j = j + 1
        }
        if all_resolved {
          if len(types) == len(st.type_params) {
            renames.set(sname, mangle_name(sname.get_name(), types))
          }
        }
      }
    }
    i = i + 1
  }
  return renames
}

// ---------------------------------------------------------------------------
// Phase 4: Rewrite call sites
// ---------------------------------------------------------------------------

func MonoPass.rewrite_value(self, v: LValue?) {
  if isnull(v) { return }
  v!.typ = self.subst_type_remove_vars(v!.typ)
  if !isnull(v!.collection) {
    self.rewrite_value(v!.collection!)
  }
  if !isnull(v!.index) {
    self.rewrite_value(v!.index!)
  }
}

func MonoPass.rewrite_stmts(self, stmts: [LStmt?]) {
  let mut i = 0
  while i < len(stmts) {
    if !isnull(stmts[i]) {
      self.rewrite_stmt(stmts[i]!)
    }
    i = i + 1
  }
}

func MonoPass.rewrite_stmt(self, s: LStmt) {
  match s.kind {
    StTempDef => {
      if !isnull(s.temp_def) {
        if !isnull(s.temp_def!.expr) {
          self.rewrite_expr(s.temp_def!.expr!)
          s.temp_def!.expr!.typ = self.subst_type_remove_vars(s.temp_def!.expr!.typ)
        }
      }
    }
    StVarDecl => {
      if !isnull(s.var_decl) {
        // Copy-out/modify/write-back: var_decl is optional struct on class LStmt,
        // so s.var_decl!.typ = x writes to a copy and is lost.
        let mut vd = s.var_decl!
        vd.typ = self.subst_type_remove_vars(vd.typ)
        if !isnull(vd.init) {
          self.rewrite_value(vd.init)
          // Sync VarDecl type from init when init has a more specific mangled type.
          if !isnull(vd.init!.typ) && !isnull(vd.typ) {
            let init_t = vd.init!.typ!
            let decl_t = vd.typ!
            if init_t.kind == decl_t.kind {
              let is_class_or_struct = (decl_t.kind is TyClassHandle || decl_t.kind is TyStruct)
              if is_class_or_struct && init_t.name != decl_t.name {
                if decl_t.name != "" && init_t.name != "" {
                  vd.typ = vd.init!.typ
                }
              }
            }
          }
        }
        s.var_decl = vd
      }
    }
    StAssign => {
      if !isnull(s.assign) {
        if !isnull(s.assign!.value) {
          self.rewrite_value(s.assign!.value)
        }
      }
    }
    StStructSet => {
      if !isnull(s.struct_set) {
        if !isnull(s.struct_set!.receiver) {
          self.rewrite_value(s.struct_set!.receiver)
        }
        if !isnull(s.struct_set!.value) {
          self.rewrite_value(s.struct_set!.value)
        }
      }
    }
    StClassSet => {
      if !isnull(s.class_set) {
        // Copy-out/modify/write-back: class_set is optional struct on class LStmt
        let mut cs = s.class_set!
        if !isnull(cs.handle) {
          self.rewrite_value(cs.handle)
          if !isnull(cs.handle!.typ) {
            if cs.handle!.typ!.kind is TyClassHandle {
              cs.class_name = cs.handle!.typ!.name
            }
          }
        }
        if !isnull(cs.value) {
          self.rewrite_value(cs.value)
        }
        s.class_set = cs
      }
    }
    StIndexSet => {
      if !isnull(s.index_set) {
        if !isnull(s.index_set!.collection) {
          self.rewrite_value(s.index_set!.collection)
        }
        if !isnull(s.index_set!.index) {
          self.rewrite_value(s.index_set!.index)
        }
        if !isnull(s.index_set!.value) {
          self.rewrite_value(s.index_set!.value)
        }
      }
    }
    StReturn => {
      if !isnull(s.ret) {
        let mut i = 0
        while i < len(s.ret!.values) {
          if !isnull(s.ret!.values[i]) {
            self.rewrite_value(s.ret!.values[i])
          }
          i = i + 1
        }
      }
    }
    StIf => {
      if !isnull(s.if_data) {
        if !isnull(s.if_data!.cond) {
          self.rewrite_value(s.if_data!.cond)
        }
        self.rewrite_stmts(s.if_data!.then_body)
        self.rewrite_stmts(s.if_data!.else_body)
      }
    }
    StWhile => {
      if !isnull(s.while_data) {
        self.rewrite_stmts(s.while_data!.cond_block)
        if !isnull(s.while_data!.cond_var) {
          self.rewrite_value(s.while_data!.cond_var)
        }
        self.rewrite_stmts(s.while_data!.body)
      }
    }
    StFor => {
      if !isnull(s.for_data) {
        // Copy-out/modify/write-back: for_data is optional struct on class LStmt
        let mut fd = s.for_data!
        fd.var_type = self.subst_type_remove_vars(fd.var_type)
        if !isnull(fd.collection) {
          self.rewrite_value(fd.collection)
        }
        s.for_data = fd
        self.rewrite_stmts(s.for_data!.body)
      }
    }
    StSwitch => {
      if !isnull(s.switch_data) {
        if !isnull(s.switch_data!.tag) {
          self.rewrite_value(s.switch_data!.tag)
        }
        let mut i = 0
        while i < len(s.switch_data!.cases) {
          self.rewrite_stmts(s.switch_data!.cases[i].body)
          i = i + 1
        }
      }
    }
    StTypeSwitch => {
      if !isnull(s.type_switch) {
        if !isnull(s.type_switch!.value) {
          self.rewrite_value(s.type_switch!.value)
        }
        let mut j = 0
        while j < len(s.type_switch!.cases) {
          s.type_switch!.cases[j].typ = self.subst_type_remove_vars(s.type_switch!.cases[j].typ)
          self.rewrite_stmts(s.type_switch!.cases[j].body)
          j = j + 1
        }
      }
    }
    StBlock => {
      if !isnull(s.block) {
        self.rewrite_stmts(s.block!.stmts)
      }
    }
    StSideEffect => {
      if !isnull(s.side_effect) {
        if !isnull(s.side_effect!.expr) {
          self.rewrite_expr(s.side_effect!.expr!)
        }
      }
    }
    StMultiAssign => {
      if !isnull(s.multi_assign) {
        if !isnull(s.multi_assign!.expr) {
          self.rewrite_expr(s.multi_assign!.expr!)
        }
        let mut i = 0
        while i < len(s.multi_assign!.types) {
          s.multi_assign!.types[i] = self.subst_type_remove_vars(s.multi_assign!.types[i])
          i = i + 1
        }
      }
    }
    StSpawn => {
      if !isnull(s.spawn_data) {
        self.rewrite_stmts(s.spawn_data!.body)
      }
    }
    StSelect => {
      if !isnull(s.select_data) {
        let mut i = 0
        while i < len(s.select_data!.cases) {
          if !isnull(s.select_data!.cases[i].channel) {
            self.rewrite_value(s.select_data!.cases[i].channel)
          }
          if !isnull(s.select_data!.cases[i].value) {
            self.rewrite_value(s.select_data!.cases[i].value)
          }
          self.rewrite_stmts(s.select_data!.cases[i].body)
          i = i + 1
        }
      }
    }
    StDefer => {
      if !isnull(s.defer_data) {
        self.rewrite_stmts(s.defer_data!.body)
      }
    }
    StLock => {
      if !isnull(s.lock_data) {
        self.rewrite_stmts(s.lock_data!.body)
      }
    }
    StSend => {
      if !isnull(s.send_data) {
        if !isnull(s.send_data!.channel) {
          self.rewrite_value(s.send_data!.channel)
        }
        if !isnull(s.send_data!.value) {
          self.rewrite_value(s.send_data!.value)
        }
      }
    }
    _ => {}
  }
}

func MonoPass.rewrite_expr(self, e: LExpr) {
  e.typ = self.subst_type_remove_vars(e.typ)
  match e.kind {
    ExCall => {
      if !isnull(e.call) {
        if len(e.call!.type_args) > 0 {
          if !has_type_vars(e.call!.type_args) {
            let mangled = mangle_name(e.call!.func_name, e.call!.type_args)
            e.call!.func_name = mangled
            e.call!.type_args = []
          }
        }
        let mut i = 0
        while i < len(e.call!.args) {
          if !isnull(e.call!.args[i]) {
            self.rewrite_value(e.call!.args[i])
          }
          i = i + 1
        }
      }
    }
    ExMethodCall => {
      if !isnull(e.method_call) {
        let mc = e.method_call!
        if !isnull(mc.receiver) {
          self.rewrite_value(mc.receiver)
          if !isnull(mc.receiver!.typ) {
            if mc.receiver!.typ!.kind is TyClassHandle {
              let has_inst = self.class_instances!.get(sym(inst_key(mc.receiver!.typ!.name, "")))
              // Check if this is a generic class with instances
              let cls_entry = self.class_by_name!.get(sym(mc.receiver!.typ!.name))
              let is_generic_class = (
                !isnull(cls_entry) &&
                len(cls_entry!.value.type_params) > 0
              )
              if is_generic_class {
                if len(mc.type_args) > 0 {
                  if !has_type_vars(mc.type_args) {
                    let mangled_class = mangle_name(mc.receiver!.typ!.name, mc.type_args)
                    mc.receiver!.typ = LType {
                      kind: TyClassHandle,
                      name: mangled_class,
                      elem: null,
                      key: null,
                      fields: [],
                      params: [],
                      ret: null,
                      variants: [],
                      type_args: [],
                      bits: 0,
                      is_exported: false,
                    }
                    mc.type_args = []
                  }
                }
              } else {
                // Interface method on concrete class — just clear type args
                if len(mc.type_args) > 0 {
                  mc.type_args = []
                }
              }
            }
          }
        }
        let mut i = 0
        while i < len(mc.args) {
          if !isnull(mc.args[i]) {
            self.rewrite_value(mc.args[i])
          }
          i = i + 1
        }
      }
    }
    ExClassAlloc => {
      if !isnull(e.class_alloc) {
        if len(e.class_alloc!.type_args) > 0 {
          if !has_type_vars(e.class_alloc!.type_args) {
            let mangled = mangle_name(e.class_alloc!.class_name, e.class_alloc!.type_args)
            e.class_alloc!.class_name = mangled
            e.typ = LType {
              kind: TyClassHandle,
              name: mangled,
              elem: null,
              key: null,
              fields: [],
              params: [],
              ret: null,
              variants: [],
              type_args: [],
              bits: 0,
              is_exported: false,
            }
            e.class_alloc!.type_args = []
          }
        }
        let mut i = 0
        while i < len(e.class_alloc!.fields) {
          if !isnull(e.class_alloc!.fields[i].value) {
            self.rewrite_value(e.class_alloc!.fields[i].value)
          }
          i = i + 1
        }
      }
    }
    ExBinOp => {
      if !isnull(e.bin_op) {
        if !isnull(e.bin_op!.left) {
          e.bin_op!.left!.typ = self.subst_type_remove_vars(e.bin_op!.left!.typ)
        }
        if !isnull(e.bin_op!.right) {
          e.bin_op!.right!.typ = self.subst_type_remove_vars(e.bin_op!.right!.typ)
        }
      }
    }
    ExUnOp => {
      if !isnull(e.un_op) {
        if !isnull(e.un_op!.operand) {
          self.rewrite_value(e.un_op!.operand)
        }
      }
    }
    ExCast => {
      if !isnull(e.cast) {
        e.cast!.target = self.subst_type_remove_vars(e.cast!.target)
        if !isnull(e.cast!.operand) {
          self.rewrite_value(e.cast!.operand)
        }
      }
    }
    ExBuiltin => {
      if !isnull(e.builtin) {
        let mut i = 0
        while i < len(e.builtin!.args) {
          if !isnull(e.builtin!.args[i]) {
            self.rewrite_value(e.builtin!.args[i])
          }
          i = i + 1
        }
      }
    }
    ExFuncLit => {
      if !isnull(e.func_lit) {
        e.func_lit!.return_type = self.subst_type_remove_vars(e.func_lit!.return_type)
        let mut i = 0
        while i < len(e.func_lit!.params) {
          e.func_lit!.params[i].typ = self.subst_type_remove_vars(e.func_lit!.params[i].typ)
          i = i + 1
        }
        self.rewrite_stmts(e.func_lit!.body)
      }
    }
    ExWrapOptional => {
      if !isnull(e.wrap_opt) {
        if !isnull(e.wrap_opt!.value) {
          self.rewrite_value(e.wrap_opt!.value)
        }
      }
    }
    ExUnwrapOptional => {
      if !isnull(e.unwrap_opt) {
        if !isnull(e.unwrap_opt!.value) {
          self.rewrite_value(e.unwrap_opt!.value)
        }
      }
    }
    ExIsNull => {
      if !isnull(e.is_null) {
        if !isnull(e.is_null!.value) {
          self.rewrite_value(e.is_null!.value)
        }
      }
    }
    ExVariantConstruct => {
      if !isnull(e.variant_construct) {
        let mut i = 0
        while i < len(e.variant_construct!.fields) {
          if !isnull(e.variant_construct!.fields[i]) {
            self.rewrite_value(e.variant_construct!.fields[i])
          }
          i = i + 1
        }
      }
    }
    ExVariantData => {
      if !isnull(e.variant_data) {
        if !isnull(e.variant_data!.value) {
          self.rewrite_value(e.variant_data!.value)
        }
      }
    }
    ExVariantTag => {
      if !isnull(e.variant_tag) {
        if !isnull(e.variant_tag!.value) {
          self.rewrite_value(e.variant_tag!.value)
        }
      }
    }
    ExSlice => {
      if !isnull(e.slice_data) {
        if !isnull(e.slice_data!.collection) {
          self.rewrite_value(e.slice_data!.collection)
        }
      }
    }
    ExStructLit => {
      if !isnull(e.struct_lit) {
        let mut i = 0
        while i < len(e.struct_lit!.fields) {
          if !isnull(e.struct_lit!.fields[i].value) {
            self.rewrite_value(e.struct_lit!.fields[i].value)
          }
          i = i + 1
        }
      }
    }
    ExStructField => {
      if !isnull(e.struct_field) {
        if !isnull(e.struct_field!.receiver) {
          self.rewrite_value(e.struct_field!.receiver)
        }
      }
    }
    ExClassGet => {
      if !isnull(e.class_get) {
        // Copy-out/modify/write-back: class_get is optional struct on class LExpr
        let mut cg = e.class_get!
        if !isnull(cg.handle) {
          self.rewrite_value(cg.handle)
          // Invariant: class_name must equal handle.typ.name after monomorphization.
          if !isnull(cg.handle!.typ) && cg.handle!.typ!.kind is TyClassHandle {
            cg.class_name = cg.handle!.typ!.name
          }
        }
        e.class_get = cg
      }
    }
    ExIndexGet => {
      if !isnull(e.index_get) {
        if !isnull(e.index_get!.collection) {
          self.rewrite_value(e.index_get!.collection)
        }
        if !isnull(e.index_get!.index) {
          self.rewrite_value(e.index_get!.index)
        }
      }
    }
    ExExtractValue => {
      if !isnull(e.extract_value) {
        if !isnull(e.extract_value!.value) {
          self.rewrite_value(e.extract_value!.value)
        }
      }
    }
    ExExtractError => {
      if !isnull(e.extract_error) {
        if !isnull(e.extract_error!.value) {
          self.rewrite_value(e.extract_error!.value)
        }
      }
    }
    ExMakeResult => {
      if !isnull(e.make_result) {
        if !isnull(e.make_result!.value) {
          self.rewrite_value(e.make_result!.value)
        }
        if !isnull(e.make_result!.err) {
          e.make_result!.err!.typ = self.subst_type_remove_vars(e.make_result!.err!.typ)
        }
      }
    }
    ExFormat => {
      if !isnull(e.format) {
        let mut i = 0
        while i < len(e.format!.parts) {
          if !e.format!.parts[i].is_literal {
            if !isnull(e.format!.parts[i].value) {
              self.rewrite_value(e.format!.parts[i].value)
            }
          }
          i = i + 1
        }
      }
    }
    ExEnvGet => {
      if !isnull(e.env_get) {
        if !isnull(e.env_get!.env) {
          e.env_get!.env!.typ = self.subst_type_remove_vars(e.env_get!.env!.typ)
        }
      }
    }
    ExFuncRef => {
      if !isnull(e.func_ref) {
        if !isnull(e.func_ref!.env) {
          e.func_ref!.env!.typ = self.subst_type_remove_vars(e.func_ref!.env!.typ)
        }
      }
    }
    ExMakeChannel => {
      if !isnull(e.make_channel) {
        e.make_channel!.elem_type = self.subst_type_remove_vars(e.make_channel!.elem_type)
      }
    }
    _ => {}
  }
}

// ---------------------------------------------------------------------------
// Helpers: type substitution
// ---------------------------------------------------------------------------

func subst_type(t: LType?, subst: Dict<Sym, LType?>?) -> LType? {
  if isnull(t) {
    return null
  }
  if isnull(subst) {
    return t
  }
  if t!.kind is TyTypeVar {
    let concrete = subst!.get(sym(t!.name))
    if !isnull(concrete) {
      return concrete!.value
    }
    return t
  }
  // Also substitute class/struct handles that are type param names
  if t!.kind is TyClassHandle || t!.kind is TyStruct {
    let concrete = subst!.get(sym(t!.name))
    if !isnull(concrete) {
      return concrete!.value
    }
  }
  // Deep copy with substitution
  let mut new_fields: [LField] = []
  let mut i = 0
  while i < len(t!.fields) {
    append(new_fields, LField { name: t!.fields[i].name, typ: subst_type(t!.fields[i].typ, subst) })
    i = i + 1
  }
  let mut new_params: [LType?] = []
  i = 0
  while i < len(t!.params) {
    append(new_params, subst_type(t!.params[i], subst))
    i = i + 1
  }
  let mut new_variants: [LVariant] = []
  i = 0
  while i < len(t!.variants) {
    let mut vfields: [LField] = []
    let mut j = 0
    while j < len(t!.variants[i].fields) {
      append(vfields, LField { name: t!.variants[i].fields[j].name, typ: subst_type(t!.variants[i].fields[j].typ, subst) })
      j = j + 1
    }
    append(new_variants, LVariant { name: t!.variants[i].name, tag: t!.variants[i].tag, fields: vfields })
    i = i + 1
  }
  let mut new_type_args: [LType?] = []
  i = 0
  while i < len(t!.type_args) {
    append(new_type_args, subst_type(t!.type_args[i], subst))
    i = i + 1
  }
  return LType {
    kind: t!.kind,
    name: t!.name,
    elem: subst_type(t!.elem, subst),
    key: subst_type(t!.key, subst),
    fields: new_fields,
    params: new_params,
    ret: subst_type(t!.ret, subst),
    variants: new_variants,
    type_args: new_type_args,
    bits: t!.bits,
    is_exported: t!.is_exported,
  }
}

func MonoPass.subst_type_remove_vars(self, t: LType?) -> LType? {
  if isnull(t) {
    return null
  }
  // Rewrite generic class/struct handles with TypeArgs into mangled names
  let is_class_or_struct = (
    t!.kind is TyClassHandle ||
    t!.kind is TyStruct
  )
  if is_class_or_struct {
    if len(t!.type_args) > 0 {
      if !has_type_vars(t!.type_args) {
        let mangled = mangle_name(t!.name, t!.type_args)
        return LType {
          kind: t!.kind,
          name: mangled,
          elem: t!.elem,
          key: t!.key,
          fields: t!.fields,
          params: [],
          ret: t!.ret,
          variants: t!.variants,
          type_args: [],
          bits: t!.bits,
          is_exported: t!.is_exported,
        }
      }
    }
  }
  // Recurse into container types
  let new_elem = self.subst_type_remove_vars(t!.elem)
  let new_key = self.subst_type_remove_vars(t!.key)
  let new_ret = self.subst_type_remove_vars(t!.ret)
  let mut new_params: [LType?] = []
  let mut i = 0
  while i < len(t!.params) {
    append(new_params, self.subst_type_remove_vars(t!.params[i]))
    i = i + 1
  }
  let mut new_type_args: [LType?] = []
  i = 0
  while i < len(t!.type_args) {
    append(new_type_args, self.subst_type_remove_vars(t!.type_args[i]))
    i = i + 1
  }
  // Always rebuild to be safe (unlike Go which checks pointer equality)
  return LType {
    kind: t!.kind,
    name: t!.name,
    elem: new_elem,
    key: new_key,
    fields: t!.fields,
    params: new_params,
    ret: new_ret,
    variants: t!.variants,
    type_args: new_type_args,
    bits: t!.bits,
    is_exported: t!.is_exported,
  }
}

// ---------------------------------------------------------------------------
// Helpers: cloning statements with type substitution
// ---------------------------------------------------------------------------

func clone_stmts(stmts: [LStmt?], subst: Dict<Sym, LType?>?) -> [LStmt?] {
  let mut out: [LStmt?] = []
  let mut i = 0
  while i < len(stmts) {
    if isnull(stmts[i]) {
      append(out, null)
    } else {
      append(out, clone_stmt(stmts[i]!, subst))
    }
    i = i + 1
  }
  return out
}

func clone_stmt(s: LStmt, subst: Dict<Sym, LType?>?) -> LStmt? {
  match s.kind {
    StTempDef => {
      if !isnull(s.temp_def) {
        let cloned_expr = clone_expr(s.temp_def!.expr, subst)
        return LStmt {
          kind: StTempDef,
          temp_def: LTempDef { id: s.temp_def!.id, expr: cloned_expr },
          var_decl: null, assign: null, struct_set: null, class_set: null,
          index_set: null, if_data: null, while_data: null, for_data: null,
          switch_data: null, block: null, ret: null, expr_stmt: null,
          defer_data: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StVarDecl => {
      if !isnull(s.var_decl) {
        let mut init_val: LValue? = null
        if !isnull(s.var_decl!.init) {
          init_val = clone_value(s.var_decl!.init!, subst)
        }
        return LStmt {
          kind: StVarDecl,
          var_decl: LVarDeclData {
            name: s.var_decl!.name,
            typ: subst_type(s.var_decl!.typ, subst),
            init: init_val,
            mutable: s.var_decl!.mutable,
          },
          temp_def: null, assign: null, struct_set: null, class_set: null,
          index_set: null, if_data: null, while_data: null, for_data: null,
          switch_data: null, block: null, ret: null, expr_stmt: null,
          defer_data: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StAssign => {
      if !isnull(s.assign) {
        let mut val: LValue? = null
        if !isnull(s.assign!.value) {
          val = clone_value(s.assign!.value!, subst)
        }
        return LStmt {
          kind: StAssign,
          assign: LAssignData { target: s.assign!.target, value: val },
          temp_def: null, var_decl: null, struct_set: null, class_set: null,
          index_set: null, if_data: null, while_data: null, for_data: null,
          switch_data: null, block: null, ret: null, expr_stmt: null,
          defer_data: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StReturn => {
      if !isnull(s.ret) {
        let mut vals: [LValue?] = []
        let mut i = 0
        while i < len(s.ret!.values) {
          if !isnull(s.ret!.values[i]) {
            append(vals, clone_value(s.ret!.values[i]!, subst))
          } else {
            append(vals, null)
          }
          i = i + 1
        }
        return LStmt {
          kind: StReturn,
          ret: LReturnData { values: vals },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, while_data: null,
          for_data: null, switch_data: null, block: null, expr_stmt: null,
          defer_data: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StIf => {
      if !isnull(s.if_data) {
        let mut cond: LValue? = null
        if !isnull(s.if_data!.cond) {
          cond = clone_value(s.if_data!.cond!, subst)
        }
        return LStmt {
          kind: StIf,
          if_data: LIfData {
            cond: cond,
            then_body: clone_stmts(s.if_data!.then_body, subst),
            else_body: clone_stmts(s.if_data!.else_body, subst),
          },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, while_data: null, for_data: null,
          switch_data: null, block: null, ret: null, expr_stmt: null,
          defer_data: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StWhile => {
      if !isnull(s.while_data) {
        let mut cv: LValue? = null
        if !isnull(s.while_data!.cond_var) {
          cv = clone_value(s.while_data!.cond_var!, subst)
        }
        return LStmt {
          kind: StWhile,
          while_data: LWhileData {
            cond_block: clone_stmts(s.while_data!.cond_block, subst),
            cond_var: cv,
            body: clone_stmts(s.while_data!.body, subst),
          },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, for_data: null,
          switch_data: null, block: null, ret: null, expr_stmt: null,
          defer_data: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StFor => {
      if !isnull(s.for_data) {
        let mut col: LValue? = null
        if !isnull(s.for_data!.collection) {
          col = clone_value(s.for_data!.collection!, subst)
        }
        return LStmt {
          kind: StFor,
          for_data: LForData {
            var_name: s.for_data!.var_name,
            var_type: subst_type(s.for_data!.var_type, subst),
            index_var: s.for_data!.index_var,
            collection: col,
            body: clone_stmts(s.for_data!.body, subst),
          },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, while_data: null,
          switch_data: null, block: null, ret: null, expr_stmt: null,
          defer_data: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StSwitch => {
      if !isnull(s.switch_data) {
        let mut cases: [LSwitchCase] = []
        let mut i = 0
        while i < len(s.switch_data!.cases) {
          let c = s.switch_data!.cases[i]
          append(cases, LSwitchCase {
            tag: c.tag,
            binding: c.binding,
            body: clone_stmts(c.body, subst),
          })
          i = i + 1
        }
        let mut tag: LValue? = null
        if !isnull(s.switch_data!.tag) {
          tag = clone_value(s.switch_data!.tag!, subst)
        }
        return LStmt {
          kind: StSwitch,
          switch_data: LSwitchData {
            tag: tag,
            cases: cases,
            enum_name: s.switch_data!.enum_name,
          },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, while_data: null,
          for_data: null, block: null, ret: null, expr_stmt: null,
          defer_data: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StBlock => {
      if !isnull(s.block) {
        return LStmt {
          kind: StBlock,
          block: LBlockData { stmts: clone_stmts(s.block!.stmts, subst) },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, while_data: null,
          for_data: null, switch_data: null, ret: null, expr_stmt: null,
          defer_data: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StSideEffect => {
      if !isnull(s.side_effect) {
        return LStmt {
          kind: StSideEffect,
          side_effect: LSideEffectData { expr: clone_expr(s.side_effect!.expr, subst) },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, while_data: null,
          for_data: null, switch_data: null, block: null, ret: null,
          expr_stmt: null, defer_data: null, spawn_data: null, lock_data: null,
          send_data: null, select_data: null, yield_data: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StExpr => {
      if !isnull(s.expr_stmt) {
        return LStmt {
          kind: StExpr,
          expr_stmt: LExprStmtData { temp_id: s.expr_stmt!.temp_id },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, while_data: null,
          for_data: null, switch_data: null, block: null, ret: null,
          defer_data: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StStructSet => {
      if !isnull(s.struct_set) {
        let mut r: LValue? = null
        if !isnull(s.struct_set!.receiver) {
          r = clone_value(s.struct_set!.receiver!, subst)
        }
        let mut v: LValue? = null
        if !isnull(s.struct_set!.value) {
          v = clone_value(s.struct_set!.value!, subst)
        }
        return LStmt {
          kind: StStructSet,
          struct_set: LStructSetData { receiver: r, field: s.struct_set!.field, value: v },
          temp_def: null, var_decl: null, assign: null, class_set: null,
          index_set: null, if_data: null, while_data: null, for_data: null,
          switch_data: null, block: null, ret: null, expr_stmt: null,
          defer_data: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StClassSet => {
      if !isnull(s.class_set) {
        let mut h: LValue? = null
        if !isnull(s.class_set!.handle) {
          h = clone_value(s.class_set!.handle!, subst)
        }
        let mut v: LValue? = null
        if !isnull(s.class_set!.value) {
          v = clone_value(s.class_set!.value!, subst)
        }
        // Derive mangled class_name from the cloned handle's type.
        let mut cname = s.class_set!.class_name
        if !isnull(h) && !isnull(h!.typ) && h!.typ!.kind is TyClassHandle {
          let ht = h!.typ!
          if len(ht.type_args) > 0 && !has_type_vars(ht.type_args) {
            cname = mangle_name(ht.name, ht.type_args)
          } else {
            cname = ht.name
          }
        }
        return LStmt {
          kind: StClassSet,
          class_set: LClassSetData { handle: h, class_name: cname, field: s.class_set!.field, value: v },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          index_set: null, if_data: null, while_data: null, for_data: null,
          switch_data: null, block: null, ret: null, expr_stmt: null,
          defer_data: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StIndexSet => {
      if !isnull(s.index_set) {
        let mut c: LValue? = null
        if !isnull(s.index_set!.collection) {
          c = clone_value(s.index_set!.collection!, subst)
        }
        let mut idx: LValue? = null
        if !isnull(s.index_set!.index) {
          idx = clone_value(s.index_set!.index!, subst)
        }
        let mut v: LValue? = null
        if !isnull(s.index_set!.value) {
          v = clone_value(s.index_set!.value!, subst)
        }
        return LStmt {
          kind: StIndexSet,
          index_set: LIndexSetData { collection: c, index: idx, value: v, field: s.index_set!.field },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, if_data: null, while_data: null, for_data: null,
          switch_data: null, block: null, ret: null, expr_stmt: null,
          defer_data: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StMultiAssign => {
      if !isnull(s.multi_assign) {
        let mut types: [LType?] = []
        let mut i = 0
        while i < len(s.multi_assign!.types) {
          append(types, subst_type(s.multi_assign!.types[i], subst))
          i = i + 1
        }
        return LStmt {
          kind: StMultiAssign,
          multi_assign: LMultiAssignData {
            names: s.multi_assign!.names,
            types: types,
            expr: clone_expr(s.multi_assign!.expr, subst),
          },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, while_data: null,
          for_data: null, switch_data: null, block: null, ret: null,
          expr_stmt: null, defer_data: null, spawn_data: null, lock_data: null,
          send_data: null, select_data: null, yield_data: null,
          side_effect: null, type_switch: null,
        }
      }
    }
    StSpawn => {
      if !isnull(s.spawn_data) {
        return LStmt {
          kind: StSpawn,
          spawn_data: LSpawnData {
            body: clone_stmts(s.spawn_data!.body, subst),
            captures: s.spawn_data!.captures,
          },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, while_data: null,
          for_data: null, switch_data: null, block: null, ret: null,
          expr_stmt: null, defer_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StDefer => {
      if !isnull(s.defer_data) {
        return LStmt {
          kind: StDefer,
          defer_data: LDeferData { body: clone_stmts(s.defer_data!.body, subst) },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, while_data: null,
          for_data: null, switch_data: null, block: null, ret: null,
          expr_stmt: null, spawn_data: null, lock_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StLock => {
      if !isnull(s.lock_data) {
        let mut mx: LValue? = null
        if !isnull(s.lock_data!.mutex) {
          mx = clone_value(s.lock_data!.mutex!, subst)
        }
        return LStmt {
          kind: StLock,
          lock_data: LLockData {
            mutex: mx,
            body: clone_stmts(s.lock_data!.body, subst),
          },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, while_data: null,
          for_data: null, switch_data: null, block: null, ret: null,
          expr_stmt: null, spawn_data: null, defer_data: null, send_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StSend => {
      if !isnull(s.send_data) {
        let mut ch: LValue? = null
        if !isnull(s.send_data!.channel) {
          ch = clone_value(s.send_data!.channel!, subst)
        }
        let mut v: LValue? = null
        if !isnull(s.send_data!.value) {
          v = clone_value(s.send_data!.value!, subst)
        }
        return LStmt {
          kind: StSend,
          send_data: LSendData { channel: ch, value: v },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, while_data: null,
          for_data: null, switch_data: null, block: null, ret: null,
          expr_stmt: null, spawn_data: null, lock_data: null, defer_data: null,
          select_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StSelect => {
      if !isnull(s.select_data) {
        let mut cases: [LSelectCase] = []
        let mut i = 0
        while i < len(s.select_data!.cases) {
          let c = s.select_data!.cases[i]
          let mut ch: LValue? = null
          if !isnull(c.channel) {
            ch = clone_value(c.channel!, subst)
          }
          let mut v: LValue? = null
          if !isnull(c.value) {
            v = clone_value(c.value!, subst)
          }
          append(cases, LSelectCase {
            kind: c.kind,
            channel: ch,
            value: v,
            binding: c.binding,
            body: clone_stmts(c.body, subst),
          })
          i = i + 1
        }
        return LStmt {
          kind: StSelect,
          select_data: LSelectData { cases: cases },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, while_data: null,
          for_data: null, switch_data: null, block: null, ret: null,
          expr_stmt: null, spawn_data: null, lock_data: null, defer_data: null,
          send_data: null, yield_data: null, side_effect: null,
          multi_assign: null, type_switch: null,
        }
      }
    }
    StTypeSwitch => {
      if !isnull(s.type_switch) {
        let mut cases: [LTypeSwitchCase] = []
        let mut i = 0
        while i < len(s.type_switch!.cases) {
          let c = s.type_switch!.cases[i]
          append(cases, LTypeSwitchCase {
            typ: subst_type(c.typ, subst),
            binding: c.binding,
            body: clone_stmts(c.body, subst),
          })
          i = i + 1
        }
        let mut val: LValue? = null
        if !isnull(s.type_switch!.value) {
          val = clone_value(s.type_switch!.value!, subst)
        }
        return LStmt {
          kind: StTypeSwitch,
          type_switch: LTypeSwitchData { value: val, cases: cases },
          temp_def: null, var_decl: null, assign: null, struct_set: null,
          class_set: null, index_set: null, if_data: null, while_data: null,
          for_data: null, switch_data: null, block: null, ret: null,
          expr_stmt: null, spawn_data: null, lock_data: null, defer_data: null,
          send_data: null, yield_data: null, side_effect: null,
          multi_assign: null,
        }
      }
    }
    _ => {}
  }
  // Fallback: return a copy of the original stmt
  return s
}

func clone_value(v: LValue, subst: Dict<Sym, LType?>?) -> LValue? {
  return LValue {
    kind: v.kind,
    name: v.name,
    temp_id: v.temp_id,
    int_val: v.int_val,
    uint_val: v.uint_val,
    float_val: v.float_val,
    str_val: v.str_val,
    bool_val: v.bool_val,
    typ: subst_type(v.typ, subst),
  }
}

func clone_expr(e: LExpr?, subst: Dict<Sym, LType?>?) -> LExpr? {
  if isnull(e) {
    return null
  }
  let typ = subst_type(e!.typ, subst)
  match e!.kind {
    ExCall => {
      if !isnull(e!.call) {
        let mut args: [LValue?] = []
        let mut i = 0
        while i < len(e!.call!.args) {
          if !isnull(e!.call!.args[i]) {
            append(args, clone_value(e!.call!.args[i]!, subst))
          } else {
            append(args, null)
          }
          i = i + 1
        }
        let mut ta: [LType?] = []
        i = 0
        while i < len(e!.call!.type_args) {
          append(ta, subst_type(e!.call!.type_args[i], subst))
          i = i + 1
        }
        return LExpr {
          kind: ExCall,
          typ: typ,
          call: LCallData { func_name: e!.call!.func_name, args: args, mut_args: e!.call!.mut_args, type_args: ta, is_exported: e!.call!.is_exported },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          env_get: null, func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExMethodCall => {
      if !isnull(e!.method_call) {
        let mc = e!.method_call!
        let mut recv: LValue? = null
        if !isnull(mc.receiver) {
          recv = clone_value(mc.receiver!, subst)
        }
        let mut args: [LValue?] = []
        let mut i = 0
        while i < len(mc.args) {
          if !isnull(mc.args[i]) {
            append(args, clone_value(mc.args[i]!, subst))
          } else {
            append(args, null)
          }
          i = i + 1
        }
        let mut ta: [LType?] = []
        i = 0
        while i < len(mc.type_args) {
          append(ta, subst_type(mc.type_args[i], subst))
          i = i + 1
        }
        return LExpr {
          kind: ExMethodCall,
          typ: typ,
          method_call: LMethodCallData {
            receiver: recv, method: mc.method, args: args, mut_args: mc.mut_args, type_args: ta,
            is_exported: mc.is_exported, param_types: mc.param_types,
          },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          env_get: null, func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExClassAlloc => {
      if !isnull(e!.class_alloc) {
        let mut fields: [LFieldInit] = []
        let mut i = 0
        while i < len(e!.class_alloc!.fields) {
          let f = e!.class_alloc!.fields[i]
          let mut v: LValue? = null
          if !isnull(f.value) {
            v = clone_value(f.value!, subst)
          }
          append(fields, LFieldInit { name: f.name, value: v })
          i = i + 1
        }
        let mut ta: [LType?] = []
        i = 0
        while i < len(e!.class_alloc!.type_args) {
          append(ta, subst_type(e!.class_alloc!.type_args[i], subst))
          i = i + 1
        }
        return LExpr {
          kind: ExClassAlloc,
          typ: typ,
          class_alloc: LClassAllocData { class_name: e!.class_alloc!.class_name, fields: fields, type_args: ta },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, make_channel: null, wrap_opt: null, unwrap_opt: null,
          is_null: null, variant_construct: null, variant_tag: null, variant_data: null,
          extract_value: null, extract_error: null, make_result: null, env_get: null,
          func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExBinOp => {
      if !isnull(e!.bin_op) {
        let mut l: LValue? = null
        if !isnull(e!.bin_op!.left) {
          l = clone_value(e!.bin_op!.left!, subst)
        }
        let mut r: LValue? = null
        if !isnull(e!.bin_op!.right) {
          r = clone_value(e!.bin_op!.right!, subst)
        }
        return LExpr {
          kind: ExBinOp,
          typ: typ,
          bin_op: LBinOpData { op: e!.bin_op!.op, left: l, right: r },
          un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          env_get: null, func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExUnOp => {
      if !isnull(e!.un_op) {
        let mut op: LValue? = null
        if !isnull(e!.un_op!.operand) {
          op = clone_value(e!.un_op!.operand!, subst)
        }
        return LExpr {
          kind: ExUnOp,
          typ: typ,
          un_op: LUnOpData { op: e!.un_op!.op, operand: op },
          bin_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          env_get: null, func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExCast => {
      if !isnull(e!.cast) {
        let mut op: LValue? = null
        if !isnull(e!.cast!.operand) {
          op = clone_value(e!.cast!.operand!, subst)
        }
        return LExpr {
          kind: ExCast,
          typ: typ,
          cast: LCastData { target: subst_type(e!.cast!.target, subst), operand: op },
          bin_op: null, un_op: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          env_get: null, func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExBuiltin => {
      if !isnull(e!.builtin) {
        let mut args: [LValue?] = []
        let mut i = 0
        while i < len(e!.builtin!.args) {
          if !isnull(e!.builtin!.args[i]) {
            append(args, clone_value(e!.builtin!.args[i]!, subst))
          } else {
            append(args, null)
          }
          i = i + 1
        }
        return LExpr {
          kind: ExBuiltin,
          typ: typ,
          builtin: LBuiltinData { name: e!.builtin!.name, args: args },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          env_get: null, func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExStructLit => {
      if !isnull(e!.struct_lit) {
        let mut fields: [LFieldInit] = []
        let mut i = 0
        while i < len(e!.struct_lit!.fields) {
          let f = e!.struct_lit!.fields[i]
          let mut v: LValue? = null
          if !isnull(f.value) {
            v = clone_value(f.value!, subst)
          }
          append(fields, LFieldInit { name: f.name, value: v })
          i = i + 1
        }
        return LExpr {
          kind: ExStructLit,
          typ: typ,
          struct_lit: LStructLitData { fields: fields },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          class_alloc: null, make_channel: null, wrap_opt: null, unwrap_opt: null,
          is_null: null, variant_construct: null, variant_tag: null, variant_data: null,
          extract_value: null, extract_error: null, make_result: null, env_get: null,
          func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExStructField => {
      if !isnull(e!.struct_field) {
        let mut r: LValue? = null
        if !isnull(e!.struct_field!.receiver) {
          r = clone_value(e!.struct_field!.receiver!, subst)
        }
        return LExpr {
          kind: ExStructField,
          typ: typ,
          struct_field: LStructFieldData { receiver: r, field: e!.struct_field!.field },
          bin_op: null, un_op: null, cast: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          env_get: null, func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExClassGet => {
      if !isnull(e!.class_get) {
        let mut h: LValue? = null
        if !isnull(e!.class_get!.handle) {
          h = clone_value(e!.class_get!.handle!, subst)
        }
        // Derive mangled class_name from the cloned handle's type.
        // clone_value uses subst_type (substitutes vars) but doesn't mangle — do it here.
        let mut cname = e!.class_get!.class_name
        if !isnull(h) && !isnull(h!.typ) && h!.typ!.kind is TyClassHandle {
          let ht = h!.typ!
          if len(ht.type_args) > 0 && !has_type_vars(ht.type_args) {
            cname = mangle_name(ht.name, ht.type_args)
          } else {
            cname = ht.name
          }
        }
        return LExpr {
          kind: ExClassGet,
          typ: typ,
          class_get: LClassGetData { handle: h, class_name: cname, field: e!.class_get!.field },
          bin_op: null, un_op: null, cast: null, struct_field: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          env_get: null, func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExIndexGet => {
      if !isnull(e!.index_get) {
        let mut c: LValue? = null
        if !isnull(e!.index_get!.collection) {
          c = clone_value(e!.index_get!.collection!, subst)
        }
        let mut idx: LValue? = null
        if !isnull(e!.index_get!.index) {
          idx = clone_value(e!.index_get!.index!, subst)
        }
        return LExpr {
          kind: ExIndexGet,
          typ: typ,
          index_get: LIndexGetData { collection: c, index: idx },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          env_get: null, func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExSlice => {
      if !isnull(e!.slice_data) {
        let mut c: LValue? = null
        if !isnull(e!.slice_data!.collection) {
          c = clone_value(e!.slice_data!.collection!, subst)
        }
        let mut lo: LValue? = null
        if !isnull(e!.slice_data!.low) {
          lo = clone_value(e!.slice_data!.low!, subst)
        }
        let mut hi: LValue? = null
        if !isnull(e!.slice_data!.high) {
          hi = clone_value(e!.slice_data!.high!, subst)
        }
        return LExpr {
          kind: ExSlice,
          typ: typ,
          slice_data: LSliceData { collection: c, low: lo, high: hi },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          env_get: null, func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExWrapOptional => {
      if !isnull(e!.wrap_opt) {
        let mut v: LValue? = null
        if !isnull(e!.wrap_opt!.value) {
          v = clone_value(e!.wrap_opt!.value!, subst)
        }
        return LExpr {
          kind: ExWrapOptional,
          typ: typ,
          wrap_opt: LWrapOptionalData { value: v },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, unwrap_opt: null,
          is_null: null, variant_construct: null, variant_tag: null, variant_data: null,
          extract_value: null, extract_error: null, make_result: null, env_get: null,
          func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExUnwrapOptional => {
      if !isnull(e!.unwrap_opt) {
        let mut v: LValue? = null
        if !isnull(e!.unwrap_opt!.value) {
          v = clone_value(e!.unwrap_opt!.value!, subst)
        }
        return LExpr {
          kind: ExUnwrapOptional,
          typ: typ,
          unwrap_opt: LUnwrapOptionalData { value: v },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          is_null: null, variant_construct: null, variant_tag: null, variant_data: null,
          extract_value: null, extract_error: null, make_result: null, env_get: null,
          func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExIsNull => {
      if !isnull(e!.is_null) {
        let mut v: LValue? = null
        if !isnull(e!.is_null!.value) {
          v = clone_value(e!.is_null!.value!, subst)
        }
        return LExpr {
          kind: ExIsNull,
          typ: typ,
          is_null: LIsNullData { value: v },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, variant_construct: null, variant_tag: null, variant_data: null,
          extract_value: null, extract_error: null, make_result: null, env_get: null,
          func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExVariantConstruct => {
      if !isnull(e!.variant_construct) {
        let mut fields: [LValue?] = []
        let mut i = 0
        while i < len(e!.variant_construct!.fields) {
          if !isnull(e!.variant_construct!.fields[i]) {
            append(fields, clone_value(e!.variant_construct!.fields[i]!, subst))
          } else {
            append(fields, null)
          }
          i = i + 1
        }
        return LExpr {
          kind: ExVariantConstruct,
          typ: typ,
          variant_construct: LVariantConstructData {
            enum_name: e!.variant_construct!.enum_name,
            variant: e!.variant_construct!.variant,
            tag: e!.variant_construct!.tag,
            fields: fields,
          },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_tag: null, variant_data: null,
          extract_value: null, extract_error: null, make_result: null, env_get: null,
          func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExVariantTag => {
      if !isnull(e!.variant_tag) {
        let mut v: LValue? = null
        if !isnull(e!.variant_tag!.value) {
          v = clone_value(e!.variant_tag!.value!, subst)
        }
        return LExpr {
          kind: ExVariantTag,
          typ: typ,
          variant_tag: LVariantTagData { value: v },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_data: null,
          extract_value: null, extract_error: null, make_result: null, env_get: null,
          func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExVariantData => {
      if !isnull(e!.variant_data) {
        let mut v: LValue? = null
        if !isnull(e!.variant_data!.value) {
          v = clone_value(e!.variant_data!.value!, subst)
        }
        return LExpr {
          kind: ExVariantData,
          typ: typ,
          variant_data: LVariantDataData {
            value: v,
            enum_name: e!.variant_data!.enum_name,
            variant: e!.variant_data!.variant,
            field: e!.variant_data!.field,
          },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          extract_value: null, extract_error: null, make_result: null, env_get: null,
          func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExExtractValue => {
      if !isnull(e!.extract_value) {
        let mut v: LValue? = null
        if !isnull(e!.extract_value!.value) {
          v = clone_value(e!.extract_value!.value!, subst)
        }
        return LExpr {
          kind: ExExtractValue,
          typ: typ,
          extract_value: LExtractValueData { value: v },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_error: null, make_result: null, env_get: null,
          func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExExtractError => {
      if !isnull(e!.extract_error) {
        let mut v: LValue? = null
        if !isnull(e!.extract_error!.value) {
          v = clone_value(e!.extract_error!.value!, subst)
        }
        return LExpr {
          kind: ExExtractError,
          typ: typ,
          extract_error: LExtractErrorData { value: v },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, make_result: null, env_get: null,
          func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExMakeResult => {
      if !isnull(e!.make_result) {
        let mut v: LValue? = null
        if !isnull(e!.make_result!.value) {
          v = clone_value(e!.make_result!.value!, subst)
        }
        let mut er: LValue? = null
        if !isnull(e!.make_result!.err) {
          er = clone_value(e!.make_result!.err!, subst)
        }
        return LExpr {
          kind: ExMakeResult,
          typ: typ,
          make_result: LMakeResultData { value: v, err: er },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, env_get: null,
          func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExFormat => {
      if !isnull(e!.format) {
        let mut parts: [LFormatPart] = []
        let mut i = 0
        while i < len(e!.format!.parts) {
          let p = e!.format!.parts[i]
          if p.is_literal {
            append(parts, p)
          } else {
            let mut v: LValue? = null
            if !isnull(p.value) {
              v = clone_value(p.value!, subst)
            }
            append(parts, LFormatPart { is_literal: false, text: p.text, value: v, format: p.format })
          }
          i = i + 1
        }
        return LExpr {
          kind: ExFormat,
          typ: typ,
          format: LFormatData { parts: parts },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          env_get: null, func_ref: null, func_lit: null,
        }
      }
    }
    ExFuncLit => {
      if !isnull(e!.func_lit) {
        let mut params: [LParam] = []
        let mut i = 0
        while i < len(e!.func_lit!.params) {
          append(params, LParam { name: e!.func_lit!.params[i].name, typ: subst_type(e!.func_lit!.params[i].typ, subst), mutable: e!.func_lit!.params[i].mutable })
          i = i + 1
        }
        return LExpr {
          kind: ExFuncLit,
          typ: typ,
          func_lit: LFuncLitData {
            params: params,
            return_type: subst_type(e!.func_lit!.return_type, subst),
            body: clone_stmts(e!.func_lit!.body, subst),
          },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          env_get: null, func_ref: null, format: null,
        }
      }
    }
    ExEnvGet => {
      if !isnull(e!.env_get) {
        let mut env: LValue? = null
        if !isnull(e!.env_get!.env) {
          env = clone_value(e!.env_get!.env!, subst)
        }
        return LExpr {
          kind: ExEnvGet,
          typ: typ,
          env_get: LEnvGetData { env: env, field: e!.env_get!.field },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          func_ref: null, func_lit: null, format: null,
        }
      }
    }
    ExFuncRef => {
      if !isnull(e!.func_ref) {
        let mut env: LValue? = null
        if !isnull(e!.func_ref!.env) {
          env = clone_value(e!.func_ref!.env!, subst)
        }
        return LExpr {
          kind: ExFuncRef,
          typ: typ,
          func_ref: LFuncRefData { name: e!.func_ref!.name, env: env },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, make_channel: null, wrap_opt: null,
          unwrap_opt: null, is_null: null, variant_construct: null, variant_tag: null,
          variant_data: null, extract_value: null, extract_error: null, make_result: null,
          env_get: null, func_lit: null, format: null,
        }
      }
    }
    ExMakeChannel => {
      if !isnull(e!.make_channel) {
        let mut buf: LValue? = null
        if !isnull(e!.make_channel!.buf_size) {
          buf = clone_value(e!.make_channel!.buf_size!, subst)
        }
        return LExpr {
          kind: ExMakeChannel,
          typ: typ,
          make_channel: LMakeChannelData { elem_type: subst_type(e!.make_channel!.elem_type, subst), buf_size: buf },
          bin_op: null, un_op: null, cast: null, struct_field: null, class_get: null,
          index_get: null, slice_data: null, call: null, method_call: null, builtin: null,
          struct_lit: null, class_alloc: null, wrap_opt: null, unwrap_opt: null,
          is_null: null, variant_construct: null, variant_tag: null, variant_data: null,
          extract_value: null, extract_error: null, make_result: null, env_get: null,
          func_ref: null, func_lit: null, format: null,
        }
      }
    }
    _ => {
      // Fallback — create NEW expr with substituted type, preserving data
      // MUST NOT mutate e — it may be cloned again for other specializations
      return LExpr {
        kind: e!.kind,
        typ: typ,
        bin_op: e!.bin_op, un_op: e!.un_op, cast: e!.cast,
        struct_field: e!.struct_field, class_get: e!.class_get,
        index_get: e!.index_get, slice_data: e!.slice_data,
        call: e!.call, method_call: e!.method_call, builtin: e!.builtin,
        struct_lit: e!.struct_lit, class_alloc: e!.class_alloc,
        make_channel: e!.make_channel, wrap_opt: e!.wrap_opt,
        unwrap_opt: e!.unwrap_opt, is_null: e!.is_null,
        variant_construct: e!.variant_construct, variant_tag: e!.variant_tag,
        variant_data: e!.variant_data, extract_value: e!.extract_value,
        extract_error: e!.extract_error, make_result: e!.make_result,
        env_get: e!.env_get, func_ref: e!.func_ref, func_lit: e!.func_lit,
        format: e!.format,
      }
    }
  }
  // Fallback — return with substituted type
  return e
}

// ---------------------------------------------------------------------------
// Helpers: name mangling
// ---------------------------------------------------------------------------

func mangle_name(base: string, type_args: [LType?]) -> string {
  let mut result = base
  let mut i = 0
  while i < len(type_args) {
    if i == 0 {
      result = f"{result}_{type_to_mangle(type_args[i])}"
    } else {
      result = f"{result}_{type_to_mangle(type_args[i])}"
    }
    i = i + 1
  }
  return result
}

func type_to_mangle(t: LType?) -> string {
  if isnull(t) {
    return "null"
  }
  match t!.kind {
    TyI8 => { return "i8" }
    TyI16 => { return "i16" }
    TyI32 => { return "i32" }
    TyI64 => { return "i64" }
    TyU8 => { return "u8" }
    TyU16 => { return "u16" }
    TyU32 => { return "u32" }
    TyU64 => { return "u64" }
    TyF32 => { return "f32" }
    TyF64 => { return "f64" }
    TyBool => { return "bool" }
    TyString => { return "string" }
    TyStruct => {
      let mut name = f"S{t!.name}"
      let mut i = 0
      while i < len(t!.type_args) {
        name = f"{name}_{type_to_mangle(t!.type_args[i])}"
        i = i + 1
      }
      return name
    }
    TyClassHandle => {
      let mut name = f"C{t!.name}"
      let mut i = 0
      while i < len(t!.type_args) {
        name = f"{name}_{type_to_mangle(t!.type_args[i])}"
        i = i + 1
      }
      return name
    }
    TySlice => { return f"slice_{type_to_mangle(t!.elem)}" }
    TyMap => { return f"map_{type_to_mangle(t!.key)}_{type_to_mangle(t!.elem)}" }
    TyOptional => { return f"opt_{type_to_mangle(t!.elem)}" }
    TyTaggedUnion => { return f"E{t!.name}" }
    TyErrorResult => { return f"res_{type_to_mangle(t!.elem)}" }
    TyAny => {
      if t!.name != "" {
        return f"I{t!.name}"
      }
      return "any"
    }
    TyTuple => {
      let mut s = "tup"
      let mut i = 0
      while i < len(t!.fields) {
        s = f"{s}_{type_to_mangle(t!.fields[i].typ)}"
        i = i + 1
      }
      return s
    }
    TyFuncPtr => { return "fn" }
    TyPlatformInt => { return "int" }
    TyPlatformUint => { return "uint" }
    _ => { return "unknown" }
  }
}

func type_key(types: [LType?]) -> string {
  let mut result = ""
  let mut i = 0
  while i < len(types) {
    if i > 0 {
      result = f"{result},{type_to_mangle(types[i])}"
    } else {
      result = type_to_mangle(types[i])
    }
    i = i + 1
  }
  return result
}

func has_type_vars(types: [LType?]) -> bool {
  let mut i = 0
  while i < len(types) {
    if type_has_var(types[i]) {
      return true
    }
    i = i + 1
  }
  return false
}

func type_has_var(t: LType?) -> bool {
  if isnull(t) {
    return false
  }
  if t!.kind is TyTypeVar {
    return true
  }
  if type_has_var(t!.elem) {
    return true
  }
  if type_has_var(t!.key) {
    return true
  }
  if type_has_var(t!.ret) {
    return true
  }
  let mut i = 0
  while i < len(t!.params) {
    if type_has_var(t!.params[i]) {
      return true
    }
    i = i + 1
  }
  i = 0
  while i < len(t!.type_args) {
    if type_has_var(t!.type_args[i]) {
      return true
    }
    i = i + 1
  }
  i = 0
  while i < len(t!.fields) {
    if type_has_var(t!.fields[i].typ) {
      return true
    }
    i = i + 1
  }
  return false
}

// ---------------------------------------------------------------------------
// Helpers: filtering out generic declarations
// ---------------------------------------------------------------------------

func filter_funcs(funcs: [LFuncDecl], class_instances: Dict<Sym, string>?, class_by_name: Dict<Sym, LClassDecl>?) -> [LFuncDecl] {
  let mut out: [LFuncDecl] = []
  let mut i = 0
  while i < len(funcs) {
    let f = funcs[i]
    let mut skip = false
    if len(f.type_params) > 0 {
      skip = true
    }
    if f.receiver != "" {
      // Check if receiver is a generic class with instances
      let cls_entry = class_by_name!.get(sym(f.receiver))
      if !isnull(cls_entry) {
        if len(cls_entry!.value.type_params) > 0 {
          skip = true
        }
      }
    }
    if !skip {
      append(out, f)
    }
    i = i + 1
  }
  return out
}

func filter_classes(classes: [LClassDecl], class_instances: Dict<Sym, string>?) -> [LClassDecl] {
  let mut out: [LClassDecl] = []
  let mut i = 0
  while i < len(classes) {
    if len(classes[i].type_params) > 0 {
      i = i + 1
      continue
    }
    append(out, classes[i])
    i = i + 1
  }
  return out
}

func filter_structs(structs: [LStructDecl], struct_instances: Dict<Sym, string>?) -> [LStructDecl] {
  let mut out: [LStructDecl] = []
  let mut i = 0
  while i < len(structs) {
    if len(structs[i].type_params) > 0 {
      i = i + 1
      continue
    }
    append(out, structs[i])
    i = i + 1
  }
  return out
}

// ---------------------------------------------------------------------------
// ResolveClassNames — post-monomorphization pass
// ---------------------------------------------------------------------------

func resolve_class_names(prog: LProgram?) {
  if isnull(prog) {
    return
  }
  let mut i = 0
  while i < len(prog!.functions) {
    let f = prog!.functions[i]
    if isnull(f.class_rename_map) {
      i = i + 1
      continue
    }
    let renames = f.class_rename_map!
    // Rewrite params
    let mut j = 0
    while j < len(f.params) {
      resolve_class_type(f.params[j].typ, renames)
      j = j + 1
    }
    // Rewrite return type
    resolve_class_type(f.return_type, renames)
    // Rewrite body
    resolve_class_stmts(f.body, renames)
    i = i + 1
  }
}

func resolve_class_stmts(stmts: [LStmt?], renames: Dict<Sym, string>) {
  let mut i = 0
  while i < len(stmts) {
    if !isnull(stmts[i]) {
      resolve_class_stmt(stmts[i]!, renames)
    }
    i = i + 1
  }
}

func resolve_class_stmt(s: LStmt, renames: Dict<Sym, string>) {
  match s.kind {
    StTempDef => {
      if !isnull(s.temp_def) {
        let td = s.temp_def!
        if !isnull(td.expr) {
          resolve_class_expr(td.expr!, renames)
        }
      }
    }
    StVarDecl => {
      if !isnull(s.var_decl) {
        let mut vd = s.var_decl!
        resolve_class_type(vd.typ, renames)
        if !isnull(vd.init) {
          resolve_class_value(vd.init!, renames)
        }
        s.var_decl = vd
      }
    }
    StAssign => {
      if !isnull(s.assign) {
        let ad = s.assign!
        if !isnull(ad.value) {
          resolve_class_value(ad.value!, renames)
        }
      }
    }
    StReturn => {
      if !isnull(s.ret) {
        let rd = s.ret!
        let mut j = 0
        while j < len(rd.values) {
          if !isnull(rd.values[j]) {
            resolve_class_value(rd.values[j]!, renames)
          }
          j = j + 1
        }
      }
    }
    StIf => {
      if !isnull(s.if_data) {
        let id = s.if_data!
        if !isnull(id.cond) {
          resolve_class_value(id.cond!, renames)
        }
        resolve_class_stmts(id.then_body, renames)
        resolve_class_stmts(id.else_body, renames)
      }
    }
    StWhile => {
      if !isnull(s.while_data) {
        let wd = s.while_data!
        resolve_class_stmts(wd.cond_block, renames)
        if !isnull(wd.cond_var) {
          resolve_class_value(wd.cond_var!, renames)
        }
        resolve_class_stmts(wd.body, renames)
      }
    }
    StFor => {
      if !isnull(s.for_data) {
        let mut fd = s.for_data!
        resolve_class_type(fd.var_type, renames)
        if !isnull(fd.collection) {
          resolve_class_value(fd.collection!, renames)
        }
        resolve_class_stmts(fd.body, renames)
        s.for_data = fd
      }
    }
    StSwitch => {
      if !isnull(s.switch_data) {
        let sd = s.switch_data!
        if !isnull(sd.tag) {
          resolve_class_value(sd.tag!, renames)
        }
        let mut j = 0
        while j < len(sd.cases) {
          resolve_class_stmts(sd.cases[j].body, renames)
          j = j + 1
        }
      }
    }
    StTypeSwitch => {
      if !isnull(s.type_switch) {
        let ts = s.type_switch!
        if !isnull(ts.value) {
          resolve_class_value(ts.value!, renames)
        }
        let mut j = 0
        while j < len(ts.cases) {
          resolve_class_stmts(ts.cases[j].body, renames)
          j = j + 1
        }
      }
    }
    StClassSet => {
      if !isnull(s.class_set) {
        let mut cs = s.class_set!
        let entry = renames.get(sym(cs.class_name))
        if !isnull(entry) {
          cs.class_name = entry!.value
        }
        if !isnull(cs.handle) {
          resolve_class_value(cs.handle!, renames)
        }
        if !isnull(cs.value) {
          resolve_class_value(cs.value!, renames)
        }
        s.class_set = cs
      }
    }
    StSideEffect => {
      if !isnull(s.side_effect) {
        let se = s.side_effect!
        if !isnull(se.expr) {
          resolve_class_expr(se.expr!, renames)
        }
      }
    }
    StBlock => {
      if !isnull(s.block) {
        resolve_class_stmts(s.block!.stmts, renames)
      }
    }
    StMultiAssign => {
      if !isnull(s.multi_assign) {
        let ma = s.multi_assign!
        let mut j = 0
        while j < len(ma.types) {
          resolve_class_type(ma.types[j], renames)
          j = j + 1
        }
        if !isnull(ma.expr) {
          resolve_class_expr(ma.expr!, renames)
        }
      }
    }
    StDefer => {
      if !isnull(s.defer_data) {
        resolve_class_stmts(s.defer_data!.body, renames)
      }
    }
    StSpawn => {
      if !isnull(s.spawn_data) {
        resolve_class_stmts(s.spawn_data!.body, renames)
      }
    }
    StLock => {
      if !isnull(s.lock_data) {
        let ld = s.lock_data!
        if !isnull(ld.mutex) {
          resolve_class_value(ld.mutex!, renames)
        }
        resolve_class_stmts(ld.body, renames)
      }
    }
    _ => {}
  }
}

func resolve_class_expr(e: LExpr, renames: Dict<Sym, string>) {
  resolve_class_type(e.typ, renames)
  match e.kind {
    ExCall => {
      if !isnull(e.call) {
        let cd = e.call!
        let mut j = 0
        while j < len(cd.args) {
          if !isnull(cd.args[j]) {
            resolve_class_value(cd.args[j]!, renames)
          }
          j = j + 1
        }
      }
    }
    ExMethodCall => {
      if !isnull(e.method_call) {
        let md = e.method_call!
        if !isnull(md.receiver) {
          resolve_class_value(md.receiver!, renames)
        }
        let mut j = 0
        while j < len(md.args) {
          if !isnull(md.args[j]) {
            resolve_class_value(md.args[j]!, renames)
          }
          j = j + 1
        }
      }
    }
    ExClassGet => {
      if !isnull(e.class_get) {
        let mut cg = e.class_get!
        let entry = renames.get(sym(cg.class_name))
        if !isnull(entry) {
          cg.class_name = entry!.value
        }
        if !isnull(cg.handle) {
          resolve_class_value(cg.handle!, renames)
        }
        e.class_get = cg
      }
    }
    ExClassAlloc => {
      if !isnull(e.class_alloc) {
        let ca = e.class_alloc!
        let entry = renames.get(sym(ca.class_name))
        if !isnull(entry) {
          ca.class_name = entry!.value
        }
        let mut j = 0
        while j < len(ca.fields) {
          if !isnull(ca.fields[j].value) {
            resolve_class_value(ca.fields[j].value!, renames)
          }
          j = j + 1
        }
      }
    }
    ExBinOp => {
      if !isnull(e.bin_op) {
        let bo = e.bin_op!
        if !isnull(bo.left) {
          resolve_class_value(bo.left!, renames)
        }
        if !isnull(bo.right) {
          resolve_class_value(bo.right!, renames)
        }
      }
    }
    ExCast => {
      if !isnull(e.cast) {
        let mut cd = e.cast!
        if !isnull(cd.operand) {
          resolve_class_value(cd.operand!, renames)
        }
        resolve_class_type(cd.target, renames)
        e.cast = cd
      }
    }
    ExStructField => {
      if !isnull(e.struct_field) {
        let sf = e.struct_field!
        if !isnull(sf.receiver) {
          resolve_class_value(sf.receiver!, renames)
        }
      }
    }
    ExFormat => {
      if !isnull(e.format) {
        let fd = e.format!
        let mut j = 0
        while j < len(fd.parts) {
          if !fd.parts[j].is_literal {
            if !isnull(fd.parts[j].value) {
              resolve_class_value(fd.parts[j].value!, renames)
            }
          }
          j = j + 1
        }
      }
    }
    ExFuncLit => {
      if !isnull(e.func_lit) {
        let fl = e.func_lit!
        let mut j = 0
        while j < len(fl.params) {
          resolve_class_type(fl.params[j].typ, renames)
          j = j + 1
        }
        resolve_class_type(fl.return_type, renames)
        resolve_class_stmts(fl.body, renames)
      }
    }
    ExWrapOptional => {
      if !isnull(e.wrap_opt) {
        let wo = e.wrap_opt!
        if !isnull(wo.value) {
          resolve_class_value(wo.value!, renames)
        }
      }
    }
    ExUnwrapOptional => {
      if !isnull(e.unwrap_opt) {
        let uo = e.unwrap_opt!
        if !isnull(uo.value) {
          resolve_class_value(uo.value!, renames)
        }
      }
    }
    ExIsNull => {
      if !isnull(e.is_null) {
        let in_data = e.is_null!
        if !isnull(in_data.value) {
          resolve_class_value(in_data.value!, renames)
        }
      }
    }
    ExMakeResult => {
      if !isnull(e.make_result) {
        let mr = e.make_result!
        if !isnull(mr.value) {
          resolve_class_value(mr.value!, renames)
        }
        if !isnull(mr.err) {
          resolve_class_value(mr.err!, renames)
        }
      }
    }
    ExIndexGet => {
      if !isnull(e.index_get) {
        let ig = e.index_get!
        if !isnull(ig.collection) {
          resolve_class_value(ig.collection!, renames)
        }
        if !isnull(ig.index) {
          resolve_class_value(ig.index!, renames)
        }
      }
    }
    ExMakeSlice => {}
    ExStructLit => {
      if !isnull(e.struct_lit) {
        let sl = e.struct_lit!
        let mut j = 0
        while j < len(sl.fields) {
          if !isnull(sl.fields[j].value) {
            resolve_class_value(sl.fields[j].value!, renames)
          }
          j = j + 1
        }
      }
    }
    ExVariantConstruct => {
      if !isnull(e.variant_construct) {
        let vc = e.variant_construct!
        let mut j = 0
        while j < len(vc.fields) {
          if !isnull(vc.fields[j]) {
            resolve_class_value(vc.fields[j]!, renames)
          }
          j = j + 1
        }
      }
    }
    ExVariantData => {
      if !isnull(e.variant_data) {
        let vd = e.variant_data!
        if !isnull(vd.value) {
          resolve_class_value(vd.value!, renames)
        }
      }
    }
    ExVariantTag => {
      if !isnull(e.variant_tag) {
        let vt = e.variant_tag!
        if !isnull(vt.value) {
          resolve_class_value(vt.value!, renames)
        }
      }
    }
    ExExtractValue => {
      if !isnull(e.extract_value) {
        let ev = e.extract_value!
        if !isnull(ev.value) {
          resolve_class_value(ev.value!, renames)
        }
      }
    }
    ExExtractError => {
      if !isnull(e.extract_error) {
        let ee = e.extract_error!
        if !isnull(ee.value) {
          resolve_class_value(ee.value!, renames)
        }
      }
    }
    ExUnOp => {
      if !isnull(e.un_op) {
        let uo = e.un_op!
        if !isnull(uo.operand) {
          resolve_class_value(uo.operand!, renames)
        }
      }
    }
    _ => {}
  }
}

func resolve_class_value(v: LValue, renames: Dict<Sym, string>) {
  resolve_class_type(v.typ, renames)
}

func resolve_class_type(t: LType?, renames: Dict<Sym, string>) {
  if isnull(t) {
    return
  }
  if t!.kind == TyClassHandle {
    let entry = renames.get(sym(t!.name))
    if !isnull(entry) {
      t!.name = entry!.value
    }
  }
  resolve_class_type(t!.elem, renames)
  resolve_class_type(t!.key, renames)
  resolve_class_type(t!.ret, renames)
  let mut i = 0
  while i < len(t!.type_args) {
    resolve_class_type(t!.type_args[i], renames)
    i = i + 1
  }
  i = 0
  while i < len(t!.fields) {
    resolve_class_type(t!.fields[i].typ, renames)
    i = i + 1
  }
  i = 0
  while i < len(t!.params) {
    resolve_class_type(t!.params[i], renames)
    i = i + 1
  }
}

// ---------------------------------------------------------------------------
// Main entry point
// ---------------------------------------------------------------------------

func monomorphize(prog: LProgram?) {
  if isnull(prog) {
    return
  }
  let m = new_mono_pass(prog)!

  // Index declarations
  let mut i = 0
  while i < len(prog!.functions) {
    let f = prog!.functions[i]
    let key = m.func_key(f)
    m.func_by_name!.set(sym(key), f)
    if f.receiver != "" {
      let existing = m.methods_by_class!.get(sym(f.receiver))
      if isnull(existing) {
        m.methods_by_class!.set(sym(f.receiver), [f])
      } else {
        let mut methods = existing!.value
        append(methods, f)
        m.methods_by_class!.set(sym(f.receiver), methods)
      }
    }
    i = i + 1
  }
  i = 0
  while i < len(prog!.classes) {
    m.class_by_name!.set(sym(prog!.classes[i].name), prog!.classes[i])
    i = i + 1
  }
  i = 0
  while i < len(prog!.structs) {
    m.struct_by_name!.set(sym(prog!.structs[i].name), prog!.structs[i])
    i = i + 1
  }

  // Phase 1a: Collect from field types
  i = 0
  while i < len(prog!.classes) {
    let mut j = 0
    while j < len(prog!.classes[i].fields) {
      m.collect_from_type(prog!.classes[i].fields[j].typ)
      j = j + 1
    }
    i = i + 1
  }
  i = 0
  while i < len(prog!.structs) {
    let mut j = 0
    while j < len(prog!.structs[i].fields) {
      m.collect_from_type(prog!.structs[i].fields[j].typ)
      j = j + 1
    }
    i = i + 1
  }

  // Phase 1b: Collect from function bodies
  i = 0
  while i < len(prog!.functions) {
    m.collect_from_stmts(prog!.functions[i].body)
    i = i + 1
  }

  // Phase 2: Generate specialized copies (iterate until no new instances)
  let mut new_funcs: [LFuncDecl] = []
  let mut new_classes: [LClassDecl] = []
  let mut new_structs: [LStructDecl] = []
  let mut generated_funcs = Dict<Sym, bool>()
  let mut generated_classes = Dict<Sym, bool>()
  let mut generated_structs = Dict<Sym, bool>()

  let mut changed = true
  while changed {
    changed = false

    // Specialize generic functions
    let func_keys = m.func_instances!.keys()
    let mut fi = 0
    while fi < len(func_keys) {
      let key = func_keys[fi]
      let gen_entry = generated_funcs.get(key)
      if !isnull(gen_entry) {
        fi = fi + 1
        continue
      }
      // Parse "name|typekey" 
      let parts = str_split(key.get_name(), "|")
      if len(parts) < 2 {
        fi = fi + 1
        continue
      }
      let func_name = parts[0]
      let orig_entry = m.func_by_name!.get(sym(func_name))
      if isnull(orig_entry) {
        fi = fi + 1
        continue
      }
      let orig = orig_entry!.value
      if len(orig.type_params) == 0 {
        fi = fi + 1
        continue
      }
      let types = get_inst_types(m.func_inst_types, m.func_inst_count, key.get_name())
      if len(types) != len(orig.type_params) {
        fi = fi + 1
        continue
      }
      generated_funcs.set(key, true)
      changed = true
      let subst = Dict<Sym, LType?>()
      let mut ti = 0
      while ti < len(orig.type_params) {
        subst.set(sym(orig.type_params[ti].name), types[ti])
        ti = ti + 1
      }
      // For interface methods with type-param receivers, use concrete receiver name
      let mut new_receiver = ""
      if orig.receiver != "" {
        let recv_type = subst.get(sym(orig.receiver))
        if !isnull(recv_type) && !isnull(recv_type!.value) {
          new_receiver = recv_type!.value!.name
        }
      }
      let mangled = if new_receiver != "" { orig.name } else { mangle_name(orig.name, types) }
      let spec = m.specialize_func(orig, subst, mangled, new_receiver)
      m.collect_from_stmts(spec.body)
      append(new_funcs, spec)
      fi = fi + 1
    }

    // Specialize generic classes + methods
    let class_keys = m.class_instances!.keys()
    let mut ci = 0
    while ci < len(class_keys) {
      let key = class_keys[ci]
      let gen_entry = generated_classes.get(key)
      if !isnull(gen_entry) {
        ci = ci + 1
        continue
      }
      let parts = str_split(key.get_name(), "|")
      if len(parts) < 2 {
        ci = ci + 1
        continue
      }
      let class_name = parts[0]
      let orig_entry = m.class_by_name!.get(sym(class_name))
      if isnull(orig_entry) {
        ci = ci + 1
        continue
      }
      let orig = orig_entry!.value
      if len(orig.type_params) == 0 {
        ci = ci + 1
        continue
      }
      let types = get_inst_types(m.class_inst_types, m.class_inst_count, key.get_name())
      if len(types) != len(orig.type_params) {
        ci = ci + 1
        continue
      }
      generated_classes.set(key, true)
      changed = true
      let subst = Dict<Sym, LType?>()
      let mut ti = 0
      while ti < len(orig.type_params) {
        subst.set(sym(orig.type_params[ti].name), types[ti])
        ti = ti + 1
      }
      let mangled = mangle_name(orig.name, types)
      m.class_renames!.set(sym(orig.name), mangled)
      let spec = m.specialize_class(orig, subst, mangled)
      // Collect from specialized fields for transitive discovery
      let mut fi2 = 0
      while fi2 < len(spec.fields) {
        m.collect_from_type(spec.fields[fi2].typ)
        fi2 = fi2 + 1
      }
      append(new_classes, spec)

      // Propagate owned status to specialized class
      if !isnull(prog!.owned_classes) {
        let owned_entry = prog!.owned_classes!.get(sym(orig.name))
        if !isnull(owned_entry) {
          prog!.owned_classes!.set(sym(mangled), true)
        }
      }

      // Specialize methods
      let methods_entry = m.methods_by_class!.get(sym(class_name))
      if !isnull(methods_entry) {
        let methods = methods_entry!.value
        let mut mi = 0
        while mi < len(methods) {
          let method = methods[mi]
          let meth_key = f"{key}.{method.name}"
          let meth_gen = generated_funcs.get(sym(meth_key))
          if isnull(meth_gen) {
            generated_funcs.set(sym(meth_key), true)
            let spec_method = m.specialize_func(method, subst, method.name, mangled)
            m.collect_from_stmts(spec_method.body)
            append(new_funcs, spec_method)
          }
          mi = mi + 1
        }
      }
      ci = ci + 1
    }

    // Specialize generic structs
    let struct_keys = m.struct_instances!.keys()
    let mut si = 0
    while si < len(struct_keys) {
      let key = struct_keys[si]
      let gen_entry = generated_structs.get(key)
      if !isnull(gen_entry) {
        si = si + 1
        continue
      }
      let parts = str_split(key.get_name(), "|")
      if len(parts) < 2 {
        si = si + 1
        continue
      }
      let struct_name = parts[0]
      let orig_entry = m.struct_by_name!.get(sym(struct_name))
      if isnull(orig_entry) {
        si = si + 1
        continue
      }
      let orig = orig_entry!.value
      if len(orig.type_params) == 0 {
        si = si + 1
        continue
      }
      let types = get_inst_types(m.struct_inst_types, m.struct_inst_count, key.get_name())
      if len(types) != len(orig.type_params) {
        si = si + 1
        continue
      }
      generated_structs.set(key, true)
      changed = true
      let subst = Dict<Sym, LType?>()
      let mut ti = 0
      while ti < len(orig.type_params) {
        subst.set(sym(orig.type_params[ti].name), types[ti])
        ti = ti + 1
      }
      let mangled = mangle_name(struct_name, types)
      let spec = m.specialize_struct(orig, subst, mangled)
      let mut fi3 = 0
      while fi3 < len(spec.fields) {
        m.collect_from_type(spec.fields[fi3].typ)
        fi3 = fi3 + 1
      }
      append(new_structs, spec)
      si = si + 1
    }
  }

  // Phase 3: Remove generic declarations, add specialized ones
  prog!.functions = filter_funcs(prog!.functions, m.class_instances, m.class_by_name)
  let mut fi4 = 0
  while fi4 < len(new_funcs) {
    append(prog!.functions, new_funcs[fi4])
    fi4 = fi4 + 1
  }
  prog!.classes = filter_classes(prog!.classes, m.class_instances)
  let mut ci2 = 0
  while ci2 < len(new_classes) {
    append(prog!.classes, new_classes[ci2])
    ci2 = ci2 + 1
  }
  prog!.structs = filter_structs(prog!.structs, m.struct_instances)
  let mut si2 = 0
  while si2 < len(new_structs) {
    append(prog!.structs, new_structs[si2])
    si2 = si2 + 1
  }

  // Phase 4: Rewrite call sites
  i = 0
  while i < len(prog!.functions) {
    m.rewrite_stmts(prog!.functions[i].body)
    i = i + 1
  }

  // Phase 5: Rewrite signatures and field types
  // LFuncDecl/LClassDecl/LStructDecl are structs in slices.
  // Must copy-out, modify, write-back to persist changes.
  i = 0
  while i < len(prog!.functions) {
    let mut f = prog!.functions[i]
    let mut j = 0
    while j < len(f.params) {
      f.params[j].typ = m.subst_type_remove_vars(f.params[j].typ)
      j = j + 1
    }
    f.return_type = m.subst_type_remove_vars(f.return_type)
    prog!.functions[i] = f
    i = i + 1
  }
  i = 0
  while i < len(prog!.classes) {
    let mut c = prog!.classes[i]
    let mut j = 0
    while j < len(c.fields) {
      c.fields[j].typ = m.subst_type_remove_vars(c.fields[j].typ)
      j = j + 1
    }
    prog!.classes[i] = c
    i = i + 1
  }
  i = 0
  while i < len(prog!.structs) {
    let mut s = prog!.structs[i]
    let mut j = 0
    while j < len(s.fields) {
      s.fields[j].typ = m.subst_type_remove_vars(s.fields[j].typ)
      j = j + 1
    }
    prog!.structs[i] = s
    i = i + 1
  }

  // Phase 6: Resolve bare generic class names using per-function ClassRenameMap
  resolve_class_names(prog)

  // Export rename map
  prog!.class_renames = m.class_renames
}


// ---------------------------------------------------------------------------
// Rewrite impl renames — called after monomorphize()
// ---------------------------------------------------------------------------

func mono_starts_with(s: string, prefix: string) -> bool {
  if len(prefix) > len(s) { return false }
  return s[0:len(prefix)] == prefix
}

func rewrite_impl_renames(prog: LProgram?) {
  if isnull(prog) { return }
  if isnull(prog!.impl_method_renames) { return }
  let rename_keys = prog!.impl_method_renames!.keys()
  if len(rename_keys) == 0 { return }

  // Build flat (className + "@" + methodName) -> concreteName lookup.
  // When multiple interfaces map the same class+method to different concrete
  // names, skip that entry — the per-specialization rewrite already handled it.
  let renames = Dict<Sym, string>()
  let conflicts = Dict<Sym, bool>()
  let mut i = 0
  while i < len(rename_keys) {
    let key = rename_keys[i]
    let new_name_entry = prog!.impl_method_renames!.get(key)
    if !isnull(new_name_entry) {
      // Key format: "Interface@TypeArg0@...@ClassName@MethodName"
      let parts = str_split(key.get_name(), "@")
      if len(parts) >= 3 {
        let class_name = parts[len(parts) - 2]
        let method_name = parts[len(parts) - 1]
        let flat_key = f"{class_name}@{method_name}"
        let existing = renames.get(sym(flat_key))
        if !isnull(existing) {
          if existing!.value != new_name_entry!.value {
            // Conflict: two interfaces map same class+method to different targets.
            // Mark as conflicting — per-specialization rewrite handles these.
            conflicts.set(sym(flat_key), true)
          }
        } else {
          renames.set(sym(flat_key), new_name_entry!.value)
        }
      }
    }
    i = i + 1
  }
  // Remove conflicting entries
  let conflict_keys = conflicts.keys()
  i = 0
  while i < len(conflict_keys) {
    renames.remove(conflict_keys[i])
    i = i + 1
  }

  let flat_keys = renames.keys()
  if len(flat_keys) == 0 { return }

  // Expand renames to cover monomorphized class names
  // E.g., DictEntry.set_parent -> set_d_parent should also apply to
  // DictEntry_CClassDecl.set_parent -> set_d_parent
  i = 0
  while i < len(prog!.functions) {
    let f = prog!.functions[i]
    if f.receiver != "" {
      let mut j = 0
      while j < len(flat_keys) {
        let fk = flat_keys[j]
        let fk_parts = str_split(fk.get_name(), "@")
        if len(fk_parts) == 2 {
          let base_class = fk_parts[0]
          let method = fk_parts[1]
          if mono_starts_with(f.receiver, f"{base_class}_") {
            let mono_key = f"{f.receiver}@{method}"
            let existing = renames.get(sym(mono_key))
            if isnull(existing) {
              let val = renames.get(fk)
              if !isnull(val) {
                renames.set(sym(mono_key), val!.value)
              }
            }
          }
        }
        j = j + 1
      }
    }
    i = i + 1
  }

  // Also check class declarations
  i = 0
  while i < len(prog!.classes) {
    let c = prog!.classes[i]
    let mut j = 0
    while j < len(flat_keys) {
      let fk = flat_keys[j]
      let fk_parts = str_split(fk.get_name(), "@")
      if len(fk_parts) == 2 {
        let base_class = fk_parts[0]
        let method = fk_parts[1]
        if mono_starts_with(c.name, f"{base_class}_") {
          let mono_key = f"{c.name}@{method}"
          let existing = renames.get(sym(mono_key))
          if isnull(existing) {
            let val = renames.get(fk)
            if !isnull(val) {
              renames.set(sym(mono_key), val!.value)
            }
          }
        }
      }
      j = j + 1
    }
    i = i + 1
  }

  // Walk all function bodies
  i = 0
  while i < len(prog!.functions) {
    rewrite_impl_stmts(prog!.functions[i].body, renames)
    i = i + 1
  }
}

func rewrite_impl_stmts(stmts: [LStmt?], renames: Dict<Sym, string>?) {
  let mut i = 0
  while i < len(stmts) {
    if !isnull(stmts[i]) {
      rewrite_impl_stmt(stmts[i]!, renames)
    }
    i = i + 1
  }
}

func rewrite_impl_stmt(s: LStmt, renames: Dict<Sym, string>?) {
  match s.kind {
    StTempDef => {
      if !isnull(s.temp_def) {
        rewrite_impl_expr(s.temp_def!.expr, renames)
      }
    }
    StSideEffect => {
      if !isnull(s.side_effect) {
        rewrite_impl_expr(s.side_effect!.expr, renames)
      }
    }
    StIf => {
      if !isnull(s.if_data) {
        rewrite_impl_stmts(s.if_data!.then_body, renames)
        rewrite_impl_stmts(s.if_data!.else_body, renames)
      }
    }
    StWhile => {
      if !isnull(s.while_data) {
        rewrite_impl_stmts(s.while_data!.cond_block, renames)
        rewrite_impl_stmts(s.while_data!.body, renames)
      }
    }
    StBlock => {
      if !isnull(s.block) {
        rewrite_impl_stmts(s.block!.stmts, renames)
      }
    }
    StFor => {
      if !isnull(s.for_data) {
        rewrite_impl_stmts(s.for_data!.body, renames)
      }
    }
    _ => {}
  }
}

func rewrite_impl_expr(e: LExpr?, renames: Dict<Sym, string>?) {
  if isnull(e) { return }
  if e!.kind is ExMethodCall {
    if !isnull(e!.method_call) {
      let mc = e!.method_call!
      if !isnull(mc.receiver) {
        if !isnull(mc.receiver!.typ) {
          if mc.receiver!.typ!.kind is TyClassHandle {
            let key = f"{mc.receiver!.typ!.name}@{mc.method}"
            let entry = renames!.get(sym(key))
            if !isnull(entry) {
              mc.method = entry!.value
            }
          }
        }
      }
    }
  }
}