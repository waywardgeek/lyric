// Test: Global Dict freed by RC between function calls
// Reproduces pattern from checker.ly where _method_aliases Dict
// gets RC'd to 0 between checker and lowerer phases.

let mut _aliases: Dict<Sym, string>? = null

func get_aliases() -> Dict<Sym, string> {
  if isnull(_aliases) {
    _aliases = Dict<Sym, string>()
  }
  return _aliases!
}

// Phase 1: populate the global dict
func phase1() {
  get_aliases().set(`alpha`, "a_base")
  get_aliases().set(`bravo`, "b_base")
  // Verify immediately
  assert_eq(get_aliases().length(), 2)
}

// Allocate many Dicts to trigger slab reuse of freed memory
func churn_dicts() {
  let d1 = Dict<Sym, string>()
  d1.set(`x1`, "noise1")
  d1.set(`x2`, "noise2")
  d1.set(`x3`, "noise3")
  let d2 = Dict<Sym, string>()
  d2.set(`y1`, "noise4")
  d2.set(`y2`, "noise5")
  let d3 = Dict<Sym, string>()
  d3.set(`z1`, "noise6")
  d3.set(`z2`, "noise7")
  d3.set(`z3`, "noise8")
  d3.set(`z4`, "noise9")
  // All these Dicts freed at scope exit
}

// Phase 2: check global dict survived
func phase2() {
  let aliases = get_aliases()
  let count = aliases.length()
  println(f"phase2: aliases count={count}")
  if count != 2 {
    println("BUG: global Dict was corrupted by slab reuse!")
    println(f"  Expected 2 entries, got {count}")
    // Try to read our entries
    let a = aliases.get(`alpha`)
    if !isnull(a) {
      println(f"  alpha = {a!}")
    } else {
      println("  alpha = MISSING")
    }
  }
  assert_eq(count, 2)
  assert_eq(aliases.get(`alpha`)!, "a_base")
  assert_eq(aliases.get(`bravo`)!, "b_base")
}

func test_global_dict_survives_churn() {
  phase1()
  churn_dicts()
  phase2()
}

// Variant: use the dict as a temporary (no local binding)
func test_global_dict_temporary_use() {
  // Access via temporary — RC inc on return, dec at statement end
  get_aliases().set(`charlie`, "c_base")
  get_aliases().set(`delta`, "d_base")

  // Churn
  churn_dicts()

  // Verify
  assert_eq(get_aliases().length(), 4)
  assert_eq(get_aliases().get(`charlie`)!, "c_base")
}
