// method_generator.ly — Regression test for class-method generators
// (func T.method(self, ...) -> gen X { yield ... }).
//
// Pre-existing bug discovered during Phase 4 Wave 1: the c_backend's
// generator call-site rewrite and gen-base-name resolution handled
// only ExCall (free fns), not ExMethodCall (class methods). A method
// generator like `f.iota(5)` lowered to `Foo_iota(f, 5)` (missing the
// `_init` suffix) and the for-loop iter site fell back to
// `unknown_gen_gen_t* / unknown_gen_next()`, producing C that didn't
// compile and silently masking the failure as a "void* /* generator */"
// dance. Free-function generators (range, iota) worked because they
// went through a different emit_call_expr path.
//
// Fixes:
//   1. gen_funcs registration keyed by mangled name (<recv>_<name>) for
//      methods, matching func_name(f) emission.
//   2. emit_method_call_expr appends `_init` when the method is a
//      generator, parallel to emit_call_expr.
//   3. extract_func_name_from_expr handles ExMethodCall by deriving
//      <class>_<method> from the receiver's resolved class.
//   4. resolve_gen_base_name PANICS on unresolved gen calls instead of
//      returning the "unknown_gen" sentinel. (See system-wide de-jank
//      rule: fallbacks are bugs in disguise.)

lyric method_generator {

  class Box { capacity: i32 }
  class Node { value: i32 }

  // Generator yielding an integer, no self capture in body.
  pub func Box.count(self, n: i32) -> gen i32 {
    let mut i: i32 = 0
    while i < n {
      yield i
      i = i + 1
    }
  }

  // Generator yielding a class handle (the shape DoublyLinked.children
  // and similar stdlib iteration APIs will use).
  pub func Box.three_nodes(self) -> gen Node {
    let mut i: i32 = 0
    while i < self.capacity {
      yield Node { value: i * 10 }
      i = i + 1
    }
  }

  // Generator reading self in the body (exercises the _gen frame
  // capturing self for resumption).
  pub func Box.range_from(self, n: i32) -> gen i32 {
    let mut i: i32 = 0
    while i < n {
      yield self.capacity + i
      i = i + 1
    }
  }

  pub func test_method_generator_basic() {
    let b = Box { capacity: 3 }
    let mut s: i32 = 0
    for x in b.count(5) {
      s = s + x
    }
    assert_eq(s, 10, "0+1+2+3+4")
  }

  pub func test_method_generator_class_yield() {
    let b = Box { capacity: 3 }
    let mut sum: i32 = 0
    for nd in b.three_nodes() {
      sum = sum + nd.value
    }
    assert_eq(sum, 30, "Node{0} + Node{10} + Node{20}")
  }

  pub func test_method_generator_self_capture() {
    let b = Box { capacity: 3 }
    let mut s: i32 = 0
    for x in b.range_from(4) {
      s = s + x
    }
    // (3+0) + (3+1) + (3+2) + (3+3) = 18
    assert_eq(s, 18, "self.capacity captured across yields")
  }
}
