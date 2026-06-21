lyric desugar_tests {
  func td_parse(src: string) -> File {
    let (file, err) = parse_file(src, "test.ly")
    assert(err == null, "parse error")
    assert(file != null, "file is null")
    return file!
  }

  // ---- Pass 2: InterfaceFields ----

  func test_field_generates_getter_and_setter() {
    let src = """lyric t { interface Named<T> { field T.name: string } }"""
    let file = td_parse(src)
    desugar_interface_fields(file)
    let named = file.fb_children()[0].id_children()[0]
    // Should have at least 2 methods: getter + setter
    assert(len(named.im_children()) >= 2, "expected getter + setter")
    let getter = named.im_children()[0]
    let setter = named.im_children()[1]
    assert(getter.name!.name == "name", "getter name")
    assert(setter.name!.name == "set_name", "setter name")
  }

  func test_field_generates_multiple_fields() {
    let src = """lyric t {
      interface HasInfo<T> {
        field T.name: string
        field T.age: i32
      }
    }"""
    let file = td_parse(src)
    desugar_interface_fields(file)
    let iface = file.fb_children()[0].id_children()[0]
    // 2 fields -> 4 methods (2 getters + 2 setters)
    assert(len(iface.im_children()) >= 4, "expected 4 methods for 2 fields")
  }

  // (Pass 1 InterfaceEmbeds removed — embed keyword deleted.)


  func test_default_impl_extracted() {
    let src = """lyric t {
      interface Printable<T> {
        func T.describe(self) -> string { return "default" }
      }
    }"""
    let file = td_parse(src)
    desugar_default_impls(file)
    // Default impl should be extracted to top-level function
    let block = file.fb_children()[0]
    assert(len(block.fd_children()) >= 1, "default impl extracted to top-level func")
  }

  func test_abstract_method_preserved() {
    let src = """lyric t {
      interface Printable<T> {
        func T.describe(self) -> string
        func T.show(self) -> string { return "default" }
      }
    }"""
    let file = td_parse(src)
    desugar_default_impls(file)
    let iface = file.fb_children()[0].id_children()[0]
    // Abstract method (describe) should remain on the interface
    assert(len(iface.im_children()) >= 1, "abstract methods preserved")
  }

  // ---- Pass ordering: desugar_all ----

  func test_desugar_all_runs_all_passes() {
    let src = """lyric t {
      interface Named<T> {
        field T.name: string
      }
    }"""
    let file = td_parse(src)
    desugar_all(file)
    let iface = file.fb_children()[0].id_children()[0]
    // After desugar_all, fields should be converted to getter/setter
    assert(len(iface.im_children()) >= 2, "desugar_all should run interface_fields")
  }

  // ---- Helper function tests ----

  func test_sym_eq_both_null() {
    assert(sym_eq(null, null), "null == null")
  }

  func test_sym_eq_one_null() {
    let s = sym("test")
    assert(!sym_eq(s, null), "sym != null")
    assert(!sym_eq(null, s), "null != sym")
  }

  func test_sym_eq_same() {
    let a = sym("hello")
    let b = sym("hello")
    assert(sym_eq(a, b), "same name should be equal")
  }

  func test_sym_eq_different() {
    let a = sym("hello")
    let b = sym("world")
    assert(!sym_eq(a, b), "different names should not be equal")
  }
}
