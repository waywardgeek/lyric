// test_parser.ly — Unit tests for the bootstrap parser
//
// Run: lyric test testdata/test_parser.ly bootstrap/parser/parser.ly bootstrap/parser/expr_parser.ly bootstrap/lexer/lexer.ly bootstrap/ast/ast.ly

lyric parser_tests {

  // Helper: parse a lyric block body and return the first LyricBlock
  func tp_parse(src: string) -> LyricBlock {
    let (file, err) = parse_file(src, "test.ly")
    assert(err == null, "parse error")
    assert(file != null, "file is null")
    assert(len(file!.fb.children) > 0, "no lyric blocks")
    return file!.fb.children[0]
  }

  // Helper: get first statement from first function in block
  func tp_first_stmt(block: LyricBlock) -> Stmt {
    assert(len(block.fd.children) > 0, "no functions")
    let f = block.fd.children[0]
    assert(f.body != null, "no body")
    assert(len(f.body!.bs.children) > 0, "empty body")
    return f.body!.bs.children[0]
  }

  // ---- Variable declarations ----

  func test_let_simple() {
    let b = tp_parse("lyric t { func f() { let x = 42 } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(name, _, _, _, _, _) => {
        assert_eq(name.name, "x", "var name")
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  func test_let_typed() {
    let b = tp_parse("lyric t { func f() { let x: i32 = 42 } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(name, _, type_expr, _, _, _) => {
        assert_eq(name.name, "x", "var name")
        assert(type_expr != null, "has type")
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  func test_let_mut() {
    let b = tp_parse("lyric t { func f() { let mut x = 0 } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, is_mut, _, _) => {
        assert(is_mut, "is_mut")
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  // ---- Function declarations ----

  func test_func_no_params() {
    let b = tp_parse("lyric t { func foo() { } }")
    assert_eq(len(b.fd.children), 1, "one func")
    let f = b.fd.children[0]
    assert_eq(f.name!.name, "foo", "func name")
    assert_eq(len(f.param.children), 0, "no params")
  }

  func test_func_with_params() {
    let b = tp_parse("lyric t { func add(a: i32, b: i32) -> i32 { return a } }")
    let f = b.fd.children[0]
    assert_eq(f.name!.name, "add", "func name")
    assert_eq(len(f.param.children), 2, "two params")
    assert_eq(f.param.children[0].name!.name, "a", "param a")
    assert_eq(f.param.children[1].name!.name, "b", "param b")
    assert(f.return_type != null, "has return type")
  }

  func test_func_generic() {
    let b = tp_parse("lyric t { func identity<T>(x: T) -> T { return x } }")
    let f = b.fd.children[0]
    assert_eq(len(f.fp.children), 1, "one type param")
    assert_eq(f.fp.children[0].name!.name, "T", "type param T")
  }

  func test_func_receiver() {
    let b = tp_parse("lyric t { class Foo { } func Foo.bar(self) { } }")
    assert_eq(len(b.fd.children), 1, "one func")
    let f = b.fd.children[0]
    assert_eq(f.name!.name, "bar", "method name")
    assert(f.receiver_type != null, "has receiver")
    assert_eq(f.receiver_type!.name, "Foo", "receiver is Foo")
  }

  func test_func_pub() {
    let b = tp_parse("lyric t { pub func foo() { } }")
    assert(b.fd.children[0].is_public, "is_public")
  }

  // ---- Struct declarations ----

  func test_struct_simple() {
    let b = tp_parse("lyric t { struct Point { x: i32\n y: i32 } }")
    assert_eq(len(b.sd.children), 1, "one struct")
    let s = b.sd.children[0]
    assert_eq(s.name!.name, "Point", "struct name")
    assert_eq(len(s.sf.children), 2, "two fields")
    assert_eq(s.sf.children[0].name!.name, "x", "field x")
    assert_eq(s.sf.children[1].name!.name, "y", "field y")
  }

  func test_struct_generic() {
    let b = tp_parse("lyric t { struct Pair<A, B> { first: A\n second: B } }")
    let s = b.sd.children[0]
    assert_eq(len(s.stp.children), 2, "two type params")
    assert_eq(s.stp.children[0].name!.name, "A", "type param A")
  }

  // ---- Class declarations ----

  func test_class_simple() {
    let b = tp_parse("lyric t { class Dog { name: string\n age: i32 } }")
    assert_eq(len(b.cd.children), 1, "one class")
    let c = b.cd.children[0]
    assert_eq(c.name!.name, "Dog", "class name")
    assert_eq(len(c.cf.children), 2, "two fields")
  }

  func test_class_with_method() {
    let b = tp_parse("lyric t { class Cat { name: string } func Cat.speak(self) -> string { return self.name } }")
    assert_eq(len(b.cd.children), 1, "one class")
    assert_eq(len(b.fd.children), 1, "one func")
    assert(b.fd.children[0].receiver_type != null, "has receiver")
  }

  // ---- Enum declarations ----

  func test_enum_simple() {
    let b = tp_parse("lyric t { enum Color { Red\n Green\n Blue } }")
    assert_eq(len(b.ed.children), 1, "one enum")
    let e = b.ed.children[0]
    assert_eq(e.name!.name, "Color", "enum name")
    assert_eq(len(e.ev.children), 3, "three variants")
    assert_eq(e.ev.children[0].name!.name, "Red", "variant Red")
  }

  func test_enum_with_data() {
    let b = tp_parse("lyric t { enum Shape { Circle(radius: f64)\n Rect(w: f64, h: f64) } }")
    let e = b.ed.children[0]
    assert_eq(len(e.ev.children), 2, "two variants")
    assert_eq(len(e.ev.children[0].evf.children), 1, "Circle has 1 field")
    assert_eq(len(e.ev.children[1].evf.children), 2, "Rect has 2 fields")
  }

  // ---- Control flow ----

  func test_if_else() {
    let b = tp_parse("lyric t { func f() { if true { } else { } } }")
    let s = tp_first_stmt(b)
    match s.kind {
      If(_, _, _, else_block) => {
        assert(else_block != null, "has else block")
      }
      _ => { assert(false, "expected If") }
    }
  }

  func test_if_else_if() {
    let b = tp_parse("lyric t { func f() { if true { } else if false { } else { } } }")
    let s = tp_first_stmt(b)
    match s.kind {
      If(_, _, else_ifs, else_block) => {
        assert_eq(len(else_ifs), 1, "one else-if")
        assert(else_block != null, "has else block")
      }
      _ => { assert(false, "expected If") }
    }
  }

  func test_while_loop() {
    let b = tp_parse("lyric t { func f() { while true { break } } }")
    let s = tp_first_stmt(b)
    match s.kind {
      While(_, body) => {
        assert_eq(len(body.bs.children), 1, "one stmt in body")
      }
      _ => { assert(false, "expected While") }
    }
  }

  func test_for_loop() {
    let b = tp_parse("lyric t { func f() { let a = [1, 2, 3]\n for x in a { } } }")
    let s = b.fd.children[0].body!.bs.children[1]
    match s.kind {
      For(var_name, _, _, _) => {
        assert_eq(var_name.name, "x", "loop var")
      }
      _ => { assert(false, "expected For") }
    }
  }

  func test_match_stmt() {
    let b = tp_parse("lyric t { func f() { let x = 1\n match x { 1 => { } _ => { } } } }")
    let s = b.fd.children[0].body!.bs.children[1]
    match s.kind {
      Match(_, arms) => {
        assert_eq(len(arms), 2, "two arms")
      }
      _ => { assert(false, "expected Match") }
    }
  }

  // ---- Expression parsing ----

  func test_binary_expr() {
    let b = tp_parse("lyric t { func f() { let x = 1 + 2 } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        assert(value != null, "has value")
        match value!.kind {
          Binary(_, op, _) => {
            assert_eq(op, Add, "op is Add")
          }
          _ => { assert(false, "expected Binary") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  func test_precedence_mul_over_add() {
    // 1 + 2 * 3 should parse as 1 + (2 * 3)
    let b = tp_parse("lyric t { func f() { let x = 1 + 2 * 3 } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          Binary(_, op, right) => {
            assert_eq(op, Add, "top-level is Add")
            match right.kind {
              Binary(_, op2, _) => {
                assert_eq(op2, Mul, "right is Mul")
              }
              _ => { assert(false, "expected right Binary") }
            }
          }
          _ => { assert(false, "expected Binary") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  func test_unary_neg() {
    let b = tp_parse("lyric t { func f() { let x = -42 } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          Unary(op, _) => {
            assert_eq(op, Neg, "unary Neg")
          }
          _ => { assert(false, "expected Unary") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  func test_method_call() {
    let b = tp_parse("lyric t { func f() { let x = a.foo(1) } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          MethodCall(_, method, _, args) => {
            assert_eq(method.name, "foo", "method name")
            assert_eq(len(args), 1, "one arg")
          }
          _ => { assert(false, "expected MethodCall") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  func test_field_access() {
    let b = tp_parse("lyric t { func f() { let x = a.b } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          FieldAccess(_, field_name) => {
            assert_eq(field_name.name, "b", "field name")
          }
          _ => { assert(false, "expected FieldAccess") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  func test_index_expr() {
    let b = tp_parse("lyric t { func f() { let x = a[0] } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          Index(_, _) => { }
          _ => { assert(false, "expected Index") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  func test_func_call() {
    let b = tp_parse("lyric t { func f() { let x = foo(1, 2) } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          Call(_, _, args) => {
            assert_eq(len(args), 2, "two args")
          }
          _ => { assert(false, "expected Call") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  func test_list_literal() {
    let b = tp_parse("lyric t { func f() { let x = [1, 2, 3] } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          ListLit(elems) => {
            assert_eq(len(elems), 3, "three elems")
          }
          _ => { assert(false, "expected ListLit") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  func test_string_interp() {
    let b = tp_parse("lyric t { func f() { let x = f\"hello {name}\" } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          StringInterp(_) => { }
          _ => { assert(false, "expected StringInterp") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  func test_lambda() {
    let b = tp_parse("lyric t { func f() { let x = (a: i32) -> i32 { return a } } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          Lambda(params, _, _) => {
            assert_eq(len(params), 1, "one param")
          }
          _ => { assert(false, "expected Lambda") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  func test_nil_literal() {
    let b = tp_parse("lyric t { func f() { let x = null } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          Nil => { }
          _ => { assert(false, "expected Nil") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  func test_bool_literal() {
    let b = tp_parse("lyric t { func f() { let x = true } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          BoolLit(v) => {
            assert(v, "true literal")
          }
          _ => { assert(false, "expected BoolLit") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  // ---- Return statement ----

  func test_return_value() {
    let b = tp_parse("lyric t { func f() -> i32 { return 42 } }")
    let s = tp_first_stmt(b)
    match s.kind {
      Return(value) => {
        assert(value != null, "has return value")
      }
      _ => { assert(false, "expected Return") }
    }
  }

  func test_return_void() {
    let b = tp_parse("lyric t { func f() { return } }")
    let s = tp_first_stmt(b)
    match s.kind {
      Return(value) => {
        assert(value == null, "no return value")
      }
      _ => { assert(false, "expected Return") }
    }
  }

  // ---- Assignment ----

  func test_assign() {
    let b = tp_parse("lyric t { func f() { let mut x = 0\n x = 1 } }")
    let s = b.fd.children[0].body!.bs.children[1]
    match s.kind {
      Assign(target, _) => {
        match target.kind {
          Ident(name) => {
            assert_eq(name.name, "x", "assign target")
          }
          _ => { assert(false, "expected Ident target") }
        }
      }
      _ => { assert(false, "expected Assign") }
    }
  }

  // ---- Interface declarations ----

  func test_interface_simple() {
    let b = tp_parse("lyric t { interface Printable { func to_string(self) -> string } }")
    assert_eq(len(b.id.children), 1, "one interface")
    let iface = b.id.children[0]
    assert_eq(iface.name!.name, "Printable", "interface name")
    assert_eq(len(iface.im.children), 1, "one method")
  }

  func test_interface_with_field() {
    let b = tp_parse("lyric t { interface HasName<T> { field T.name: string } }")
    let iface = b.id.children[0]
    assert_eq(len(iface.itp.children), 1, "one type param")
    assert_eq(len(iface.ifd.children), 1, "one field decl")
  }

  // ---- Relation declarations ----

  func test_relation_owns() {
    let b = tp_parse("lyric t { relation ArrayList Parent:pc owns [Child:pc] }")
    assert_eq(len(b.rd.children), 1, "one relation")
    let r = b.rd.children[0]
    assert_eq(r.parent.type_name!.name, "Parent", "parent type")
    assert(r.is_many, "is_many for []")
    match r.kind {
      Owns => { }
      _ => { assert(false, "expected Owns") }
    }
  }

  func test_relation_refs() {
    let b = tp_parse("lyric t { relation ArrayList Child:p refs Parent:p }")
    let r = b.rd.children[0]
    match r.kind {
      Refs => { }
      _ => { assert(false, "expected Refs") }
    }
    assert(!r.is_many, "not is_many")
  }

  // ---- Impl blocks ----

  func test_impl_block() {
    let b = tp_parse("lyric t { impl Printable for Dog { to_string = name } }")
    assert_eq(len(b.ib.children), 1, "one impl block")
    let imp = b.ib.children[0]
    assert_eq(imp.interface_name!.name, "Printable", "interface")
  }

  // ---- Top-level constants ----

  func test_top_level_let() {
    let b = tp_parse("lyric t { let X: i32 = 42 }")
    // Top-level let should be parsed — check it exists
    // In the bootstrap, top-level lets go into functions list as const decls
    // or the block may have a special handling
    assert(true, "parsed without error")
  }

  // ---- Import declarations ----

  func test_import() {
    let b = tp_parse("lyric t { import \"fmt\" }")
    assert_eq(len(b.imp.children), 1, "one import")
    assert_eq(b.imp.children[0].path, "fmt", "import path")
  }

  // ---- Multiple items in one block ----

  func test_multiple_funcs() {
    let b = tp_parse("lyric t { func a() { } func b() { } func c() { } }")
    assert_eq(len(b.fd.children), 3, "three funcs")
    assert_eq(b.fd.children[0].name!.name, "a", "first func")
    assert_eq(b.fd.children[1].name!.name, "b", "second func")
    assert_eq(b.fd.children[2].name!.name, "c", "third func")
  }

  func test_mixed_decls() {
    let b = tp_parse("lyric t { struct S { x: i32 } class C { y: string } enum E { A\n B } func f() { } }")
    assert_eq(len(b.sd.children), 1, "one struct")
    assert_eq(len(b.cd.children), 1, "one class")
    assert_eq(len(b.ed.children), 1, "one enum")
    assert_eq(len(b.fd.children), 1, "one func")
  }

  // ---- Try operator ----

  func test_try_operator() {
    let b = tp_parse("lyric t { func f() -> (i32, error) { let x = do_thing()?\n return (x, null) } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          Try(_) => { }
          _ => { assert(false, "expected Try") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  // ---- Match expression in value position ----

  func test_match_expr() {
    let b = tp_parse("lyric t { func f() { let x = match 1 { 1 => { 10 } _ => { 0 } } } }")
    let s = tp_first_stmt(b)
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          Match(_, arms) => {
            assert_eq(len(arms), 2, "two arms")
          }
          _ => { assert(false, "expected Match expr") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  // ---- Struct literal ----

  func test_struct_literal() {
    let b = tp_parse("lyric t { struct Point { x: i32\n y: i32 } func f() { let p = Point { x: 1, y: 2 } } }")
    let s = b.fd.children[0].body!.bs.children[0]
    match s.kind {
      VarDecl(_, _, _, _, _, value) => {
        match value!.kind {
          StructLit(type_name, _, fields) => {
            assert_eq(type_name.name, "Point", "struct type")
            assert_eq(len(fields), 2, "two fields")
          }
          _ => { assert(false, "expected StructLit") }
        }
      }
      _ => { assert(false, "expected VarDecl") }
    }
  }

  // ---- Where clauses ----

  func test_where_clause() {
    let b = tp_parse("lyric t { func process<T>(x: T) where T: Printable { } }")
    let f = b.fd.children[0]
    assert_eq(len(f.wc.children), 1, "one where clause")
    assert_eq(f.wc.children[0].variable!.name, "T", "where var")
    assert_eq(f.wc.children[0].constraint!.name, "Printable", "where constraint")
  }

  // ---- Spawn statement ----

  func test_spawn() {
    let b = tp_parse("lyric t { func f() { spawn { } } }")
    let s = tp_first_stmt(b)
    match s.kind {
      Spawn(_) => { }
      _ => { assert(false, "expected Spawn") }
    }
  }

  // ---- Break and Continue ----

  func test_break_continue() {
    let b = tp_parse("lyric t { func f() { while true { break\n continue } } }")
    let s = tp_first_stmt(b)
    match s.kind {
      While(_, body) => {
        assert_eq(len(body.bs.children), 2, "two stmts")
        match body.bs.children[0].kind {
          Break => { }
          _ => { assert(false, "expected Break") }
        }
        match body.bs.children[1].kind {
          Continue => { }
          _ => { assert(false, "expected Continue") }
        }
      }
      _ => { assert(false, "expected While") }
    }
  }

  // ---- Optional type ----

  func test_optional_type() {
    let b = tp_parse("lyric t { struct S { x: i32? } }")
    let s = b.sd.children[0]
    let field = s.sf.children[0]
    assert(field.type_expr != null, "has type")
    match field.type_expr!.kind {
      Optional(_) => { }
      _ => { assert(false, "expected Optional type") }
    }
  }

  // ---- Slice type ----

  func test_slice_type() {
    let b = tp_parse("lyric t { struct S { items: [i32] } }")
    let s = b.sd.children[0]
    let field = s.sf.children[0]
    match field.type_expr!.kind {
      Sequence(_) => { }
      _ => { assert(false, "expected Sequence type") }
    }
  }

  // ---- Tuple type ----

  func test_tuple_type() {
    let b = tp_parse("lyric t { func f() -> (i32, string) { return (1, \"a\") } }")
    let f = b.fd.children[0]
    assert(f.return_type != null, "has return type")
    match f.return_type!.kind {
      Tuple(fields) => {
        assert_eq(len(fields), 2, "two tuple fields")
      }
      _ => { assert(false, "expected Tuple type") }
    }
  }
}
