// Test: Global Dict freed by RC between function calls
// Reproduces bug where a global optional Dict handle gets RC'd to 0
// and slab reuses the memory.
//
// KNOWN FAILURE: This test demonstrates the Dict corruption bug documented
// in TODO.md. The global _aliases Dict gets freed by RC between phase1()
// and phase2() calls, and slab reuses the memory for churn_dicts() allocations.

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
func test_global_dict_survives_churn() {
  phase1()

  // Verify immediately after phase1 — should be fine
  let count_before = get_aliases().len()
  assert(count_before == 2)

  churn_dicts()

  // After churn, the global Dict may have been freed and its slab reused
  let aliases = get_aliases()
  let count = aliases.len()
  assert(count == 2)
  let a = aliases.get(`alpha`)
  assert(!isnull(a))
  assert(a!.value == "a_base")
  let b = aliases.get(`bravo`)
  assert(!isnull(b))
  assert(b!.value == "b_base")
}
