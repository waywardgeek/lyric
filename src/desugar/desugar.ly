// desugar.ly — AST desugaring passes for the Lyric bootstrap compiler
// Ported from pkg/ast/desugar.go
// Desugar order (must be maintained):
//   0. DesugarInterfaceExtends — materialize interface inheritance by
//      copying parent's members into child (Phase 4 Wave 1 / 4w1-b)
//   1. DesugarInterfaceFields — convert field decls to getter/setter methods
//   2. DesugarFieldAccess — rewrite self.field shorthand into getter/setter calls
//   3. DesugarRelations — inject fields + impl blocks from relations
//   4. DesugarDestructors — copy interface destructors to concrete classes
//   5. DesugarDefaultImpls — extract interface methods with bodies to top-level functions

lyric desugar {


  // ---- 0. DesugarInterfaceExtends ----
  // Materializes single-parent interface inheritance.
  //   interface Child<A, B> extends Parent<A, B> { ... }
  // gets the parent's abstract methods, fields, and destructors copied
  // into the child with parent's type-params substituted by the child's
  // extends_args. Default-bodied parent methods are copied with the body
  // deep-cloned + substituted. Child wins on name collision (matching
  // member already declared on the child).
  //
  // Runs BEFORE DesugarInterfaceFields so inherited fields get their
  // getter/setter pair synthesized by pass 1 on the child. The pass
  // resolves parent extends recursively (transitive inheritance) using
  // a visited set keyed by interface name to detect cycles.

  func desugar_interface_extends(file: File) {
    // Build cross-block index: interface name -> InterfaceDecl.
    let mut iface_index: Dict<Sym, InterfaceDecl> = Dict<Sym, InterfaceDecl>()
    for block in file.fb.children() {
      for iface in block.id.children() {
        if !isnull(iface.name) {
          iface_index.set(iface.name!, iface)
        }
      }
    }

    // Resolve every interface that has an extends clause. Use a visited
    // dict on the iface name so transitive extends and shared ancestors
    // each materialize exactly once per interface.
    let mut resolved: Dict<Sym, bool> = Dict<Sym, bool>()
    for block in file.fb.children() {
      for iface in block.id.children() {
        resolve_extends(iface, iface_index, resolved)
      }
    }
  }

  func resolve_extends(iface: InterfaceDecl, iface_index: Dict<Sym, InterfaceDecl>, resolved: Dict<Sym, bool>) {
    if isnull(iface.name) { return }
    if resolved.has(iface.name!) { return }
    resolved.set(iface.name!, true)
    if isnull(iface.extends_name) { return }

    let parent_entry = iface_index.get(iface.extends_name!)
    if isnull(parent_entry) { return }   // unresolved parent — checker will diagnose elsewhere
    let parent = parent_entry!.value

    // Resolve parent's own extends FIRST so we copy a fully-materialized parent.
    resolve_extends(parent, iface_index, resolved)

    // Build substitution: parent's type-param NAME -> child's extends arg name string.
    let mut type_map: Dict<Sym, string> = Dict<Sym, string>()
    let parent_tps = parent.itp.children()
    let mut limit = len(parent_tps)
    if len(iface.extends_args) < limit { limit = len(iface.extends_args) }
    for i in range(0, limit) {
      if !isnull(parent_tps[i].name) {
        type_map.set(parent_tps[i].name!, iface.extends_args[i].name)
      }
    }

    // Build child's existing-member name sets so child-wins-on-collision works.
    let mut child_method_names: Dict<Sym, bool> = Dict<Sym, bool>()
    for m in iface.im.children() {
      if !isnull(m.name) { child_method_names.set(m.name!, true) }
    }
    let mut child_field_names: Dict<Sym, bool> = Dict<Sym, bool>()
    for f in iface.ifd.children() {
      if !isnull(f.name) { child_field_names.set(f.name!, true) }
    }

    // Inherit fields.
    for pf in parent.ifd.children() {
      if !isnull(pf.name) && child_field_names.has(pf.name!) { continue }
      let nf = InterfaceFieldDecl {
        type_param: substitute_sym(pf.type_param, type_map),
        name: pf.name,
        type_expr: substitute_type_expr_copy(pf.type_expr, type_map),
        span: pf.span,
      }
      array_append<InterfaceDecl, InterfaceFieldDecl>(iface, nf)
    }

    // Inherit methods (abstract + default).
    for pm in parent.im.children() {
      if !isnull(pm.name) && child_method_names.has(pm.name!) { continue }
      let nm = deep_copy_func_decl(pm, type_map)
      array_append<InterfaceDecl, FuncDecl>(iface, nm)
    }

    // Inherit destructors (no name dedupe — destructors are positional by
    // type_param + kind, not by name).
    for pd in parent.idb.children() {
      let nd_body = deep_copy_block(pd.body)
      if !isnull(nd_body) {
        substitute_type_params_in_block(nd_body!, type_map)
      }
      let nd = DestructorBlock {
        type_param: substitute_sym(pd.type_param, type_map),
        kind: pd.kind,
        body: nd_body,
        span: pd.span,
      }
      array_append<InterfaceDecl, DestructorBlock>(iface, nd)
    }
  }

  // Deep-copy a FuncDecl, substituting type-param names in the receiver,
  // param types, return type, where-clauses, and body. Inheriting an
  // interface method needs a fresh AST node so child-side mutations
  // (later desugar passes) don't bleed back into the parent.
  func deep_copy_func_decl(f: FuncDecl, type_map: Dict<Sym, string>) -> FuncDecl {
    let new_fn = FuncDecl {
      name: f.name,
      is_public: f.is_public,
      is_final: f.is_final,
      is_trusted: f.is_trusted,
      is_synthesized: f.is_synthesized,
      receiver_type: substitute_sym(f.receiver_type, type_map),
      return_type: substitute_type_expr_copy(f.return_type, type_map),
      body: deep_copy_block(f.body),
      span: f.span,
    }
    for tp in f.fp.children() {
      let new_tp = TypeParam {
        name: tp.name,
        constraint: tp.constraint,
        span: tp.span,
      }
      array_append<FuncDecl, TypeParam>(new_fn, new_tp)
    }
    for p in f.param.children() {
      let new_p = Param {
        name: p.name,
        type_expr: substitute_type_expr_copy(p.type_expr, type_map),
        is_mut: p.is_mut,
        is_self: p.is_self,
        span: p.span,
      }
      array_append<FuncDecl, Param>(new_fn, new_p)
    }
    for wc in f.wc.children() {
      let new_wc = WhereClause {
        variable: wc.variable,
        constraint: wc.constraint,
        span: wc.span,
      }
      for wa in wc.wc_arg.children() {
        let new_wa = TypeExpr { kind: wa.kind, span: wa.span }
        substitute_type_params_in_type_expr(new_wa, type_map)
        array_append<WhereClause, TypeExpr>(new_wc, new_wa)
      }
      array_append<FuncDecl, WhereClause>(new_fn, new_wc)
    }
    if !isnull(new_fn.body) {
      substitute_type_params_in_block(new_fn.body!, type_map)
    }
    return new_fn
  }


  // ---- 2. DesugarInterfaceFields ----
  // Converts interface field declarations into getter/setter methods.
  //   field P.first: C?  =>  func P.first(self) -> C?  +  func P.set_first(mut self, val: C?)

  func desugar_interface_fields(file: File) {
    for block in file.fb.children() {
      for iface in block.id.children() {
        for fd in iface.ifd.children() {
          // Getter: func T.name(self) -> Type
          let self_param = Param {
            name: `self`,
            type_expr: null,
            is_mut: false,
            is_self: true,
            span: fd.span,
          }
          let getter = FuncDecl {
            name: fd.name,
            is_public: false,
            is_synthesized: true,
            receiver_type: fd.type_param,
            return_type: fd.type_expr,
            body: null,
            span: fd.span,
          }
          array_append<FuncDecl, Param>(getter, self_param)
          array_append<InterfaceDecl, FuncDecl>(iface, getter)

          // Setter: func T.set_name(mut self, val: Type)
          let mut setter_name: Sym? = null
          if !isnull(fd.name) {
            setter_name = sym("set_" + fd.name!.name)
          }
          let self_param2 = Param {
            name: `self`,
            type_expr: null,
            is_mut: true,
            is_self: true,
            span: fd.span,
          }
          let val_param = Param {
            name: `val`,
            type_expr: fd.type_expr,
            is_mut: false,
            is_self: false,
            span: fd.span,
          }
          let setter = FuncDecl {
            name: setter_name,
            is_public: false,
            is_synthesized: true,
            receiver_type: fd.type_param,
            return_type: null,
            body: null,
            span: fd.span,
          }
          array_append<FuncDecl, Param>(setter, self_param2)
          array_append<FuncDecl, Param>(setter, val_param)
          array_append<InterfaceDecl, FuncDecl>(iface, setter)
        }
      }
    }
  }

  // ---- 3. DesugarRelations ----
  // Processes relation declarations: injects fields into concrete classes
  // ---- 3. DesugarRelations ----
  // Two phases:
  //
  //   Phase A: synthesize an impl skeleton from each relation declaration.
  //     For each `relation Hint Parent:l1 owns/refs [Child:l2]`, find or
  //     create a matching `impl Hint<Parent, Child>` block, drop the
  //     per-side labels into ib_args[i].label, and drop rel.kind into
  //     impl.kind. No mapping/field injection yet.
  //
  //   Phase B: materialize every ownership-bearing impl block.
  //     For each impl block with non-null kind whose interface declares
  //     hint shape (field T.name: Type decls), inject label-prefixed
  //     fields onto the concrete classes named by ib_args[i].type_expr
  //     and emit FieldBind mappings (deduped against any user-authored
  //     mappings already on the impl).
  //
  // Phase B reads everything off the impl, so a user-authored
  // `impl Hint<A:l1, B:l2> owns { }` over a user-defined hint interface
  // gets identical treatment to a desugared relation. See
  // cr/docs/multi-class-interface-redesign.md §3.9.

  func desugar_relations(file: File) {
    // Build GLOBAL interface lookup across ALL blocks
    let iface_map = Dict<Sym, InterfaceDecl>()
    for block in file.fb.children() {
      for iface in block.id.children() {
        if !isnull(iface.name) {
          iface_map.set(sym(iface.name!.name), iface)
        }
      }
    }

    // Build GLOBAL class lookup across ALL blocks
    // Relations may reference classes in other blocks (e.g., Lexer:lc owns [Comment:lc]
    // where Comment is in ast.ly but the relation is in lexer.ly)
    let class_map = Dict<Sym, ClassDecl>()
    for block in file.fb.children() {
      for cls in block.cd.children() {
        if !isnull(cls.name) {
          class_map.set(sym(cls.name!.name), cls)
        }
      }
    }

    for block in file.fb.children() {

      // ===== Phase A: relation → impl skeleton =====
      // Per-type-var labels (redesign §3.8) ride on ib_args[i].label.
      // Ownership annotation (redesign §3.9) rides on impl.kind. No
      // mapping generation here; Phase B handles that uniformly for
      // user-authored ownership impls as well.
      for rel in block.rd.children() {
        if isnull(rel.hint) { continue }
        let mut parent_name: string = ""
        if !isnull(rel.parent.type_name) {
          parent_name = rel.parent.type_name!.name
        }
        let mut child_name: string = ""
        if !isnull(rel.child.type_name) {
          child_name = rel.child.type_name!.name
        }
        if parent_name == "" || child_name == "" { continue }

        // Look for existing impl block matching (hint, parent, child).
        // Per-side labels are part of the relation's identity: an existing
        // impl with non-null labels that DON'T match this relation's labels
        // is a different relation (e.g. `Node:fwd refs [Edge:a]` vs
        // `Node:bwd refs [Edge:b]` — two independent DLLs over the same
        // (Node, Edge) pair). We only merge when both labels are either
        // null (slot still open) or match the relation's labels exactly.
        let rel_p_label = if !isnull(rel.parent.label) { rel.parent.label!.name } else { "" }
        let rel_c_label = if !isnull(rel.child.label) { rel.child.label!.name } else { "" }
        let mut existing: ImplBlock? = null
        for ib in block.ib.children() {
          if !isnull(ib.interface_name) && ib.interface_name!.name == rel.hint!.name {
            let ta = ib.ib_arg.children()
            if len(ta) >= 2 {
              let n0 = if !isnull(ta[0].type_expr) { type_expr_name(ta[0].type_expr!) } else { null }
              let n1 = if !isnull(ta[1].type_expr) { type_expr_name(ta[1].type_expr!) } else { null }
              if !isnull(n0) && !isnull(n1) && n0!.name == parent_name && n1!.name == child_name {
                let p_lab = if !isnull(ta[0].label) { ta[0].label!.name } else { "" }
                let c_lab = if !isnull(ta[1].label) { ta[1].label!.name } else { "" }
                let p_ok = (p_lab == "") || (rel_p_label != "" && p_lab == rel_p_label)
                let c_ok = (c_lab == "") || (rel_c_label != "" && c_lab == rel_c_label)
                if p_ok && c_ok {
                  existing = ib
                }
              }
            }
          }
        }

        if !isnull(existing) {
          // Drop labels and kind onto existing impl if slots are empty.
          // User-authored values win; relation-synthesized values fill gaps.
          let ex_args = existing!.ib_arg.children()
          if len(ex_args) >= 2 {
            if isnull(ex_args[0].label) && !isnull(rel.parent.label) {
              ex_args[0].label = rel.parent.label
            }
            if isnull(ex_args[1].label) && !isnull(rel.child.label) {
              ex_args[1].label = rel.child.label
            }
          }
          if isnull(existing!.kind) {
            existing!.kind = rel.kind
          }
        } else {
          let new_ib = ImplBlock {
            interface_name: rel.hint,
            kind: rel.kind,
            span: rel.span,
          }
          let mut parent_args: [TypeExpr] = []
          for ta in rel.parent.type_args {
            append(parent_args, TypeExpr { kind: Named(ta, []), span: rel.span })
          }
          let ta0 = TypeExpr { kind: Named(sym(parent_name), parent_args), span: rel.span }
          let mut child_args_te: [TypeExpr] = []
          for ta in rel.child.type_args {
            append(child_args_te, TypeExpr { kind: Named(ta, []), span: rel.span })
          }
          let ta1 = TypeExpr { kind: Named(sym(child_name), child_args_te), span: rel.span }
          let ita0 = ImplTypeArg { type_expr: ta0, label: rel.parent.label, span: rel.span }
          let ita1 = ImplTypeArg { type_expr: ta1, label: rel.child.label, span: rel.span }
          array_append<ImplBlock, ImplTypeArg>(new_ib, ita0)
          array_append<ImplBlock, ImplTypeArg>(new_ib, ita1)
          array_append<LyricBlock, ImplBlock>(block, new_ib)
        }
      }

      // ===== Phase B: materialize ownership impls =====
      // Iterate impls (NOT relations) so user-authored
      // `impl Hint<A:l, B:l> owns { }` over user-defined hint interfaces
      // gets identical treatment to a relation-synthesized impl.
      for impl_block in block.ib.children() {
        if isnull(impl_block.kind) { continue }
        if isnull(impl_block.interface_name) { continue }
        let iface_entry = iface_map.get(sym(impl_block.interface_name!.name))
        if isnull(iface_entry) { continue }
        let iface = iface_entry!.value

        let iface_fields = iface.ifd.children()
        if len(iface_fields) == 0 { continue }

        let iface_tps = iface.itp.children()
        if len(iface_tps) < 2 { continue }

        let impl_args = impl_block.ib_arg.children()
        if len(impl_args) < 2 { continue }

        // Snapshot existing mappings for dedup. User-authored
        // FieldBind mappings win over auto-synthesized ones. The
        // snapshot is read-only; new mappings are buffered into
        // `new_mappings` and appended AFTER the per-field loop, so
        // that mid-loop reallocation of impl_block's mapping backing
        // can't invalidate the slice we're iterating.
        let existing_mappings = impl_block.ibm.children()
        let mut new_mappings: [ImplMapping] = []

        for fd in iface_fields {
          if isnull(fd.type_param) { continue }
          let tp_name = fd.type_param!.name

          // Find which iface type-param slot this field references.
          let mut slot: i32 = -1
          if !isnull(iface_tps[0].name) && tp_name == iface_tps[0].name!.name {
            slot = 0
          } else if !isnull(iface_tps[1].name) && tp_name == iface_tps[1].name!.name {
            slot = 1
          }
          if slot < 0 { continue }

          let ta = impl_args[slot]
          if isnull(ta.type_expr) { continue }
          let side_name_sym = type_expr_name(ta.type_expr!)
          if isnull(side_name_sym) { continue }
          let side_name = side_name_sym!.name
          let mut side_label: string = ""
          if !isnull(ta.label) { side_label = ta.label!.name }

          let cls_entry = class_map.get(sym(side_name))
          if isnull(cls_entry) { continue }
          let cls = cls_entry!.value

          // Build field name with optional label prefix.
          let mut field_name: string = ""
          if !isnull(fd.name) {
            field_name = fd.name!.name
          }
          if side_label != "" {
            // Phase 3e (redesign §3.1): scope-injected names are mangled
            // with a double-underscore prefix so they're unreachable by
            // bare-flat access (`team.roster_children` errors as "no such
            // field"). The dotted-scope sugar (`team.roster.children`)
            // is the only user-visible path. Mangling also frees the
            // user namespace — `class Team { roster_children: ... }`
            // can coexist with `relation ArrayList Team:roster owns [...]`.
            field_name = "__" + side_label + "_" + field_name
          }

          // Rewrite the field's type, substituting iface type-params
          // with the impl's per-side concrete TypeExpr.
          let field_type = rewrite_field_type_impl(fd.type_expr, iface_tps, impl_args)

          // Inject the field into the concrete class. Always: a user-
          // authored impl mapping like `P.children <-> Team.roster_children`
          // names a class field that this pass is responsible for
          // injecting; deduping field injection on the mapping presence
          // would leave the binding dangling.
          let new_field = Field {
            name: sym(field_name),
            is_public: false,
            type_expr: field_type,
            default_value: null,
            guarded_by: null,
            span: fd.span,
          }
          array_append<ClassDecl, Field>(cls, new_field)

          // Generate getter mapping unless user already provided one for
          // (type_param, fd.name). User-authored mappings win.
          let mut getter_user_bound = false
          for em in existing_mappings {
            if (!isnull(em.type_param) && !isnull(fd.type_param) &&
                em.type_param!.name == fd.type_param!.name &&
                !isnull(em.method_name) && !isnull(fd.name) &&
                em.method_name!.name == fd.name!.name) {
              getter_user_bound = true
            }
          }
          if !getter_user_bound {
            let getter_mapping = ImplMapping {
              type_param: fd.type_param,
              method_name: fd.name,
              kind: FieldBind,
              target_class: sym(side_name),
              target_member: sym(field_name),
              inline_func: null,
              span: fd.span,
            }
            new_mappings = append(new_mappings, getter_mapping)
          }

          // Generate setter mapping unless user already provided one for
          // (type_param, set_<fd.name>).
          let mut setter_name: Sym? = null
          if !isnull(fd.name) {
            setter_name = sym("set_" + fd.name!.name)
          }
          let mut setter_user_bound = false
          for em in existing_mappings {
            if (!isnull(em.type_param) && !isnull(fd.type_param) &&
                em.type_param!.name == fd.type_param!.name &&
                !isnull(em.method_name) && !isnull(setter_name) &&
                em.method_name!.name == setter_name!.name) {
              setter_user_bound = true
            }
          }
          if !setter_user_bound {
            let setter_mapping = ImplMapping {
              type_param: fd.type_param,
              method_name: setter_name,
              kind: FieldBind,
              target_class: sym(side_name),
              target_member: sym(field_name),
              inline_func: null,
              span: fd.span,
            }
            new_mappings = append(new_mappings, setter_mapping)
          }
        }
        // Flush buffered mappings onto the impl in a single pass.
        for m in new_mappings {
          array_append<ImplBlock, ImplMapping>(impl_block, m)
        }
      }

      // ===== Phase C: populate SubScope records on bound classes =====
      // Tier 1 of the sub-scope refactor (cr/docs/sub-scope-refactor.md):
      // for every impl-with-kind, append one SubScope per labeled side
      // onto the bound class. Metadata only — no member migration. The
      // checker reads ClassDecl.css to fire a precise collision
      // diagnostic when a user-declared field/method shares a name with
      // a relation label (e.g. graph.ly Part 3: user method `Network.nodes`
      // colliding with relation label `Network:nodes`).
      //
      // Iterates impls (not relations) so user-authored
      // `impl Hint<A:l, B:l> owns { }` over user-defined hint interfaces
      // gets identical treatment to a relation-synthesized impl.
      for impl_block in block.ib.children() {
        if isnull(impl_block.kind) { continue }
        if isnull(impl_block.interface_name) { continue }
        let impl_args = impl_block.ib_arg.children()
        let mut side_idx: i32 = 0
        while side_idx < len(impl_args) as i32 {
          let ta = impl_args[side_idx]
          if !isnull(ta.label) && !isnull(ta.type_expr) {
            let side_name_sym = type_expr_name(ta.type_expr!)
            if !isnull(side_name_sym) {
              let cls_entry = class_map.get(sym(side_name_sym!.name))
              if !isnull(cls_entry) {
                let cls = cls_entry!.value
                let scope = SubScope {
                  label: ta.label,
                  hint_iface: impl_block.interface_name,
                  side_index: side_idx,
                  span: ta.span,
                }
                array_append<ClassDecl, SubScope>(cls, scope)
              }
            }
          }
          side_idx = side_idx + 1
        }
      }
    }
  }
  // ---- 4. DesugarDestructors ----
  // Generates destroy methods on classes named by ownership-bearing
  // impl blocks (impl.kind != null) by deep-copying the hint interface's
  // matching destructor blocks onto each concrete class. Iterates impls
  // (not relations) so a user-authored
  // `impl Hint<A:l, B:l> owns { }` over a user-defined hint interface
  // gets identical treatment to a relation-synthesized impl. See
  // cr/docs/multi-class-interface-redesign.md §3.9.

  func desugar_destructors(file: File) {
    // Build GLOBAL interface lookup across ALL blocks
    let iface_map = Dict<Sym, InterfaceDecl>()
    for block in file.fb.children() {
      for iface in block.id.children() {
        if !isnull(iface.name) {
          iface_map.set(sym(iface.name!.name), iface)
        }
      }
    }

    // Build GLOBAL class lookup across ALL blocks
    let class_map = Dict<Sym, ClassDecl>()
    for block in file.fb.children() {
      for cls in block.cd.children() {
        if !isnull(cls.name) {
          class_map.set(sym(cls.name!.name), cls)
        }
      }
    }

    for block in file.fb.children() {
      // Collect destructor blocks per class name. One destroy method
      // per class; multiple ownership impls touching the same class
      // append wrapped block-stmts.
      let destroy_methods = Dict<Sym, FuncDecl>()

      for impl_block in block.ib.children() {
        if isnull(impl_block.kind) { continue }
        if isnull(impl_block.interface_name) { continue }
        let iface_entry = iface_map.get(sym(impl_block.interface_name!.name))
        if isnull(iface_entry) { continue }
        let iface = iface_entry!.value

        let destructors = iface.idb.children()
        if len(destructors) == 0 { continue }

        let iface_tps = iface.itp.children()
        if len(iface_tps) < 2 { continue }

        let impl_args = impl_block.ib_arg.children()
        if len(impl_args) < 2 { continue }

        // Map type-param-name → concrete class name (simple string map).
        let type_map = Dict<Sym, string>()
        // Rich type map (preserves TypeArgs for generic instantiation).
        let rich_type_map = Dict<Sym, TypeExpr>()
        // Map type-param-name → label (for method renaming).
        let type_param_to_label = Dict<Sym, string>()

        let mut slot: i32 = 0
        while slot < 2 {
          if !isnull(iface_tps[slot].name) && !isnull(impl_args[slot].type_expr) {
            let tp_name = iface_tps[slot].name!
            let te = impl_args[slot].type_expr!
            let name_sym = type_expr_name(te)
            if !isnull(name_sym) {
              type_map.set(sym(tp_name.name), name_sym!.name)
            }
            // Rich map: bind type-param to a fresh TypeExpr cloned from
            // the impl's per-side type-arg, preserving its generic args.
            let te_copy = TypeExpr { kind: te.kind, span: te.span }
            rich_type_map.set(sym(tp_name.name), te_copy)
            if !isnull(impl_args[slot].label) {
              type_param_to_label.set(sym(tp_name.name), impl_args[slot].label!.name)
            }
          }
          slot = slot + 1
        }

        // Build method rename map for label-prefixed fields. For each
        // interface field whose type-param-side carries a label, the
        // generic getter/setter call inside a destructor body is
        // renamed to its label-prefixed concrete form.
        let method_renames = Dict<Sym, string>()
        for ifield in iface.ifd.children() {
          if isnull(ifield.type_param) { continue }
          let label_entry = type_param_to_label.get(sym(ifield.type_param!.name))
          if !isnull(label_entry) && !isnull(ifield.name) {
            let label = label_entry!.value
            let fname = ifield.name!.name
            method_renames.set(sym(fname), "__" + label + "_" + fname)
            method_renames.set(sym("set_" + fname), "set___" + label + "_" + fname)
          }
        }

        for db in destructors {
          if isnull(db.type_param) { continue }
          // Skip destructors not matching the impl's owns/refs kind.
          // Default (legacy) destructors carry kind=Owns and so still
          // apply to owns impls.
          if db.kind is Owns {
            if !(impl_block.kind! is Owns) { continue }
          } else {
            if !(impl_block.kind! is Refs) { continue }
          }
          let mut class_name: string = ""
          let entry = type_map.get(sym(db.type_param!.name))
          if isnull(entry) { continue }
          class_name = entry!.value

          let cls_entry = class_map.get(sym(class_name))
          if isnull(cls_entry) { continue }

          // Deep copy and substitute type params (rich, preserving type args)
          let body_copy = deep_copy_block(db.body)
          if isnull(body_copy) { continue }
          substitute_type_params_rich_in_block(body_copy!, rich_type_map)
          // Rename generic interface method calls to label-prefixed versions
          rename_method_calls_in_block(body_copy!, method_renames)

          // Get or create destroy method for this class
          let method_entry = destroy_methods.get(sym(class_name))
          let mut method: FuncDecl? = null
          if !isnull(method_entry) {
            method = method_entry!.value
          } else {
            let self_param = Param {
              name: `self`,
              type_expr: null,
              is_mut: true,
              is_self: true,
              span: db.span,
            }
            let destroy_body = Block { span: db.span }
            let m = FuncDecl {
              name: `destroy`,
              is_public: true,
              receiver_type: null,
              return_type: null,
              body: destroy_body,
              span: db.span,
            }
            array_append<FuncDecl, Param>(m, self_param)
            destroy_methods.set(sym(class_name), m)
            method = m
          }

          // Wrap destructor body in a block stmt to avoid variable collisions
          let wrapper_stmt = Stmt {
            kind: BlockStmt(body_copy!),
            span: db.span,
          }
          array_append<Block, Stmt>(method!.body!, wrapper_stmt)
        }
      }

      // Attach destroy methods to classes
      // Attach destroy methods to classes
      for cls in block.cd.children() {
        if isnull(cls.name) { continue }
        let entry = destroy_methods.get(sym(cls.name!.name))
        if !isnull(entry) {
          array_append<ClassDecl, FuncDecl>(cls, entry!.value)
        }
      }
    }
  }
  // ---- 4.5. DesugarSpecializeDefaultImpls ----
  // For each plain `impl Iface<Concrete...> { aliases }` block, emit a
  // fully-substituted concrete copy of every default-bodied interface
  // method as a top-level function on the concrete receiver class. The
  // impl's type-arg bindings are known at desugar time, so we substitute
  // them in directly — no monomorphizer needed.
  //
  // Bodies reference the interface's named surface (`self.nodes()`,
  // `n.outgoing_edges()`). With the receiver substituted to the concrete
  // class and the loop-variable types substituted to their concrete
  // bindings, those calls resolve to alias-wrapper methods that the
  // lowerer synthesizes from the impl's ImplMappings (lowerer.ly:701,
  // `lower_impl_block`). End-to-end: this pass + the lowerer's existing
  // wrapper emission = working default-method calls without any
  // monomorphizer involvement.
  //
  // Runs BEFORE desugar_default_impls (pass 5) because that pass moves
  // the bodied FuncDecls off the interface and onto the block — we need
  // them still on the interface to deep-copy from.
  //
  // Override: if the impl explicitly binds the method (an ImplMapping
  // whose method_name matches), the user's binding wins and we skip
  // the specialized copy.
  //
  // Partial impls (`impl<W> Iface<..., W>`): the impl's own type-params
  // (ib.imtp) get carried onto the specialized function. The
  // monomorphizer's existing single-var specialization path handles the
  // remaining W per use site.
  //
  // Collision: if two impls produce the same `Class.method` specialization
  // (e.g. a class impls two unrelated interfaces with a same-named
  // default method, neither overriding), this is a user error. We panic
  // with a clear message pointing at the receiver+method; the user can
  // disambiguate by labeling one of the interfaces (Wave 2 work) or by
  // providing an explicit alias binding in one of the impls.

  func desugar_specialize_default_impls(file: File) {
    // Cross-block interface index.
    let mut iface_index: Dict<Sym, InterfaceDecl> = Dict<Sym, InterfaceDecl>()
    for block in file.fb.children() {
      for iface in block.id.children() {
        if !isnull(iface.name) {
          iface_index.set(iface.name!, iface)
        }
      }
    }

    // Collision tracker keyed on "ClassName.method_name".
    let mut emitted: Dict<Sym, bool> = Dict<Sym, bool>()

    for block in file.fb.children() {
      for ib in block.ib.children() {
        if isnull(ib.interface_name) { continue }
        if !isnull(ib.kind) { continue }    // owns/refs impl — destructor pass owns it

        let iface_entry = iface_index.get(ib.interface_name!)
        if isnull(iface_entry) { continue }
        let iface = iface_entry!.value

        // Build the substitution maps from iface type-params to impl bindings.
        // rich_map preserves full type args (needed for Dict<K,V>, FlexVia<W>);
        // str_map gives quick lookup of the top-level concrete class name for
        // receiver-type rewriting (which uses Sym, not TypeExpr).
        let iface_tps = iface.itp.children()
        let impl_args = ib.ib_arg.children()
        let mut rich_map: Dict<Sym, TypeExpr> = Dict<Sym, TypeExpr>()
        let mut str_map: Dict<Sym, string> = Dict<Sym, string>()
        let mut limit = len(iface_tps)
        if len(impl_args) < limit { limit = len(impl_args) }
        for i in range(0, limit) {
          if !isnull(iface_tps[i].name) && !isnull(impl_args[i].type_expr) {
            rich_map.set(iface_tps[i].name!, impl_args[i].type_expr!)
            match impl_args[i].type_expr!.kind {
              Named(nm, _) => { str_map.set(iface_tps[i].name!, nm.name) }
              _ => {}
            }
          }
        }

        // Override set: methods explicitly bound by the impl win over defaults.
        let mut override_set: Dict<Sym, bool> = Dict<Sym, bool>()
        for m in ib.ibm.children() {
          if !isnull(m.method_name) {
            override_set.set(m.method_name!, true)
          }
        }

        // Walk default-bodied methods on the iface. Pass 0 (extends) has
        // already flattened parent methods into child, so iface.im
        // contains everything.
        for m in iface.im.children() {
          if isnull(m.body) { continue }         // abstract — skip
          if isnull(m.name) { continue }
          if isnull(m.receiver_type) { continue }
          if override_set.has(m.name!) { continue }

          // Resolve the concrete receiver class via str_map.
          let recv_entry = str_map.get(m.receiver_type!)
          if isnull(recv_entry) { continue }
          let recv_class_name = recv_entry!.value

          let key_sym = sym(recv_class_name + "." + m.name!.name)
          if emitted.has(key_sym) {
            panic("desugar: default-method specialization collision on " +
                  recv_class_name + "." + m.name!.name +
                  " — two impls produce the same specialization. " +
                  "Disambiguate with an explicit alias binding in one impl.")
          }
          emitted.set(key_sym, true)

          // Deep-clone with empty string-map (no string substitution; rich
          // substitution below handles everything).
          let empty_map: Dict<Sym, string> = Dict<Sym, string>()
          let fn = deep_copy_func_decl(m, empty_map)

          // Rich substitution: param types, return type, body, where-clauses.
          for p in fn.param.children() {
            if !isnull(p.type_expr) {
              substitute_type_params_rich_in_type_expr(p.type_expr!, rich_map)
            }
          }
          if !isnull(fn.return_type) {
            substitute_type_params_rich_in_type_expr(fn.return_type!, rich_map)
          }
          if !isnull(fn.body) {
            substitute_type_params_rich_in_block(fn.body!, rich_map)
          }
          for wc in fn.wc.children() {
            for wa in wc.wc_arg.children() {
              substitute_type_params_rich_in_type_expr(wa, rich_map)
            }
          }

          // Receiver becomes the concrete class.
          fn.receiver_type = sym(recv_class_name)

          // Carry impl-level type-params (partial impls) onto the fn so the
          // monomorphizer still sees them as the remaining free vars.
          for tp in ib.imtp.children() {
            let new_tp = TypeParam {
              name: tp.name,
              constraint: tp.constraint,
              span: tp.span,
            }
            array_append<FuncDecl, TypeParam>(fn, new_tp)
          }

          array_append<LyricBlock, FuncDecl>(block, fn)
        }
      }
    }
  }

  // ---- 5. DesugarDefaultImpls ----
  // Extracts interface methods with bodies into top-level functions
  // with relational where clauses.

  func desugar_default_impls(file: File) {
    for block in file.fb.children() {
      for iface in block.id.children() {
        let methods = iface.im.children()
        let mut kept: [FuncDecl] = []
        for m in methods {
          if !isnull(m.body) {
            // Extract as top-level function
            let fn = m
            // Preserve receiver_type — the checker uses it to register methods
            // on concrete types that implement this interface via impl blocks.

            // Add interface type params
            let iface_tps = iface.itp.children()
            for tp in iface_tps {
              let new_tp = TypeParam {
                name: tp.name,
                constraint: tp.constraint,
                span: tp.span,
              }
              array_append<FuncDecl, TypeParam>(fn, new_tp)
            }

            // Add relational where clause
            let mut where_args: [TypeExpr] = []
            for tp in iface_tps {
              let arg = TypeExpr {
                kind: Named(tp.name!, []),
                span: tp.span,
              }
              where_args = append(where_args, arg)
            }
            let wc = WhereClause {
              variable: null,
              constraint: iface.name,
              span: iface.span,
            }
            for arg in where_args {
              array_append<WhereClause, TypeExpr>(wc, arg)
            }
            array_append<FuncDecl, WhereClause>(fn, wc)

            // Remove from interface, add to block
            array_append<LyricBlock, FuncDecl>(block, fn)
          } else {
            kept = append(kept, m)
          }
        }

        // Replace interface methods with kept-only list
        // Clear existing and re-add kept methods
        // NOTE: ArrayList doesn't have a bulk-replace API, so we remove all
        // and re-add the kept ones
        for m in methods {
          let mut found = false
          for k in kept {
            // Reference equality — both are class handles
            if m == k {
              found = true
            }
          }
          if !found {
            // Already moved to block.Functions via array_append above
            // Just need to remove from interface's method list
            array_remove<InterfaceDecl, FuncDecl>(m)
          }
        }
      }
    }
  }

  // ---- 2.5. DesugarFieldAccess ----
  // Rewrites field access on interface type params to getter/setter calls.
  // Inside interface method bodies, `self.children` becomes `self.children()`
  // and `self.children = x` becomes `self.set_children(x)`.

  func desugar_field_access(file: File) {
    for block in file.fb.children() {
      for iface in block.id.children() {
        // Collect field names per type param
        let field_names = Dict<Sym, bool>()
        for fd in iface.ifd.children() {
          if !isnull(fd.name) {
            field_names.set(sym(fd.name!.name), true)
          }
        }
        let has_fields = len(iface.ifd.children()) > 0
        if !has_fields { continue }

        // Rewrite field accesses in method bodies
        for m in iface.im.children() {
          if !isnull(m.body) {
            rewrite_field_access_block(m.body!, field_names)
          }
        }

        // Also rewrite in destructor bodies
        for db in iface.idb.children() {
          if !isnull(db.body) {
            rewrite_field_access_block(db.body!, field_names)
          }
        }
      }
    }
  }

  func rewrite_field_access_block(block: Block, field_names: Dict<Sym, bool>) {
    for s in block.bs.children() {
      rewrite_field_access_stmt(s, field_names)
    }
  }

  func rewrite_field_access_stmt(s: Stmt, field_names: Dict<Sym, bool>) {
    match s.kind {
      Assign(target, value) => {
        // Check for `self.field = value` -> `self.set_field(value)`
        match target.kind {
          FieldAccess(recv, fname) => {
            if field_names.get(sym(fname.name)) != null {
              let setter_name = sym("set_" + fname.name)
              let empty_ta: [TypeExpr] = []
              let empty_ma: [bool] = [false]
              s.kind = ExprStmt(Expr {
                kind: MethodCall(recv, setter_name, empty_ta, [value], empty_ma),
                span: s.span,
              })
              rewrite_field_access_expr(value, field_names)
              return
            }
          }
          _ => {}
        }
        rewrite_field_access_expr(target, field_names)
        rewrite_field_access_expr(value, field_names)
      }
      VarDecl(_, _, _, _, _, value) => {
        if !isnull(value) {
          rewrite_field_access_expr(value!, field_names)
        }
      }
      Return(value) => {
        if !isnull(value) {
          rewrite_field_access_expr(value!, field_names)
        }
      }
      ExprStmt(expr) => {
        rewrite_field_access_expr(expr, field_names)
      }
      If(cond, then_block, else_ifs, else_block) => {
        rewrite_field_access_expr(cond, field_names)
        rewrite_field_access_block(then_block, field_names)
        for ei in else_ifs {
          if !isnull(ei.condition) {
            rewrite_field_access_expr(ei.condition!, field_names)
          }
          if !isnull(ei.body) {
            rewrite_field_access_block(ei.body!, field_names)
          }
        }
        if !isnull(else_block) {
          rewrite_field_access_block(else_block!, field_names)
        }
      }
      While(cond, body) => {
        rewrite_field_access_expr(cond, field_names)
        rewrite_field_access_block(body, field_names)
      }
      For(_, _, iter, body) => {
        rewrite_field_access_expr(iter, field_names)
        rewrite_field_access_block(body, field_names)
      }
      BlockStmt(block) => {
        rewrite_field_access_block(block, field_names)
      }
      Match(expr, arms) => {
        rewrite_field_access_expr(expr, field_names)
        for arm in arms {
          if !isnull(arm.body) {
            rewrite_field_access_block(arm.body!, field_names)
          }
        }
      }
      _ => {}
    }
  }

  func rewrite_field_access_expr(e: Expr, field_names: Dict<Sym, bool>) {
    match e.kind {
      FieldAccess(recv, fname) => {
        if field_names.get(sym(fname.name)) != null {
          // Rewrite `self.field` -> `self.field()` (getter call)
          let empty_ta: [TypeExpr] = []
          let empty_args: [Expr] = []
          let empty_ma: [bool] = []
          e.kind = MethodCall(recv, fname, empty_ta, empty_args, empty_ma)
        }
        rewrite_field_access_expr(recv, field_names)
      }
      MethodCall(recv, _, _, args, _) => {
        rewrite_field_access_expr(recv, field_names)
        for arg in args {
          rewrite_field_access_expr(arg, field_names)
        }
      }
      Call(func_expr, _, args, _) => {
        rewrite_field_access_expr(func_expr, field_names)
        for arg in args {
          rewrite_field_access_expr(arg, field_names)
        }
      }
      Binary(left, _, right) => {
        rewrite_field_access_expr(left, field_names)
        rewrite_field_access_expr(right, field_names)
      }
      Unary(_, operand) => {
        rewrite_field_access_expr(operand, field_names)
      }
      Index(recv, idx) => {
        rewrite_field_access_expr(recv, field_names)
        rewrite_field_access_expr(idx, field_names)
      }
      IfElse(cond, then_block, else_ifs, else_block) => {
        rewrite_field_access_expr(cond, field_names)
        rewrite_field_access_block(then_block, field_names)
        for ei in else_ifs {
          if !isnull(ei.condition) {
            rewrite_field_access_expr(ei.condition!, field_names)
          }
          if !isnull(ei.body) {
            rewrite_field_access_block(ei.body!, field_names)
          }
        }
        rewrite_field_access_block(else_block, field_names)
      }
      StringInterp(parts) => {
        for part in parts {
          rewrite_field_access_expr(part, field_names)
        }
      }
      _ => {}
    }
  }

  // ---- Main entry point ----

  func desugar_all(file: File) {
    desugar_interface_extends(file)
    desugar_interface_fields(file)
    desugar_field_access(file)
    desugar_relations(file)
    desugar_destructors(file)
    desugar_specialize_default_impls(file)
    desugar_default_impls(file)
  }

  // ---- Helper functions ----

  // Extract the name Sym from a TypeExpr if it's a Named type
  func type_expr_name(te: TypeExpr) -> Sym? {
    match te.kind {
      Named(name, args) => { return name }
      _ => { return null }
    }
  }

  // Compare two optional Syms by name string
  func sym_eq(a: Sym?, b: Sym?) -> bool {
    if isnull(a) && isnull(b) { return true }
    if isnull(a) || isnull(b) { return false }
    return a!.name == b!.name
  }

  // Substitute a Sym through the type map
  func substitute_sym(s: Sym?, type_map: Dict<Sym, string>) -> Sym? {
    if isnull(s) { return null }
    let entry = type_map.get(sym(s!.name))
    if isnull(entry) { return s }
    return sym(entry!.value)
  }

  // Create a copy of a TypeExpr with type params substituted
  func substitute_type_expr_copy(te: TypeExpr?, type_map: Dict<Sym, string>) -> TypeExpr? {
    if isnull(te) { return null }
    let result = TypeExpr {
      kind: te!.kind,
      span: te!.span,
    }
    substitute_type_params_in_type_expr(result, type_map)
    return result
  }

  // Substitute type params in a TypeExpr in place
  func substitute_type_params_in_type_expr(te: TypeExpr, type_map: Dict<Sym, string>) {
    match te.kind {
      Named(name, args) => {
        let entry = type_map.get(sym(name.name))
        if !isnull(entry) {
          te.kind = Named(sym(entry!.value), args)
        }
        let mut i: i32 = 0
        while i < len(args) {
          // args[i] is a TypeExpr class — recurse
          substitute_type_params_in_type_expr(args[i], type_map)
          i = i + 1
        }
      }
      Optional(inner) => {
        substitute_type_params_in_type_expr(inner, type_map)
      }
      Sequence(elem) => {
        substitute_type_params_in_type_expr(elem, type_map)
      }
      _ => {}
    }
  }

  // Rewrite a field's type, replacing interface type params with concrete types from the relation
  // Rewrite a field's type, replacing iface type-param references with
  // the impl's per-side concrete TypeExpr (preserving generic args).
  // Indexes impl_args by name match against iface_tps. See redesign §3.9.
  func rewrite_field_type_impl(te: TypeExpr?, iface_tps: [TypeParam], impl_args: [ImplTypeArg]) -> TypeExpr? {
    if isnull(te) { return null }
    match te!.kind {
      Named(name, args) => {
        let mut slot: i32 = -1
        if len(iface_tps) >= 1 && !isnull(iface_tps[0].name) && name.name == iface_tps[0].name!.name {
          slot = 0
        } else if len(iface_tps) >= 2 && !isnull(iface_tps[1].name) && name.name == iface_tps[1].name!.name {
          slot = 1
        }
        if slot >= 0 && slot < len(impl_args) && !isnull(impl_args[slot].type_expr) {
          let src = impl_args[slot].type_expr!
          return TypeExpr { kind: src.kind, span: te!.span }
        }
        return te
      }
      Optional(inner) => {
        let new_inner = rewrite_field_type_impl(inner, iface_tps, impl_args)
        if isnull(new_inner) { return te }
        return TypeExpr {
          kind: TypeExprKind.Optional(new_inner!),
          span: te!.span,
        }
      }
      Sequence(elem) => {
        let new_elem = rewrite_field_type_impl(elem, iface_tps, impl_args)
        if isnull(new_elem) { return te }
        return TypeExpr {
          kind: TypeExprKind.Sequence(new_elem!),
          span: te!.span,
        }
      }
      _ => { return te }
    }
  }
  // ---- Deep copy ----

  func deep_copy_block(b: Block?) -> Block? {
    if isnull(b) { return null }
    let new_block = Block { span: b!.span }
    for stmt in b!.bs.children() {
      let new_stmt = deep_copy_stmt(stmt)
      array_append<Block, Stmt>(new_block, new_stmt)
    }
    return new_block
  }

  func deep_copy_stmt(s: Stmt) -> Stmt {
    let result = Stmt { kind: s.kind, span: s.span }
    match s.kind {
      ExprStmt(expr) => {
        result.kind = ExprStmt(deep_copy_expr(expr))
      }
      VarDecl(name, names, type_expr, is_mut, is_ref, value) => {
        let mut new_value: Expr? = null
        if !isnull(value) {
          new_value = deep_copy_expr(value!)
        }
        let mut new_te: TypeExpr? = null
        if !isnull(type_expr) {
          new_te = deep_copy_type_expr(type_expr!)
        }
        result.kind = VarDecl(name, names, new_te, is_mut, is_ref, new_value)
      }
      Assign(target, value) => {
        result.kind = Assign(deep_copy_expr(target), deep_copy_expr(value))
      }
      Return(value) => {
        let mut new_value: Expr? = null
        if !isnull(value) {
          new_value = deep_copy_expr(value!)
        }
        result.kind = Return(new_value)
      }
      If(condition, then_block, else_ifs, else_block) => {
        let new_then = deep_copy_block_val(then_block)
        let mut new_else_ifs: [ElseIf] = []
        for ei in else_ifs {
          let mut new_cond: Expr? = null
          if !isnull(ei.condition) {
            new_cond = deep_copy_expr(ei.condition!)
          }
          let new_ei = ElseIf {
            condition: new_cond,
            body: deep_copy_block(ei.body),
            span: ei.span,
          }
          new_else_ifs = append(new_else_ifs, new_ei)
        }
        let mut new_else: Block? = null
        if !isnull(else_block) {
          new_else = deep_copy_block_val(else_block!)
        }
        result.kind = If(deep_copy_expr(condition), new_then, new_else_ifs, new_else)
      }
      While(condition, body) => {
        result.kind = While(deep_copy_expr(condition), deep_copy_block_val(body))
      }
      For(var_name, index_var, collection, body) => {
        result.kind = For(var_name, index_var, deep_copy_expr(collection), deep_copy_block_val(body))
      }
      Match(value, arms) => {
        let mut new_arms: [MatchArm] = []
        for arm in arms {
          let mut new_guard: Expr? = null
          if !isnull(arm.guard) {
            new_guard = deep_copy_expr(arm.guard!)
          }
          let new_arm = MatchArm {
            pattern: arm.pattern,
            patterns: arm.patterns,
            guard: new_guard,
            body: deep_copy_block(arm.body),
            span: arm.span,
          }
          new_arms = append(new_arms, new_arm)
        }
        result.kind = StmtKind.Match(deep_copy_expr(value), new_arms)
      }
      BlockStmt(block) => {
        result.kind = BlockStmt(deep_copy_block_val(block))
      }
      IfLet(pattern, value, then_block, else_block) => {
        let mut new_else: Block? = null
        if !isnull(else_block) {
          new_else = deep_copy_block_val(else_block!)
        }
        result.kind = IfLet(pattern, deep_copy_expr(value), deep_copy_block_val(then_block), new_else)
      }
      LetElse(pattern, value, else_block) => {
        result.kind = LetElse(pattern, deep_copy_expr(value), deep_copy_block_val(else_block))
      }
      Spawn(body) => {
        result.kind = Spawn(deep_copy_block_val(body))
      }
      Lock(mutex, body) => {
        // Cannot reconstruct: Lock() collides with TypeKind.Lock (checker bug)
        // Shallow copy — Lock stmts don't appear in destructor bodies
      }
      Yield(value) => {
        let mut new_value: Expr? = null
        if !isnull(value) {
          new_value = deep_copy_expr(value!)
        }
        result.kind = Yield(new_value)
      }
      _ => {}  // Break, Continue, Select — no nested Exprs/Blocks to copy
    }
    return result
  }

  // Deep copy a non-optional Block (for use in enum fields that are Block not Block?)
  func deep_copy_block_val(b: Block) -> Block {
    let new_block = Block { span: b.span }
    for stmt in b.bs.children() {
      let new_stmt = deep_copy_stmt(stmt)
      array_append<Block, Stmt>(new_block, new_stmt)
    }
    return new_block
  }

  func deep_copy_expr(e: Expr) -> Expr {
    let result = Expr {
      kind: e.kind,
      span: e.span,
      resolved_type: e.resolved_type,
    }
    match e.kind {
      Call(func_expr, type_args, args, _) => {
        let new_ta = deep_copy_type_args(type_args)
        let new_args = deep_copy_expr_list(args)
        result.kind = Call(deep_copy_expr(func_expr), new_ta, new_args, [])
      }
      MethodCall(receiver, method, type_args, args, _) => {
        let new_ta = deep_copy_type_args(type_args)
        let new_args = deep_copy_expr_list(args)
        result.kind = MethodCall(deep_copy_expr(receiver), method, new_ta, new_args, [])
      }
      FieldAccess(receiver, field_name) => {
        result.kind = FieldAccess(deep_copy_expr(receiver), field_name)
      }
      Index(receiver, index) => {
        result.kind = Index(deep_copy_expr(receiver), deep_copy_expr(index))
      }
      Slice(receiver, low, high) => {
        let new_low: Expr? = if !isnull(low) { deep_copy_expr(low!) } else { null }
        let new_high: Expr? = if !isnull(high) { deep_copy_expr(high!) } else { null }
        result.kind = Slice(deep_copy_expr(receiver), new_low, new_high)
      }
      Unary(op, operand) => {
        result.kind = Unary(op, deep_copy_expr(operand))
      }
      Binary(left, op, right) => {
        result.kind = Binary(deep_copy_expr(left), op, deep_copy_expr(right))
      }
      TupleLit(elems) => {
        result.kind = TupleLit(deep_copy_expr_list(elems))
      }
      ListLit(elems) => {
        result.kind = ListLit(deep_copy_expr_list(elems))
      }
      MapLit(keys, values) => {
        result.kind = MapLit(deep_copy_expr_list(keys), deep_copy_expr_list(values))
      }
      StringInterp(parts) => {
        result.kind = StringInterp(deep_copy_expr_list(parts))
      }
      StructLit(type_name, ta, fields) => {
        let new_ta = deep_copy_type_args(ta)
        let mut new_fields: [StructLitField] = []
        for f in fields {
          let new_val: Expr? = if !isnull(f.value) { deep_copy_expr(f.value!) } else { null }
          let nf = StructLitField {
            name: f.name,
            value: new_val,
          }
          new_fields = append(new_fields, nf)
        }
        result.kind = StructLit(type_name, new_ta, new_fields)
      }
      Lambda(params, return_type, body) => {
        let new_rt: TypeExpr? = if !isnull(return_type) { deep_copy_type_expr(return_type!) } else { null }
        result.kind = Lambda(params, new_rt, deep_copy_block_val(body))
      }
      Match(value, arms) => {
        // Cannot reconstruct: Match() collides with StmtKind.Match (checker bug)
        // Shallow copy — match expressions in destructor bodies are rare
      }
      Cast(target_type, operand) => {
        result.kind = Cast(deep_copy_type_expr(target_type), deep_copy_expr(operand))
      }
      Unwrap(operand) => {
        result.kind = Unwrap(deep_copy_expr(operand))
      }
      Try(operand) => {
        result.kind = Try(deep_copy_expr(operand))
      }
      Is(operand, variant) => {
        result.kind = Is(deep_copy_expr(operand), variant)
      }
      IfElse(cond, then_block, else_ifs, else_block) => {
        let mut new_else_ifs: [ElseIf] = []
        for ei in else_ifs {
          let new_ei = ElseIf {
            condition: if !isnull(ei.condition) { deep_copy_expr(ei.condition!) } else { null },
            body: deep_copy_block(ei.body),
            span: ei.span,
          }
          new_else_ifs = append(new_else_ifs, new_ei)
        }
        result.kind = IfElse(deep_copy_expr(cond), deep_copy_block_val(then_block), new_else_ifs, deep_copy_block_val(else_block))
      }
      _ => {}  // Ident, IntLit, FloatLit, StringLit, BoolLit, Nil — no nested data
    }
    return result
  }

  func deep_copy_type_expr(te: TypeExpr) -> TypeExpr {
    // Shallow copy of TypeExpr is safe for desugar: substitute_type_params_in_type_expr
    // creates new enum values rather than mutating class fields. The critical deep copy
    // is for Expr/Stmt trees where destructor method renames cause contamination.
    return TypeExpr { kind: te.kind, span: te.span }
  }

  func deep_copy_type_args(args: [TypeExpr]) -> [TypeExpr] {
    let mut result: [TypeExpr] = []
    let mut i: i32 = 0
    while i < len(args) {
      result = append(result, TypeExpr { kind: args[i].kind, span: args[i].span })
      i = i + 1
    }
    return result
  }

  func deep_copy_expr_list(exprs: [Expr]) -> [Expr] {
    let mut result: [Expr] = []
    let mut i: i32 = 0
    while i < len(exprs) {
      result = append(result, deep_copy_expr(exprs[i]))
      i = i + 1
    }
    return result
  }

  // ---- Type param substitution in blocks/stmts/exprs ----

  func substitute_type_params_in_block(block: Block, type_map: Dict<Sym, string>) {
    for stmt in block.bs.children() {
      substitute_type_params_in_stmt(stmt, type_map)
    }
  }

  func substitute_type_params_in_stmt(stmt: Stmt, type_map: Dict<Sym, string>) {
    match stmt.kind {
      ExprStmt(expr) => {
        substitute_type_params_in_expr(expr, type_map)
      }
      Assign(target, value) => {
        substitute_type_params_in_expr(value, type_map)
      }
      VarDecl(name, names, type_expr, is_mut, is_ref, value) => {
        if !isnull(value) {
          substitute_type_params_in_expr(value!, type_map)
        }
      }
      Return(value) => {
        if !isnull(value) {
          substitute_type_params_in_expr(value!, type_map)
        }
      }
      If(condition, then_block, else_ifs, else_block) => {
        substitute_type_params_in_expr(condition, type_map)
        substitute_type_params_in_block(then_block, type_map)
        for ei in else_ifs {
          if !isnull(ei.condition) {
            substitute_type_params_in_expr(ei.condition!, type_map)
          }
          if !isnull(ei.body) {
            substitute_type_params_in_block(ei.body!, type_map)
          }
        }
        if !isnull(else_block) {
          substitute_type_params_in_block(else_block!, type_map)
        }
      }
      While(condition, body) => {
        substitute_type_params_in_expr(condition, type_map)
        substitute_type_params_in_block(body, type_map)
      }
      For(var_name, index_var, collection, body) => {
        substitute_type_params_in_expr(collection, type_map)
        substitute_type_params_in_block(body, type_map)
      }
      BlockStmt(block) => {
        substitute_type_params_in_block(block, type_map)
      }
      _ => {}
    }
  }

  func substitute_type_params_in_expr(expr: Expr, type_map: Dict<Sym, string>) {
    match expr.kind {
      Call(func_expr, type_args, args, _) => {
        let mut i: i32 = 0
        while i < len(type_args) {
          substitute_type_params_in_type_expr(type_args[i], type_map)
          i = i + 1
        }
        substitute_type_params_in_expr(func_expr, type_map)
        i = 0
        while i < len(args) {
          substitute_type_params_in_expr(args[i], type_map)
          i = i + 1
        }
      }
      MethodCall(receiver, method, type_args, args, _) => {
        let mut i: i32 = 0
        while i < len(type_args) {
          substitute_type_params_in_type_expr(type_args[i], type_map)
          i = i + 1
        }
        substitute_type_params_in_expr(receiver, type_map)
        i = 0
        while i < len(args) {
          substitute_type_params_in_expr(args[i], type_map)
          i = i + 1
        }
      }
      Unary(op, operand) => {
        substitute_type_params_in_expr(operand, type_map)
      }
      Binary(left, op, right) => {
        substitute_type_params_in_expr(left, type_map)
        substitute_type_params_in_expr(right, type_map)
      }
      FieldAccess(receiver, field_name) => {
        substitute_type_params_in_expr(receiver, type_map)
      }
      Index(receiver, index) => {
        substitute_type_params_in_expr(receiver, type_map)
        substitute_type_params_in_expr(index, type_map)
      }
      _ => {}
    }
  }

  // === Rich type parameter substitution (map[string]TypeExpr) ===
  // Used by DesugarDestructors: when replacing type param P with Dict<Sym, V>,
  // === Rich type parameter substitution (map[string]TypeExpr) ===
  // Used by DesugarDestructors: when replacing type param P with Dict<Sym, V>,
  // the replacement carries Args so generic instantiations are preserved.

  func substitute_type_params_rich_in_block(block: Block, type_map: Dict<Sym, TypeExpr>) {
    for stmt in block.bs.children() {
      substitute_type_params_rich_in_stmt(stmt, type_map)
    }
  }

  func substitute_type_params_rich_in_stmt(stmt: Stmt, type_map: Dict<Sym, TypeExpr>) {
    match stmt.kind {
      ExprStmt(expr) => {
        substitute_type_params_rich_in_expr(expr, type_map)
      }
      Assign(target, value) => {
        substitute_type_params_rich_in_expr(target, type_map)
        substitute_type_params_rich_in_expr(value, type_map)
      }
      VarDecl(name, names, type_expr, is_mut, is_ref, value) => {
        if !isnull(value) { substitute_type_params_rich_in_expr(value!, type_map) }
      }
      Return(value) => {
        if !isnull(value) { substitute_type_params_rich_in_expr(value!, type_map) }
      }
      If(condition, then_block, else_ifs, else_block) => {
        substitute_type_params_rich_in_expr(condition, type_map)
        substitute_type_params_rich_in_block(then_block, type_map)
        for ei in else_ifs {
          if !isnull(ei.condition) {
            substitute_type_params_rich_in_expr(ei.condition!, type_map)
          }
          if !isnull(ei.body) {
            substitute_type_params_rich_in_block(ei.body!, type_map)
          }
        }
        if !isnull(else_block) { substitute_type_params_rich_in_block(else_block!, type_map) }
      }
      While(condition, body) => {
        substitute_type_params_rich_in_expr(condition, type_map)
        substitute_type_params_rich_in_block(body, type_map)
      }
      For(var_name, index_var, collection, body) => {
        substitute_type_params_rich_in_expr(collection, type_map)
        substitute_type_params_rich_in_block(body, type_map)
      }
      Match(value, arms) => {
        substitute_type_params_rich_in_expr(value, type_map)
        for arm in arms {
          if !isnull(arm.body) { substitute_type_params_rich_in_block(arm.body!, type_map) }
        }
      }
      BlockStmt(block) => {
        substitute_type_params_rich_in_block(block, type_map)
      }
      _ => {}
    }
  }

  func substitute_type_params_rich_in_expr(expr: Expr, type_map: Dict<Sym, TypeExpr>) {
    match expr.kind {
      Call(func_expr, type_args, args, _) => {
        let mut i: i32 = 0
        while i < len(type_args) {
          substitute_type_params_rich_in_type_expr(type_args[i], type_map)
          i = i + 1
        }
        substitute_type_params_rich_in_expr(func_expr, type_map)
        i = 0
        while i < len(args) {
          substitute_type_params_rich_in_expr(args[i], type_map)
          i = i + 1
        }
      }
      MethodCall(receiver, method, type_args, args, _) => {
        let mut i: i32 = 0
        while i < len(type_args) {
          substitute_type_params_rich_in_type_expr(type_args[i], type_map)
          i = i + 1
        }
        substitute_type_params_rich_in_expr(receiver, type_map)
        i = 0
        while i < len(args) {
          substitute_type_params_rich_in_expr(args[i], type_map)
          i = i + 1
        }
      }
      Unary(op, operand) => {
        substitute_type_params_rich_in_expr(operand, type_map)
      }
      Binary(left, op, right) => {
        substitute_type_params_rich_in_expr(left, type_map)
        substitute_type_params_rich_in_expr(right, type_map)
      }
      FieldAccess(receiver, field_name) => {
        substitute_type_params_rich_in_expr(receiver, type_map)
      }
      Index(receiver, index) => {
        substitute_type_params_rich_in_expr(receiver, type_map)
        substitute_type_params_rich_in_expr(index, type_map)
      }
      _ => {}
    }
  }

  func substitute_type_params_rich_in_type_expr(te: TypeExpr, type_map: Dict<Sym, TypeExpr>) {
    match te.kind {
      Named(name, args) => {
        let entry = type_map.get(sym(name.name))
        if !isnull(entry) {
          // Replace entirely with the rich TypeExpr (which carries Args)
          let replacement = entry!.value
          te.kind = replacement.kind
          te.span = replacement.span
          return
        }
        let mut i: i32 = 0
        while i < len(args) {
          substitute_type_params_rich_in_type_expr(args[i], type_map)
          i = i + 1
        }
      }
      Optional(inner) => {
        substitute_type_params_rich_in_type_expr(inner, type_map)
      }
      Sequence(elem) => {
        substitute_type_params_rich_in_type_expr(elem, type_map)
      }
      _ => {}
    }
  }

  // === Method call renaming ===
  // Used by DesugarDestructors to rewrite generic interface method names
  // (e.g. "children") to label-prefixed concrete names (e.g. "fb_children").

  func rename_method_calls_in_block(block: Block, renames: Dict<Sym, string>) {
    for stmt in block.bs.children() {
      rename_method_calls_in_stmt(stmt, renames)
    }
  }

  func rename_method_calls_in_stmt(stmt: Stmt, renames: Dict<Sym, string>) {
    match stmt.kind {
      ExprStmt(expr) => {
        rename_method_calls_in_expr(expr, renames)
      }
      Assign(target, value) => {
        rename_method_calls_in_expr(target, renames)
        rename_method_calls_in_expr(value, renames)
      }
      VarDecl(name, names, type_expr, is_mut, is_ref, value) => {
        if !isnull(value) { rename_method_calls_in_expr(value!, renames) }
      }
      If(condition, then_block, else_ifs, else_block) => {
        rename_method_calls_in_expr(condition, renames)
        rename_method_calls_in_block(then_block, renames)
        for ei in else_ifs {
          if !isnull(ei.condition) {
            rename_method_calls_in_expr(ei.condition!, renames)
          }
          if !isnull(ei.body) {
            rename_method_calls_in_block(ei.body!, renames)
          }
        }
        if !isnull(else_block) { rename_method_calls_in_block(else_block!, renames) }
      }
      While(condition, body) => {
        rename_method_calls_in_expr(condition, renames)
        rename_method_calls_in_block(body, renames)
      }
      For(var_name, index_var, collection, body) => {
        rename_method_calls_in_expr(collection, renames)
        rename_method_calls_in_block(body, renames)
      }
      Match(value, arms) => {
        rename_method_calls_in_expr(value, renames)
        for arm in arms {
          if !isnull(arm.body) { rename_method_calls_in_block(arm.body!, renames) }
        }
      }
      Return(value) => {
        if !isnull(value) { rename_method_calls_in_expr(value!, renames) }
      }
      BlockStmt(block) => {
        rename_method_calls_in_block(block, renames)
      }
      _ => {}
    }
  }

  func rename_method_calls_in_expr(expr: Expr, renames: Dict<Sym, string>) {
    match expr.kind {
      MethodCall(receiver, method, type_args, args, _) => {
        let entry = renames.get(sym(method.name))
        if !isnull(entry) {
          expr.kind = MethodCall(receiver, sym(entry!.value), type_args, args, [])
        }
        rename_method_calls_in_expr(receiver, renames)
        let mut i: i32 = 0
        while i < len(args) {
          rename_method_calls_in_expr(args[i], renames)
          i = i + 1
        }
      }
      Call(func_expr, type_args, args, _) => {
        rename_method_calls_in_expr(func_expr, renames)
        let mut i: i32 = 0
        while i < len(args) {
          rename_method_calls_in_expr(args[i], renames)
          i = i + 1
        }
      }
      Binary(left, op, right) => {
        rename_method_calls_in_expr(left, renames)
        rename_method_calls_in_expr(right, renames)
      }
      Unary(op, operand) => {
        rename_method_calls_in_expr(operand, renames)
      }
      FieldAccess(receiver, field_name) => {
        rename_method_calls_in_expr(receiver, renames)
      }
      Index(receiver, index) => {
        rename_method_calls_in_expr(receiver, renames)
        rename_method_calls_in_expr(index, renames)
      }
      Unwrap(operand) => {
        rename_method_calls_in_expr(operand, renames)
      }
      _ => {}
    }
  }

}

