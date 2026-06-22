// test_dict_len.ly — regression test for Dict.len() (renamed from Dict.length).
// Verifies the method exists, returns i32, and reflects insertions/removals.

func test_dict_len_empty() {
  let d = Dict<Sym, i32>()
  assert_eq(d.len(), 0, "empty dict has len 0")
}

func test_dict_len_after_inserts() {
  let d = Dict<Sym, i32>()
  d.set(`alpha`, 1)
  d.set(`bravo`, 2)
  d.set(`gamma`, 3)
  assert_eq(d.len(), 3, "len after 3 inserts")
}

func test_dict_len_set_same_key_no_growth() {
  let d = Dict<Sym, i32>()
  d.set(`x`, 1)
  d.set(`x`, 2)
  d.set(`x`, 3)
  assert_eq(d.len(), 1, "overwriting same key keeps len at 1")
}

func test_dict_len_after_remove() {
  let d = Dict<Sym, i32>()
  d.set(`a`, 1)
  d.set(`b`, 2)
  d.set(`c`, 3)
  assert_eq(d.len(), 3, "len 3 before remove")
  let removed = d.remove(`b`)
  assert(removed)
  assert_eq(d.len(), 2, "len 2 after one remove")
  let removed_missing = d.remove(`zzz`)
  assert(!removed_missing)
  assert_eq(d.len(), 2, "removing missing key doesn't change len")
}
