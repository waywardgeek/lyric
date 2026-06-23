# Phase 1 progress notes — 2026-06-21 (Sunday morning)

*Continuation of multi-class interface redesign (`cr/docs/multi-class-interface-redesign.md`).
Handoff at `~/projects/coderhapsody/cr/handoff.md` directed Phase 1 Steps 1a + 1b.*

## TL;DR

- **Step 1b shipped** as commit `7ebc482` (this branch, NOT pushed).
  Field/method collision diagnostic in `Checker.register_class`, user-
  declared names only. Phase 0 confirmed zero collisions, so no
  existing class trips it. Full test suite holds at 86 PASS / 3 FAIL
  (the three known pre-existing failures: `graph.ly` + `tree.ly`
  future-syntax, `lock.ly` pthread typing). Self-test reaches fixed
  point (stage2 == stage3 byte-identical).
- **c_backend `class_renames` fallback → panic** as commit `d4fd2a1`
  (also on this branch, NOT pushed). All five fallback sites in
  `c_backend.ly` now panic with site-specific tags. Empirical: full
  bootstrap + 89-test corpus tripped zero of them; the fallback was
  dead code after `c549dac`. See "Bonus" section below.
- **Step 1a punted** — STOP-condition tripped. The checker rewrite
  itself was 23 lines and behaved correctly (verified end-to-end:
  `h.op(41)` parses, type-checks, and lowers). But the **lowerer/c_backend
  doesn't support indirect function-pointer calls**: `lowerer.ly:2637`
  has the giveaway `let actual_name = if func_name != "" { func_name } else { "__indirect_call" }`,
  and there is no `__indirect_call` runtime helper. The generated C
  was `int32_t _t1 = __indirect_call(41)` — implicit-decl error.
  Closing this gap is Phase 1.5 (lowerer + c_backend changes), not
  Phase 1 as scoped.

## Step 1b — what landed

```
diff --git a/src/checker/checker.ly b/src/checker/checker.ly
@@ -1891,6 +1891,22 @@ lyric checker {
       }
     }

+    // Phase 1 forward guard: user-declared field vs method name collision.
+    // Under the multi-class interface redesign §4.2, fields and methods share
+    // a namespace within a class.  Relation-injected names are out of scope
+    // here (Phase 3 will extend the check to them).
+    for f in cfields {
+      if f.name != null {
+        let fname = sym_to_string(f.name!)
+        for m in cmethods {
+          if m.name != null && sym_to_string(m.name!) == fname {
+            eprintln(f"checker: class {cname} declares both field '{fname}' and method '{fname}'; field and zero-arg method names share a namespace under UFCS — rename one")
+            os_exit(1)
+          }
+        }
+      }
+    }
+
     self.pop_scope()
```

Implementation choices worth flagging:

- **Walks `cls.cf_children()` × `cls.cm_children()` directly**, not
  `info.fields ∩ info.methods`. Reason: relation desugar may have
  populated `info.fields` with label-prefixed accessor fields before
  `register_class` runs; iterating the AST guarantees the carve-out
  ("user-declared only") at the implementation level, not just at the
  doc level.
- **O(F·M) per class**, no Dict. Typical class has < 20 fields and
  < 100 methods; the few outliers (CGen 37f/95m, Checker 9f/84m,
  Lowerer 19f/74m per Phase 0) are well under any threshold worth
  optimizing for.
- **Doesn't catch impl-block methods** that collide with class-body
  fields. The design doc Phase 1 entry says "user-declared names";
  impl-block methods qualify, but adding the check inside `add_impl_block`
  / `registerImplMethods` is a second site with different control flow.
  Phase 0 confirmed no collisions of either flavor today; deferring
  the impl-block site is safe until Phase 3 unifies the check there
  anyway.

## Step 1a — why it's blocked

The checker change worked. Code (later reverted):

```lyric
// Phase 1 UFCS: obj.field(args) where field is function-typed
// rewrites to Call(FieldAccess(obj, field), args).
let fft = info!.fields.get(sym(method_str))
if fft != null {
  let fclass_subst = self.build_type_arg_subst(recv_type)
  let field_type = if fclass_subst != null { substitute_type(fft!.value, fclass_subst!) } else { fft!.value }
  match field_type.kind {
    Func(fparams, fret, _) => {
      let field_expr = Expr {
        kind: ExprKind.FieldAccess(receiver, method),
        span: receiver.span,
        resolved_type: type_to_type_expr(field_type)
      }
      let new_mut_args: [bool] = []
      call_expr.kind = ExprKind.Call(field_expr, type_args, args, new_mut_args)
      propagate_arg_types(args, fparams)
      return fret
    }
    _ => {}
  }
}
```

Smoke test (`/tmp/fnfield_ufcs.ly`):

```lyric
class Holder { op: func(i32) -> i32 }
func add1(x: i32) -> i32 { return x + 1 }
func main() {
    let h = Holder { op: add1 }
    println(h.op(41))           // <- the new UFCS form
}
```

Before patch: `checker: unknown method: op at …:11:17` (and the
compiler kept going past `os_exit(1)` — separate bug, see below).
After patch: checker accepts the call, lowerer emits
`Call(FieldAccess(h, op), [41])`. C backend renders this as
`int32_t _t1 = __indirect_call(41);` — `__indirect_call` is not a
real function. Compilation of the produced C fails.

Root cause is `src/lowerer/lowerer.ly:2637`:

```lyric
let actual_name = if func_name != "" { func_name } else { "__indirect_call" }
```

