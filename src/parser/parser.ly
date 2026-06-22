// parser.ly — PEG parser for .lyric/.ly files
// Ported from pkg/parser/parser.go
//
// Design:
//   - Parser is a class wrapping Lexer
//   - Returns (T, error) using stdlib Error for now
//   - All parse methods are external: func Parser.xxx(self)

lyric parser {

  // ---- Parser ----

  permanent class Parser {
    lex: Lexer?
    no_struct_lit: bool
    pushed: Token?
    expr_depth: i32
  }

  func new_parser(src_text: string, filename: string) -> Parser {
    let lex = new_lexer(src_text, sym(filename))
    return Parser { lex: lex }
  }

  func parse_file(src_text: string, filename: string) -> (File?, error) {
    let p = new_parser(src_text, filename)
    return p.do_parse_file()
  }

  // ---- Helpers ----

  func Parser.peek(self) -> Token {
    if self.pushed != null {
      return self.pushed!
    }
    return self.lex!.peek()
  }

  func Parser.next(self) -> Token {
    if self.pushed != null {
      let t = self.pushed!
      self.pushed = null
      return t
    }
    return self.lex!.next()
  }

  func Parser.push_back(self, t: Token) {
    self.pushed = t
  }

  func Parser.skip_newlines(self) {
    while self.peek().kind == SNewline {
      self.next()
    }
  }

  func Parser.make_error(self, span: Span, msg: string) -> Error {
    return Error { msg: f"{self.lex!.filename!.get_name()}:{itoa(span.start.line)}:{itoa(span.start.column)}: {msg}" }
  }

  func Parser.expect(self, kind: TokenKind) -> (Token?, error) {
    let tok = self.next()
    if tok.kind != kind {
      return (null, self.make_error(tok.span, f"expected {kind}, got {tok.kind} ({tok.text})"))
    }
    return (tok, null)
  }

  func Parser.expect_ident(self) -> (Token?, error) {
    let tok = self.next()
    if tok.kind == LIdent {
      return (tok, null)
    }
    // Accept certain reserved words as identifiers
    match tok.kind {
      KField => { return (tok, null) }
      KDestructor => { return (tok, null) }
      KImplements => { return (tok, null) }
      KFrom => { return (tok, null) }
      KAs => { return (tok, null) }
      KIs => { return (tok, null) }
      KIn => { return (tok, null) }
      _ => {}
    }
    return (null, self.make_error(tok.span, f"expected identifier, got {tok.kind} ({tok.text})"))
  }

  func Parser.make_span(self, start: Pos) -> Span {
    return Span { start: start, end: self.peek().span.end }
  }

  // peek_annotation checks if the current token is a contextual keyword
  // (field, lock, implements) used as an identifier. Returns the keyword
  // TokenKind if it matches, or null if not.
  func Parser.peek_annotation(self) -> TokenKind? {
    let tok = self.peek()
    if tok.kind != LIdent {
      return null
    }
    if tok.text == "field" { return KField }
    if tok.text == "lock" { return KLock }
    if tok.text == "implements" { return KImplements }
    return null
  }

  // consume_annotation consumes the current token if it's a contextual keyword
  // matching the given kind. Returns true if consumed.
  func Parser.consume_annotation(self, kind: TokenKind) -> bool {
    let ann = self.peek_annotation()
    if isnull(ann) { return false }
    if ann! == kind {
      self.next()
      return true
    }
    return false
  }

  // ---- Top-level parsing ----

  func Parser.do_parse_file(self) -> (File?, error) {
    let start = self.peek().span.start
    let file = File { filename: self.lex!.filename!.get_name(), span: Span { start: start, end: start } }
    self.skip_newlines()

    while self.peek().kind != SEOF {
      self.skip_newlines()
      if self.peek().kind == SEOF {
        break
      }
      let block = self.parse_lyric_block()?
      array_append<File, LyricBlock>(file, block!)
      self.skip_newlines()
    }

    file.span = self.make_span(start)
    return (file, null)
  }

  func Parser.parse_lyric_block(self) -> (LyricBlock?, error) {
    let start = self.peek().span.start

    // Implicit lyric block: if file doesn't start with `lyric`, derive name from filename
    let implicit = self.peek().kind != KLyric
    let mut block: LyricBlock

    if implicit {
      // Derive module name from filename: "/path/foo.ly" -> "foo"
      let fname = self.lex!.filename!.get_name()
      let parts = str_split(fname, "/")
      let mut base = parts[len(parts) - 1]
      let dot_parts = str_split(base, ".")
      base = dot_parts[0]
      if len(base) == 0 {
        base = "main"
      }
      block = LyricBlock { name: sym(base), span: Span { start: start, end: start } }
    } else {
      self.expect(KLyric)?
      let name_tok = self.expect(LIdent)?
      block = LyricBlock { name: sym(name_tok!.text), span: Span { start: start, end: start } }
      self.expect(PLBrace)?
    }

    self.skip_newlines()

    let end_kind = PRBrace
    if implicit {
      while self.peek().kind != SEOF {
        self.parse_lyric_item(block)?
        self.skip_newlines()
      }
    } else {
      while self.peek().kind != PRBrace && self.peek().kind != SEOF {
        self.parse_lyric_item(block)?
        self.skip_newlines()
      }
      self.expect(PRBrace)?
    }

    block.span = self.make_span(start)
    return (block, null)
  }

  // ---- parse_lyric_item ----

  func Parser.parse_lyric_item(self, block: LyricBlock) -> (bool, error) {
    let tok = self.peek()

    // Handle `pub` visibility modifier
    let mut is_pub = false
    if tok.kind == KPub {
      is_pub = true
      self.next()
    }

    // Handle `permanent` modifier for classes
    let mut is_permanent = false
    if self.peek().kind == LIdent && self.peek().text == "permanent" {
      is_permanent = true
      self.next()
    }

    // Handle `trusted` modifier for functions
    let mut is_trusted = false
    if self.peek().kind == LIdent && self.peek().text == "trusted" {
      is_trusted = true
      self.next()
    }

    // Handle `final` modifier for functions
    let mut is_final = false
    if self.peek().kind == LIdent && self.peek().text == "final" {
      is_final = true
      self.next()
    }

    match self.peek().kind {
      KImport => {
        if is_pub { return (false, self.make_error(tok.span, "pub cannot be applied to import")) }
        let imp = self.parse_import()?
        array_append<LyricBlock, ImportDecl>(block, imp!)
        return (true, null)
      }
      KLet => {
        let decl = self.parse_const_decl()?
        decl!.is_public = is_pub
        array_append<LyricBlock, ConstDecl>(block, decl!)
        return (true, null)
      }
      KStruct => {
        let s = self.parse_struct()?
        s!.is_public = is_pub
        array_append<LyricBlock, StructDecl>(block, s!)
        return (true, null)
      }
      KEnum => {
        let e = self.parse_enum()?
        e!.is_public = is_pub
        array_append<LyricBlock, EnumDecl>(block, e!)
        return (true, null)
      }
      KInterface => {
        let iface = self.parse_interface()?
        iface!.is_public = is_pub
        array_append<LyricBlock, InterfaceDecl>(block, iface!)
        return (true, null)
      }
      KClass => {
        let cls = self.parse_class()?
        cls!.is_public = is_pub
        cls!.is_permanent = is_permanent
        array_append<LyricBlock, ClassDecl>(block, cls!)
        return (true, null)
      }
      KFunc => {
        let fn = self.parse_func()?
        fn!.is_public = is_pub
        fn!.is_trusted = is_trusted
        fn!.is_final = is_final
        array_append<LyricBlock, FuncDecl>(block, fn!)
        return (true, null)
      }
      KRelation => {
        if is_pub { return (false, self.make_error(tok.span, "pub cannot be applied to relation")) }
        let rel = self.parse_relation()?
        array_append<LyricBlock, RelationDecl>(block, rel!)
        return (true, null)
      }
      KType => {
        let ta = self.parse_type_alias()?
        ta!.is_public = is_pub
        array_append<LyricBlock, TypeAliasDecl>(block, ta!)
        return (true, null)
      }
      KImpl => {
        if is_pub { return (false, self.make_error(tok.span, "pub cannot be applied to impl")) }
        let impl_block = self.parse_impl()?
        array_append<LyricBlock, ImplBlock>(block, impl_block!)
        return (true, null)
      }
      _ => {
        if is_permanent {
          return (false, self.make_error(tok.span, "permanent can only be applied to class"))
        }
        if is_trusted {
          return (false, self.make_error(tok.span, "trusted can only be applied to func"))
        }
        if is_final {
          return (false, self.make_error(tok.span, "final can only be applied to func"))
        }
        return (false, self.make_error(tok.span, f"unexpected token in lyric block"))
      }
    }

    return (false, self.make_error(tok.span, f"unexpected token {tok.kind} ({tok.text}) in lyric block"))
  }

  // ---- Simple declarations ----

  func Parser.parse_import(self) -> (ImportDecl?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'import'

    // Three forms:
    //   import "path"              → alias=null, path="path"
    //   import ident               → alias=ident, path=ident
    //   import ident from "path"   → alias=ident, path="path"
    if self.peek().kind == LStringLit {
      let path = self.next()
      return (ImportDecl {
        path: path.text,
        span: self.make_span(start)
      }, null)
    }

    let alias = self.expect(LIdent)?
    // Check for optional 'from "path"'
    if self.peek().kind == KFrom {
      self.next()  // consume 'from'
      let path = self.expect(LStringLit)?
      return (ImportDecl {
        sym(alias!.text),
        path: path!.text,
        span: self.make_span(start)
      }, null)
    }
    // Simple form: import ident — alias IS the path
    return (ImportDecl {
      sym(alias!.text),
      path: alias!.text,
      span: self.make_span(start)
    }, null)
  }

  func Parser.parse_type_alias(self) -> (TypeAliasDecl?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'type'
    let name = self.expect_ident()?
    self.expect(OAssign)?
    let type_expr = self.parse_type_expr()?
    return (TypeAliasDecl {
      sym(name!.text),
      is_public: false,
      type_expr: type_expr,
      span: self.make_span(start)
    }, null)
  }

  // ---- Fields and params ----

  func Parser.parse_field(self) -> (Field?, error) {
    let start = self.peek().span.start
    let name = self.expect_ident()?
    self.expect(PColon)?
    let typ = self.parse_type_expr()?
    let field = Field {
      name: sym(name!.text),
      is_public: false,
      type_expr: typ,
      span: self.make_span(start)
    }
    // Optional guarded_by (contextual keyword — lexer emits LIdent)
    if self.peek().kind == LIdent && self.peek().text == "guarded_by" {
      self.next()
      self.expect(PLParen)?
      let lock = self.expect(LIdent)?
      field.guarded_by = sym(lock!.text)
      self.expect(PRParen)?
    }
    // Optional default value
    if self.peek().kind == OAssign {
      self.next()
      let def_expr = self.parse_expr()?
      field.default_value = def_expr
    }
    return (field, null)
  }

  func Parser.parse_param(self) -> (Param?, error) {
    let start = self.peek().span.start
    let mut is_mut = false
    if self.peek().kind == KMut {
      is_mut = true
      self.next()
    }
    // Check for self
    if self.peek().kind == KSelf {
      let tok = self.next()
      return (Param {
        `self`,
        is_mut: is_mut,
        is_self: true,
        span: self.make_span(start)
      }, null)
    }
    let name = self.expect_ident()?
    if name!.text == "self" {
      return (Param {
        `self`,
        is_mut: is_mut,
        is_self: true,
        span: self.make_span(start)
      }, null)
    }
    self.expect(PColon)?
    let typ = self.parse_type_expr()?
    return (Param {
      sym(name!.text),
      is_mut: is_mut,
      is_self: false,
      type_expr: typ,
      span: self.make_span(start)
    }, null)
  }

  func Parser.parse_tuple_field(self) -> (TupleField?, error) {
    // Could be "name: Type" or just "Type"
    if self.peek().kind == LIdent {
      let saved = self.lex!.save_state()
      let name = self.next()
      if self.peek().kind == PColon {
        self.next()
        let typ = self.parse_type_expr()?
        return (TupleField { sym(name.text), type_expr: typ }, null)
      }
      // Restore — it was just a type name
      self.lex!.restore_state(saved)
    }
    let typ = self.parse_type_expr()?
    return (TupleField { type_expr: typ }, null)
  }

  // ---- Type params ----

  func Parser.parse_type_params(self) -> ([TypeParam], error) {
    self.expect(PLt)?
    let mut params: [TypeParam] = []
    while self.peek().kind != PGt && self.peek().kind != SEOF {
      let start = self.peek().span.start
      let name = self.expect(LIdent)?
      let tp = TypeParam { name: sym(name!.text), span: self.make_span(start) }
      if self.peek().kind == PColon {
        self.next()
        let constraint = self.expect(LIdent)?
        tp.constraint = sym(constraint!.text)
      }
      params = append(params, tp)
      if self.peek().kind == PComma {
        self.next()
      }
    }
    self.expect(PGt)?
    return (params, null)
  }

  // ---- Struct ----

  func Parser.parse_struct(self) -> (StructDecl?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'struct'
    let name = self.expect_ident()?
    let s = StructDecl { name: sym(name!.text), span: Span { start: start, end: start } }

    if self.peek().kind == PLt {
      let params = self.parse_type_params()?
      for tp in params {
        array_append<StructDecl, TypeParam>(s, tp)
      }
    }

    self.expect(PLBrace)?
    self.skip_newlines()

    while self.peek().kind != PRBrace && self.peek().kind != SEOF {
      let field = self.parse_field()?
      array_append<StructDecl, Field>(s, field!)
      if self.peek().kind == PComma {
        self.next()  // allow optional comma between fields
      }
      self.skip_newlines()
    }

    self.expect(PRBrace)?
    s.span = self.make_span(start)
    return (s, null)
  }

  // ---- Enum ----

  func Parser.parse_enum(self) -> (EnumDecl?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'enum'
    let name = self.expect(LIdent)?
    let e = EnumDecl { name: sym(name!.text), span: Span { start: start, end: start } }

    if self.peek().kind == PLt {
      let params = self.parse_type_params()?
      for tp in params {
        array_append<EnumDecl, TypeParam>(e, tp)
      }
    }

    self.expect(PLBrace)?
    self.skip_newlines()

    while self.peek().kind != PRBrace && self.peek().kind != SEOF {
      let variant = self.parse_enum_variant()?
      array_append<EnumDecl, EnumVariant>(e, variant!)
      self.skip_newlines()
    }

    self.expect(PRBrace)?
    e.span = self.make_span(start)
    return (e, null)
  }

  func Parser.parse_enum_variant(self) -> (EnumVariant?, error) {
    let start = self.peek().span.start
    let name = self.expect(LIdent)?
    let v = EnumVariant { name: sym(name!.text), span: Span { start: start, end: start } }

    if self.peek().kind == PLParen {
      self.next()
      self.skip_newlines()
      while self.peek().kind != PRParen && self.peek().kind != SEOF {
        let field = self.parse_tuple_field()?
        array_append<EnumVariant, TupleField>(v, field!)
        if self.peek().kind == PComma {
          self.next()
        }
        self.skip_newlines()
      }
      self.expect(PRParen)?
    }

    v.span = self.make_span(start)
    return (v, null)
  }

  // ---- Interface ----

  func Parser.parse_interface(self) -> (InterfaceDecl?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'interface'
    let name = self.expect(LIdent)?
    let iface = InterfaceDecl { name: sym(name!.text), span: Span { start: start, end: start } }

    if self.peek().kind == PLt {
      let params = self.parse_type_params()?
      for tp in params {
        array_append<InterfaceDecl, TypeParam>(iface, tp)
      }
    }

    self.expect(PLBrace)?
    self.skip_newlines()

    while self.peek().kind != PRBrace && self.peek().kind != SEOF {
      // Check contextual keywords (field, implements) before match
      let ann = self.peek_annotation()
      if !isnull(ann) && ann! == KImplements {
        self.next()
        let imp = self.expect(LIdent)?
        iface.implements = append(iface.implements, sym(imp!.text))
        self.skip_newlines()
        continue
      }
      if !isnull(ann) && ann! == KField {
        let fd = self.parse_interface_field()?
        array_append<InterfaceDecl, InterfaceFieldDecl>(iface, fd!)
        self.skip_newlines()
        continue
      }
      match self.peek().kind {
        KImplements => {
          self.next()
          let imp = self.expect(LIdent)?
          iface.implements = append(iface.implements, sym(imp!.text))
        }
        KPub => {
          self.next()
          let mut pub_trusted = false
          if self.peek().kind == LIdent && self.peek().text == "trusted" {
            pub_trusted = true
            self.next()
          }
          let fn = self.parse_func()?
          fn!.is_public = true
          fn!.is_trusted = pub_trusted
          array_append<InterfaceDecl, FuncDecl>(iface, fn!)
        }
        KFunc => {
          let fn = self.parse_func()?
          array_append<InterfaceDecl, FuncDecl>(iface, fn!)
        }
        KField => {
          let fd = self.parse_interface_field()?
          array_append<InterfaceDecl, InterfaceFieldDecl>(iface, fd!)
        }
        KDestructor => {
          let db = self.parse_destructor_block()?
          array_append<InterfaceDecl, DestructorBlock>(iface, db!)
        }
        _ => {
          return (null, self.make_error(self.peek().span, f"unexpected {self.peek().kind} in interface body"))
        }
      }
      self.skip_newlines()
    }
    self.expect(PRBrace)?
    iface.span = self.make_span(start)
    return (iface, null)
  }

  func Parser.parse_interface_field(self) -> (InterfaceFieldDecl?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'field'
    let type_name = self.expect(LIdent)?
    self.expect(PDot)?
    let field_name = self.expect(LIdent)?
    self.expect(PColon)?
    let te = self.parse_type_expr()?
    return (InterfaceFieldDecl {
      sym(type_name!.text),
      name: sym(field_name!.text),
      type_expr: te,
      span: self.make_span(start)
    }, null)
  }

  func Parser.parse_destructor_block(self) -> (DestructorBlock?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'destructor'
    // Optional kind keyword: `destructor owns P { ... }` or `destructor refs P { ... }`.
    // Absent kind keeps legacy behavior (Owns), so existing stdlib still parses.
    let mut kind = Owns
    if self.peek().kind == KOwns {
      self.next()
    } else if self.peek().kind == KRefs {
      self.next()
      kind = Refs
    }
    let type_name = self.expect(LIdent)?
    let body = self.parse_block()?
    return (DestructorBlock {
      sym(type_name!.text),
      kind: kind,
      body: body,
      span: self.make_span(start)
    }, null)
  }

  // ---- Impl ----

  func Parser.parse_impl(self) -> (ImplBlock?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'impl'
    let name = self.expect(LIdent)?
    let impl_block = ImplBlock { interface_name: sym(name!.text), span: Span { start: start, end: start } }

    if self.peek().kind == PLt {
      self.next()
      while self.peek().kind != PGt && self.peek().kind != SEOF {
        let te = self.parse_type_expr()?
        // Optional per-class-type-variable label: `: <ident>`. See
        // cr/docs/multi-class-interface-redesign.md §3.8.
        let mut arg_label: Sym? = null
        if self.peek().kind == PColon {
          self.next()
          let lbl = self.expect(LIdent)?
          arg_label = sym(lbl!.text)
        }
        let arg = ImplTypeArg { type_expr: te, label: arg_label, span: te!.span }
        array_append<ImplBlock, ImplTypeArg>(impl_block, arg)
        if self.peek().kind == PComma {
          self.next()
        }
      }
      self.expect(PGt)?
    }

    // Optional: for ConcreteType
    if self.peek().kind == KFor {
      self.next()
      let for_type = self.expect(LIdent)?
      impl_block.for_type = sym(for_type!.text)
    }

    self.expect(PLBrace)?
    self.skip_newlines()

    while self.peek().kind != PRBrace && self.peek().kind != SEOF {
      if impl_block.for_type != null {
        let mapping = self.parse_impl_mapping_short(impl_block.for_type!)?
        array_append<ImplBlock, ImplMapping>(impl_block, mapping!)
      } else {
        let mapping = self.parse_impl_mapping()?
        array_append<ImplBlock, ImplMapping>(impl_block, mapping!)
      }
      self.skip_newlines()
    }

    self.expect(PRBrace)?
    impl_block.span = self.make_span(start)
    return (impl_block, null)
  }

  // Short-form mapping: method = member (used with "impl Iface for Type")
  func Parser.parse_impl_mapping_short(self, for_type: Sym) -> (ImplMapping?, error) {
    let start = self.peek().span.start
    let method_name = self.expect(LIdent)?

    let m = ImplMapping {
      type_param: for_type,
      method_name: sym(method_name!.text),
      kind: Alias,
      span: Span { start: start, end: start }
    }

    match self.peek().kind {
      OAssign => {
        self.next()
        let member = self.expect(LIdent)?
        m.kind = Alias
        m.target_class = for_type
        m.target_member = sym(member!.text)
      }
      PBiArrow => {
        self.next()
        let member = self.expect(LIdent)?
        m.kind = FieldBind
        m.target_class = for_type
        m.target_member = sym(member!.text)
      }
      PLParen => {
        let fn = FuncDecl {
          name: sym(method_name!.text),
          receiver_type: for_type,
          span: Span { start: start, end: start }
        }
        self.next()  // consume '('
        self.skip_newlines()
        while self.peek().kind != PRParen && self.peek().kind != SEOF {
          let param = self.parse_param()?
          array_append<FuncDecl, Param>(fn, param!)
          if self.peek().kind == PComma {
            self.next()
          }
          self.skip_newlines()
        }
        self.expect(PRParen)?
        if self.peek().kind == PArrow {
          self.next()
          let ret = self.parse_type_expr()?
          fn.return_type = ret
        }
        let body = self.parse_block()?
        fn.body = body
        m.kind = Inline
        m.inline_func = fn
      }
      _ => {
        return (null, self.make_error(self.peek().span, f"expected '=', '<->', or '(' in impl mapping"))
      }
    }

    m.span = self.make_span(start)
    return (m, null)
  }

  func Parser.parse_impl_mapping(self) -> (ImplMapping?, error) {
    let start = self.peek().span.start
    let type_param = self.expect(LIdent)?
    self.expect(PDot)?
    let method_name = self.expect(LIdent)?

    let m = ImplMapping {
      type_param: sym(type_param!.text),
      method_name: sym(method_name!.text),
      kind: Alias,
      span: Span { start: start, end: start }
    }

    match self.peek().kind {
      OAssign => {
        // T.method = Class.method
        self.next()
        let cls = self.expect(LIdent)?
        self.expect(PDot)?
        let member = self.expect(LIdent)?
        m.kind = Alias
        m.target_class = sym(cls!.text)
        m.target_member = sym(member!.text)
      }
      PBiArrow => {
        // T.field <-> Class.field
        self.next()
        let cls = self.expect(LIdent)?
        self.expect(PDot)?
        let member = self.expect(LIdent)?
        m.kind = FieldBind
        m.target_class = sym(cls!.text)
        m.target_member = sym(member!.text)
      }
      PLParen => {
      // Inline: T.method(params) -> RetType { body }
      let fn = FuncDecl {
        name: sym(method_name!.text),
        receiver_type: sym(type_param!.text),
        span: Span { start: start, end: start }
      }
      self.next()  // consume '('
      self.skip_newlines()
      while self.peek().kind != PRParen && self.peek().kind != SEOF {
        let param = self.parse_param()?
        array_append<FuncDecl, Param>(fn, param!)
        if self.peek().kind == PComma {
          self.next()
        }
        self.skip_newlines()
      }
      self.expect(PRParen)?
      if self.peek().kind == PArrow {
        self.next()
        let ret = self.parse_type_expr()?
        fn.return_type = ret
      }
      let body = self.parse_block()?
      fn.body = body
      m.kind = Inline
      m.inline_func = fn
      }
      _ => {
        return (null, self.make_error(self.peek().span, f"expected '=', '<->', or '(' in impl mapping"))
      }
    }

    m.span = self.make_span(start)
    return (m, null)
  }

  // ---- Class ----

  func Parser.parse_class(self) -> (ClassDecl?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'class'
    let name = self.expect(LIdent)?
    let cls = ClassDecl { name: sym(name!.text), span: Span { start: start, end: start } }

    if self.peek().kind == PLt {
      let params = self.parse_type_params()?
      for tp in params {
        array_append<ClassDecl, TypeParam>(cls, tp)
      }
    }

    // Optional where clause
    self.skip_newlines()
    while self.peek().kind == KWhere {
      self.next()
      while true {
        let wc = self.parse_where_clause()?
        array_append<ClassDecl, WhereClause>(cls, wc!)
        if self.peek().kind == PComma {
          self.next()
        } else {
          break
        }
      }
      self.skip_newlines()
    }

    // Optional implements
    if self.peek().kind == KImplements || (!isnull(self.peek_annotation()) && self.peek_annotation()! == KImplements) {
      self.next()
      while true {
        let imp = self.expect(LIdent)?
        cls.implements = append(cls.implements, sym(imp!.text))
        if self.peek().kind == PComma {
          self.next()
        } else {
          break
        }
      }
    }

    self.expect(PLBrace)?
    self.skip_newlines()

    while self.peek().kind != PRBrace && self.peek().kind != SEOF {
      match self.peek().kind {
        KPub => {
          self.next()
          let mut pub_trusted = false
          if self.peek().kind == LIdent && self.peek().text == "trusted" {
            pub_trusted = true
            self.next()
          }
          if self.peek().kind == KFunc {
            let fn = self.parse_func()?
            fn!.is_public = true
            fn!.is_trusted = pub_trusted
            array_append<ClassDecl, FuncDecl>(cls, fn!)
          } else {
            let field = self.parse_field()?
            field!.is_public = true
            array_append<ClassDecl, Field>(cls, field!)
          }
        }
        KFunc => {
          let fn = self.parse_func()?
          array_append<ClassDecl, FuncDecl>(cls, fn!)
        }
        _ => {
          // Check for 'final func' or 'trusted func' (contextual keywords)
          if self.peek().kind == LIdent && self.peek().text == "final" {
            self.next()  // consume 'final'
            let fn = self.parse_func()?
            fn!.is_final = true
            array_append<ClassDecl, FuncDecl>(cls, fn!)
          } else if self.peek().kind == LIdent && self.peek().text == "trusted" {
            self.next()  // consume 'trusted'
            let fn = self.parse_func()?
            fn!.is_trusted = true
            array_append<ClassDecl, FuncDecl>(cls, fn!)
          } else {
            let field = self.parse_field()?
            array_append<ClassDecl, Field>(cls, field!)
          }
        }
      }
      if self.peek().kind == PComma {
        self.next()  // allow optional comma between class members
      }
      self.skip_newlines()
    }

    self.expect(PRBrace)?
    cls.span = self.make_span(start)
    return (cls, null)
  }

  // ---- Function ----

  func Parser.parse_func(self) -> (FuncDecl?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'func'
    let name = self.expect(LIdent)?
    let fn = FuncDecl { name: sym(name!.text), span: Span { start: start, end: start } }

    // Check for T.method syntax
    if self.peek().kind == PDot {
      self.next()
      let method_name = self.expect(LIdent)?
      fn.receiver_type = sym(name!.text)
      fn.name = sym(method_name!.text)
    }

    // Optional type parameters
    if self.peek().kind == PLt {
      let params = self.parse_type_params()?
      for tp in params {
        array_append<FuncDecl, TypeParam>(fn, tp)
      }
    }

    // Parameters
    self.expect(PLParen)?
    self.skip_newlines()
    while self.peek().kind != PRParen && self.peek().kind != SEOF {
      let param = self.parse_param()?
      array_append<FuncDecl, Param>(fn, param!)
      if self.peek().kind == PComma {
        self.next()
      }
      self.skip_newlines()
    }
    self.expect(PRParen)?

    // Optional return type
    if self.peek().kind == PArrow {
      self.next()
      let ret = self.parse_type_expr()?
      fn.return_type = ret
    }

    // Optional where clause
    self.skip_newlines()
    while self.peek().kind == KWhere {
      self.next()
      while true {
        let wc = self.parse_where_clause()?
        array_append<FuncDecl, WhereClause>(fn, wc!)
        if self.peek().kind == PComma {
          self.next()
        } else {
          break
        }
      }
      self.skip_newlines()
    }

    // Optional body
    if self.peek().kind == PLBrace {
      let body = self.parse_block()?
      fn.body = body
    }

    fn.span = self.make_span(start)
    return (fn, null)
  }

  func Parser.parse_const_decl(self) -> (ConstDecl?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'let'
    let mut is_mut = false
    if self.peek().kind == KMut {
      self.next()
      is_mut = true
    }
    let name = self.expect_ident()?
    let decl = ConstDecl {
      name: sym(name!.text),
      is_public: false,
      span: Span { start: start, end: start },
    }

    // Optional type annotation
    if self.peek().kind == PColon {
      self.next()
      let typ = self.parse_type_expr()?
      decl.type_expr = typ
    }

    // Required initializer
    self.expect(OAssign)?
    let val = self.parse_expr()?
    decl.value = val
    decl.span = self.make_span(start)
    return (decl, null)
  }

  func Parser.parse_where_clause(self) -> (WhereClause?, error) {
    let start = self.peek().span.start
    let name = self.expect(LIdent)?

    // Bare relational constraint: Graph<G, N, E>
    if self.peek().kind == PLt {
      self.next()
      let wc = WhereClause {
        constraint: sym(name!.text),
        span: Span { start: start, end: start }
      }
      while self.peek().kind != PGt && self.peek().kind != SEOF {
        let te = self.parse_type_expr()?
        array_append<WhereClause, TypeExpr>(wc, te!)
        if self.peek().kind == PComma {
          self.next()
        }
      }
      self.expect(PGt)?
      wc.span = self.make_span(start)
      return (wc, null)
    }

    // Single-type constraint: T: Integer
    self.expect(PColon)?
    let constraint = self.expect(LIdent)?
    return (WhereClause {
      sym(name!.text),
      constraint: sym(constraint!.text),
      span: self.make_span(start)
    }, null)
  }

  // ---- Relation ----

  func Parser.parse_relation(self) -> (RelationDecl?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'relation'

    let first = self.next()

    // Look ahead: if next is owns/refs/colon, first is parent; otherwise first is hint
    let mut hint: Sym? = null
    let mut parent_tok = first
    if self.peek().kind != KOwns && self.peek().kind != KRefs && self.peek().kind != PColon {
      hint = sym(first.text)
      parent_tok = self.next()
    }

    let parent = self.parse_relation_side(parent_tok)

    let kind_tok = self.next()
    let mut rel_kind = Owns
    if kind_tok.kind == KRefs {
      rel_kind = Refs
    } else if kind_tok.kind != KOwns {
      return (null, self.make_error(kind_tok.span, f"expected 'owns' or 'refs', got {kind_tok.text}"))
    }

    let mut is_many = false
    let mut child_tok = self.peek()
    if child_tok.kind == PLBracket {
      self.next()
      is_many = true
      child_tok = self.next()
    } else {
      child_tok = self.next()
    }

    let child = self.parse_relation_side(child_tok)

    if is_many {
      self.expect(PRBracket)?
    }

    return (RelationDecl {
      hint,
      parent,
      rel_kind,
      child,
      is_many,
      self.make_span(start)
    }, null)
  }

  func Parser.parse_relation_side(self, first: Token) -> RelationSide {
    // Collect optional type parameters: <V>, <K, V>, etc.
    let mut type_args: [Sym] = []
    if self.peek().kind == PLt {
      self.next()  // consume '<'
      while self.peek().kind != PGt && self.peek().kind != SEOF {
        let tok = self.next()
        if tok.kind == LIdent {
          type_args = append(type_args, sym(tok.text))
        }
        if self.peek().kind == PComma {
          self.next()
        }
      }
      if self.peek().kind == PGt {
        self.next()  // consume '>'
      }
    }
    // Check for :label — label can be an ident or a contextual keyword used as name
    let mut label: Sym? = null
    if self.peek().kind == PColon {
      self.next()
      let tok = self.peek()
      if tok.kind != PLBrace && tok.kind != PRBrace && tok.kind != PLBracket && tok.kind != PRBracket && tok.kind != SEOF && tok.kind != SNewline && tok.text != "" {
        label = sym(self.next().text)
      }
    }
    return RelationSide { type_name: sym(first.text), label: label, type_args: type_args }
  }

  // ---- Type expressions ----

  func Parser.parse_type_expr(self) -> (TypeExpr?, error) {
    let start = self.peek().span.start
    let left = self.parse_base_type()?

    match self.peek().kind {
      PQuestion => {
        self.next()
        return (TypeExpr {
          TypeExprKind.Optional(left!),
          span: self.make_span(start)
        }, null)
      }
      PPipe => {
        let mut variants: [TypeExpr] = [left!]
        while self.peek().kind == PPipe {
          self.next()
          let right = self.parse_base_type()?
          variants = append(variants, right!)
        }
        return (TypeExpr {
          TypeExprKind.Union(variants),
          span: self.make_span(start)
        }, null)
      }
      PArrow => {
        self.next()
        let ret = self.parse_type_expr()?
        let mut params: [TypeExpr] = []
        // If left is a tuple like (T, U), unwrap fields as params
        // If left is a single type like T, treat as single param
        match left!.kind {
          Tuple(fields) => {
            for f in fields {
              append(params, f.type_expr!)
            }
          }
          _ => {
            append(params, left!)
          }
        }
        return (TypeExpr {
          TypeExprKind.Func(params, ret!),

          span: self.make_span(start)
        }, null)
      }
      _ => {
        return (left, null)
      }
    }
  }

  func Parser.parse_base_type(self) -> (TypeExpr?, error) {
    let start = self.peek().span.start
    let tok = self.peek()

    match tok.kind {
      KFunc => {
        // fn(T, U) -> V
        self.next()
        self.expect(PLParen)?
        let mut params: [TypeExpr] = []
        self.skip_newlines()
        while self.peek().kind != PRParen && self.peek().kind != SEOF {
          let param = self.parse_type_expr()?
          params = append(params, param!)
          if self.peek().kind == PComma {
            self.next()
          }
          self.skip_newlines()
        }
        self.expect(PRParen)?
        self.expect(PArrow)?
        let ret = self.parse_type_expr()?
        return (TypeExpr {
          TypeExprKind.Func(params, ret!),
          span: self.make_span(start)
        }, null)


      }
      PLBracket => {
        // [T] — sequence
        self.next()
        let elem = self.parse_type_expr()?
        self.expect(PRBracket)?
        return (TypeExpr {
          TypeExprKind.Sequence(elem!),
          span: self.make_span(start)
        }, null)
      }
      PLParen => {
        // Tuple: (T, U)
        self.next()
        let mut fields: [TupleField] = []
        self.skip_newlines()
        while self.peek().kind != PRParen && self.peek().kind != SEOF {
          let field = self.parse_tuple_field()?
          fields = append(fields, field!)
          if self.peek().kind == PComma {
            self.next()
          }
          self.skip_newlines()
        }
        self.expect(PRParen)?
        return (TypeExpr {
          TypeExprKind.Tuple(fields),
          span: self.make_span(start)
        }, null)
      }
      LIdent => {

      let name = self.next()

      // map[K]V
      if name.text == "map" && self.peek().kind == PLBracket {
        self.next()
        let key = self.parse_type_expr()?
        self.expect(PRBracket)?
        let value = self.parse_type_expr()?
        return (TypeExpr {
          TypeExprKind.Map(key!, value!),
          span: self.make_span(start)
        }, null)
      }

      // gen T
      if name.text == "gen" {
        let elem = self.parse_base_type()?
        return (TypeExpr {
          TypeExprKind.Generator(elem!),
          span: self.make_span(start)
        }, null)
      }

      // channel<T>
      if name.text == "channel" && self.peek().kind == PLt {
        self.next()
        let elem = self.parse_type_expr()?
        self.expect(PGt)?
        return (TypeExpr {
          TypeExprKind.Channel(elem!),
          span: self.make_span(start)
        }, null)
      }

      // lock (contextual keyword — type for mutexes)
      if name.text == "lock" {
        return (TypeExpr { Lock, self.make_span(start) }, null)
      }

      // unit
      if name.text == "unit" {
        return (TypeExpr { Unit, self.make_span(start) }, null)
      }

      // Named type with optional type args
      let mut type_name = name.text

      // Dotted names: pkg.Type
      if self.peek().kind == PDot {
        self.next()
        let sub = self.expect(LIdent)?
        type_name = type_name + "." + sub!.text
      }

      let mut args: [TypeExpr] = []
      if self.peek().kind == PLt {
        self.next()
        while self.peek().kind != PGt && self.peek().kind != OShr && self.peek().kind != SEOF {
          let arg = self.parse_type_expr()?
          args = append(args, arg!)
          if self.peek().kind == PComma {
            self.next()
          }
        }
        if self.peek().kind == OShr {
          // >> is two > tokens — consume one, push back the other
          let shr_tok = self.next()
          self.push_back(Token {
            kind: PGt,
            text: ">",
            span: Span {
              start: Pos { file: shr_tok.span.start.file, line: shr_tok.span.start.line, column: shr_tok.span.start.column + 1 },
              end: shr_tok.span.end
            }
          })
        } else {
          self.expect(PGt)?
        }
      }

      return (TypeExpr {
        Named(sym(type_name), args),
        span: self.make_span(start)
      }, null)
      }
      KLock => {
        self.next()
        return (TypeExpr { TypeExprKind.Lock, self.make_span(start) }, null)
      }
      _ => {
        return (null, self.make_error(tok.span, f"expected type, got {tok.kind} ({tok.text})"))
      }
    }
  }

}