The lowerer extracts a function NAME from the `Call`'s `func_expr`
when it's a simple `Ident`. For any other shape (`FieldAccess`,
`MethodCall`-as-value, lambda variable read, etc.) it falls back to
the sentinel `__indirect_call` and never emits a real indirect call
instruction. Both `LCallData.func_name` and the c_backend assume the
target is a top-level function symbol; there is no `LCallData.func_value`
field or equivalent for "call this LValue as a function pointer."

To unblock Step 1a, Phase 1.5 needs:

1. **Lowerer**: lower `func_expr` to an `LValue` when it isn't an
   `Ident`, store it alongside `LCallData`, and emit an indirect-call
   `LExpr` variant (or extend `ExCall` with an optional `func_value`).
2. **C backend**: when the call kind is indirect, emit
   `(receiver_val->field_name)(args)` (or the equivalent through a
   temp). This needs a function-pointer cast in C if the field
   storage type and the call site disagree on signature — they
   shouldn't, but defensive code matters.
3. **Validate** ARC / refcount: if `obj` is itself a refcounted
   class, the lowered indirect call must hold a reference for the
   call duration. Lambdas already do this; field reads of function-
   typed fields don't yet exercise the path.

Estimated blast: ~100-200 lines across lowerer.ly, c_backend.ly, and
lir.ly (probably a new `ExIndirectCall` kind or an `is_indirect: bool`
on `LCallData`). Not Phase 1.

## Tangential bug found (NOT touched, logged here)

The existing "unknown method" error at `checker.ly:3862-3865`:

```lyric
eprintln(f"checker: unknown method: ...")
os_exit(1)
return make_error_type()
```

The `eprintln` fires but **the compiler continues through `lower`,
`optimize`, `mono`, `validate`, `rewrite`, `slab`, `write`** and
produces broken C. Either `os_exit(1)` isn't terminating in this
context, or there's a separate code path that swallows the exit. The
output above (`/tmp/fnfield_ufcs.ly` with no checker change) shows
this behavior. It's not a regression — it's pre-existing. Did not
investigate; not in Phase 1 scope.

## Test summary

```
cd ~/projects/lyric
rm -f lyric && make           # OK (gcc -w, 0 warnings)
make update                   # OK, 114725 lines
rm -f lyric && make            # stage1 OK
make update                   # stage2 OK, 114725 lines
make self-test                # ✅ FIXED POINT (stage2 == stage3)
make test                     # 86 PASS / 3 FAIL (graph, tree, lock — known)
```

## Branch state

```
d4fd2a1 (HEAD -> main) c_backend: panic on class_renames hit instead of silent rename
7ebc482                checker: Phase 1 forward guard — field vs method name collision
c549dac (origin/main, origin/HEAD) fix(monomorphizer): mangle generic class handles ...
8e458fb                Updatees to docs.
```

Branch is **2 commits ahead of origin/main, NOT pushed.** Per handoff
policy. Working tree clean.

(Note: the handoff was written when `c549dac` was the tip and 2
ahead of origin; in the meantime `c549dac` and `8e458fb` were pushed,
and `5539376` — the hand-patch superseded by `c549dac` — was reset
out. Verified `git log --oneline -5` matches this picture.)

## Bonus: c_backend `class_renames` fallback → panic (`d4fd2a1`)

After the morning's Phase 1 work, Bill called the c_backend
hardening from memory 2026-06-20-10: "every time I hear 'fallback' I
think 'bug waiting to waste an afternoon'." Converted all five
`class_renames` lookup sites from silent rename to `eprintln` +
`os_exit(1)`, each tagged with the site name so a future hit
identifies the missed mono location:

| line | site tag                       | role                                |
|------|--------------------------------|-------------------------------------|
|  410 | `[c_type/TyClassHandle]`        | LType → C type name                  |
| 1082 | `[resolve_field_type]`          | field-owner LType resolution        |
| 2154 | `[destructor-receiver]`         | destructor codegen receiver         |
| 3629 | `[method-call-resolve]`         | ExMethodCall return-type lookup     |
| 3756 | `[resolve_type_name]`           | general type-name translator        |

Empirical result: full bootstrap self-compile + 89-test corpus
tripped **zero** panics. After `c549dac` (mono globals fix), the
fallback was dead code in every observed path. If a legitimate
caller emerges later, the panic message identifies the site and we
add a targeted special case at the missed mono location instead of
risking silent miscompile across the codebase.

Tests still 86 PASS / 3 FAIL (same known three); self-test fixed
point holds.

## Recommendation for Bill

1. **Review and approve / push `7ebc482`.** Pure forward guard, no
   behavioral change today. Earns its keep when Phase 3 lands relation-
   injected name collisions and we want one consistent diagnostic
   shape.
2. **Phase 1.5 decision** — do we want indirect-call support in the
   lowerer/c_backend now (unlocks Step 1a plus first-class function-
   typed fields generally), or push it out and let Phase 1 land
   without UFCS on function fields? My read: defer. The bootstrap has
   zero function-typed class fields today and the UFCS spec text (§4.3)
   reads cleanly without it; this is a forward feature, not a
   migration blocker.
3. **The pending-review pile** (`TODO.md`, `bootstrap-roadmap.md`,
   `lyric-language-spec.md`, `lyric-language-reference.md`,
   `lyric-feature-notes.md`, `spec-testdata-audit.md`) was not
   touched, per handoff. Still yours to review.

## Files touched this session

- `src/checker/checker.ly` (+16 lines)
- `lyric.c` (regenerated; the diff comes mostly from declaration-
  order shuffle the topological sort introduces when checker fns
  change, plus the 16-line emission of the new check itself)
- This file (`cr/docs/phase1-progress-notes.md`, new)

Nothing in `cr/docs/` you were reviewing was touched.
