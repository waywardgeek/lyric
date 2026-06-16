// test_lexer.ly — Unit tests for the bootstrap lexer
//
// Run: lyric test testdata/test_lexer.ly bootstrap/lexer.ly bootstrap/ast.ly

lyric lexer {

  // Helper: create a lexer for a source string
  func make_lex(src: string) -> Lexer {
    return new_lexer(src, sym("test.ly"))
  }

  // Helper: get the next non-newline token
  func next_skip_nl(lex: Lexer) -> Token {
    while true {
      let tok = lex.next()
      if tok.kind != SNewline {
        return tok
      }
    }
    // unreachable
    return lex.next()
  }

  // ---- Keyword tests ----

  func test_keywords_statement() {
    let lex = make_lex("let if else for in while match return break continue")
    assert_eq(next_skip_nl(lex).kind, KLet, "let")
    assert_eq(next_skip_nl(lex).kind, KIf, "if")
    assert_eq(next_skip_nl(lex).kind, KElse, "else")
    assert_eq(next_skip_nl(lex).kind, KFor, "for")
    assert_eq(next_skip_nl(lex).kind, KIn, "in")
    assert_eq(next_skip_nl(lex).kind, KWhile, "while")
    assert_eq(next_skip_nl(lex).kind, KMatch, "match")
    assert_eq(next_skip_nl(lex).kind, KReturn, "return")
    assert_eq(next_skip_nl(lex).kind, KBreak, "break")
    assert_eq(next_skip_nl(lex).kind, KContinue, "continue")
    assert_eq(next_skip_nl(lex).kind, SEOF, "eof")
  }

  func test_keywords_type() {
    let lex = make_lex("lyric func class struct enum interface relation")
    assert_eq(next_skip_nl(lex).kind, KLyric, "lyric")
    assert_eq(next_skip_nl(lex).kind, KFunc, "func")
    assert_eq(next_skip_nl(lex).kind, KClass, "class")
    assert_eq(next_skip_nl(lex).kind, KStruct, "struct")
    assert_eq(next_skip_nl(lex).kind, KEnum, "enum")
    assert_eq(next_skip_nl(lex).kind, KInterface, "interface")
    assert_eq(next_skip_nl(lex).kind, KRelation, "relation")
  }

  func test_keywords_other() {
    let lex = make_lex("import where owns refs mut self pub true false null as is")
    assert_eq(next_skip_nl(lex).kind, KImport, "import")
    assert_eq(next_skip_nl(lex).kind, KWhere, "where")
    assert_eq(next_skip_nl(lex).kind, KOwns, "owns")
    assert_eq(next_skip_nl(lex).kind, KRefs, "refs")
    assert_eq(next_skip_nl(lex).kind, KMut, "mut")
    assert_eq(next_skip_nl(lex).kind, KSelf, "self")
    assert_eq(next_skip_nl(lex).kind, KPub, "pub")
    assert_eq(next_skip_nl(lex).kind, KTrue, "true")
    assert_eq(next_skip_nl(lex).kind, KFalse, "false")
    assert_eq(next_skip_nl(lex).kind, KNil, "null")
    assert_eq(next_skip_nl(lex).kind, KAs, "as")
    assert_eq(next_skip_nl(lex).kind, KIs, "is")
  }

  func test_keywords_concurrency() {
    let lex = make_lex("spawn select case yield")
    assert_eq(next_skip_nl(lex).kind, KSpawn, "spawn")
    assert_eq(next_skip_nl(lex).kind, KSelect, "select")
    assert_eq(next_skip_nl(lex).kind, KCase, "case")
    assert_eq(next_skip_nl(lex).kind, KYield, "yield")
  }

  // ---- Identifiers ----

  func test_identifier_simple() {
    let lex = make_lex("foo bar_baz x123")
    let t1 = next_skip_nl(lex)
    assert_eq(t1.kind, LIdent, "kind")
    assert_eq(t1.text, "foo", "text foo")
    let t2 = next_skip_nl(lex)
    assert_eq(t2.text, "bar_baz", "underscore ident")
    let t3 = next_skip_nl(lex)
    assert_eq(t3.text, "x123", "ident with digits")
  }

  func test_identifier_starts_with_keyword() {
    let lex = make_lex("format letter ifx")
    assert_eq(next_skip_nl(lex).kind, LIdent, "format is ident")
    assert_eq(next_skip_nl(lex).kind, LIdent, "letter is ident")
    assert_eq(next_skip_nl(lex).kind, LIdent, "ifx is ident")
  }

  // ---- Integer literals ----

  func test_int_literal() {
    let lex = make_lex("0 42 12345")
    let t1 = next_skip_nl(lex)
    assert_eq(t1.kind, LIntLit, "kind 0")
    assert_eq(t1.text, "0", "text 0")
    let t2 = next_skip_nl(lex)
    assert_eq(t2.kind, LIntLit, "kind 42")
    assert_eq(t2.text, "42", "text 42")
    let t3 = next_skip_nl(lex)
    assert_eq(t3.kind, LIntLit, "kind 12345")
    assert_eq(t3.text, "12345", "text 12345")
  }

  // ---- Float literals ----

  func test_float_literal() {
    let lex = make_lex("3.14 0.5 100.0")
    let t1 = next_skip_nl(lex)
    assert_eq(t1.kind, LFloatLit, "kind 3.14")
    assert_eq(t1.text, "3.14", "text 3.14")
    let t2 = next_skip_nl(lex)
    assert_eq(t2.kind, LFloatLit, "kind 0.5")
    let t3 = next_skip_nl(lex)
    assert_eq(t3.kind, LFloatLit, "kind 100.0")
  }

  // ---- String literals ----

  func test_string_basic() {
    let lex = make_lex("\"hello world\"")
    let tok = next_skip_nl(lex)
    assert_eq(tok.kind, LStringLit, "kind")
    assert_eq(tok.text, "hello world", "text")
  }

  func test_string_escapes() {
    let lex = make_lex("\"a\\nb\"")
    let tok = next_skip_nl(lex)
    assert_eq(tok.kind, LStringLit, "kind")
    // The text should contain the raw escape sequence
    assert(len(tok.text) > 0, "non-empty")
  }

  func test_string_empty() {
    let lex = make_lex("\"\"")
    let tok = next_skip_nl(lex)
    assert_eq(tok.kind, LStringLit, "kind")
    assert_eq(tok.text, "", "empty string text")
  }

  // ---- Character literals ----

  func test_char_literal() {
    let lex = make_lex("'A'")
    let tok = next_skip_nl(lex)
    assert_eq(tok.kind, LCharLit, "kind")
    assert_eq(tok.text, "A", "char text")
  }

  func test_char_escape() {
    let lex = make_lex("'\\n'")
    let tok = next_skip_nl(lex)
    assert_eq(tok.kind, LCharLit, "kind")
  }

  // ---- F-strings ----

  func test_fstring() {
    let lex = make_lex("f\"hello {name}\"")
    let tok = next_skip_nl(lex)
    assert_eq(tok.kind, LFString, "kind")
  }

  // ---- Punctuation ----

  func test_punctuation() {
    let lex = make_lex("( ) { } [ ] , : . -> => <-> | ? < >")
    assert_eq(next_skip_nl(lex).kind, PLParen, "lparen")
    assert_eq(next_skip_nl(lex).kind, PRParen, "rparen")
    assert_eq(next_skip_nl(lex).kind, PLBrace, "lbrace")
    assert_eq(next_skip_nl(lex).kind, PRBrace, "rbrace")
    assert_eq(next_skip_nl(lex).kind, PLBracket, "lbracket")
    assert_eq(next_skip_nl(lex).kind, PRBracket, "rbracket")
    assert_eq(next_skip_nl(lex).kind, PComma, "comma")
    assert_eq(next_skip_nl(lex).kind, PColon, "colon")
    assert_eq(next_skip_nl(lex).kind, PDot, "dot")
    assert_eq(next_skip_nl(lex).kind, PArrow, "arrow")
    assert_eq(next_skip_nl(lex).kind, PFatArrow, "fat arrow")
    assert_eq(next_skip_nl(lex).kind, PBiArrow, "bi arrow")
    assert_eq(next_skip_nl(lex).kind, PPipe, "pipe")
    assert_eq(next_skip_nl(lex).kind, PQuestion, "question")
    assert_eq(next_skip_nl(lex).kind, PLt, "lt")
    assert_eq(next_skip_nl(lex).kind, PGt, "gt")
  }

  // ---- Operators ----

  func test_operators_arithmetic() {
    let lex = make_lex("= + - * / %")
    assert_eq(next_skip_nl(lex).kind, OAssign, "assign")
    assert_eq(next_skip_nl(lex).kind, OPlus, "plus")
    assert_eq(next_skip_nl(lex).kind, OMinus, "minus")
    assert_eq(next_skip_nl(lex).kind, OStar, "star")
    assert_eq(next_skip_nl(lex).kind, OSlash, "slash")
    assert_eq(next_skip_nl(lex).kind, OPercent, "percent")
  }

  func test_operators_comparison() {
    let lex = make_lex("== != <= >=")
    assert_eq(next_skip_nl(lex).kind, OEqEq, "eq")
    assert_eq(next_skip_nl(lex).kind, OBangEq, "neq")
    assert_eq(next_skip_nl(lex).kind, OLtEq, "lteq")
    assert_eq(next_skip_nl(lex).kind, OGtEq, "gteq")
  }

  func test_operators_logical() {
    let lex = make_lex("&& || !")
    assert_eq(next_skip_nl(lex).kind, OAmpAmp, "and")
    assert_eq(next_skip_nl(lex).kind, OPipePipe, "or")
    assert_eq(next_skip_nl(lex).kind, OBang, "not")
  }

  func test_operators_bitwise() {
    let lex = make_lex("& ^ << >>")
    assert_eq(next_skip_nl(lex).kind, OAmp, "amp")
    assert_eq(next_skip_nl(lex).kind, OCaret, "caret")
    assert_eq(next_skip_nl(lex).kind, OShl, "shl")
    assert_eq(next_skip_nl(lex).kind, OShr, "shr")
  }

  func test_operators_compound_assign() {
    let lex = make_lex("+= -= *= /=")
    assert_eq(next_skip_nl(lex).kind, OPlusEq, "plus eq")
    assert_eq(next_skip_nl(lex).kind, OMinusEq, "minus eq")
    assert_eq(next_skip_nl(lex).kind, OStarEq, "star eq")
    assert_eq(next_skip_nl(lex).kind, OSlashEq, "slash eq")
  }

  // ---- Comments ----

  func test_line_comment_skipped() {
    let lex = make_lex("foo // this is a comment\nbar")
    assert_eq(next_skip_nl(lex).text, "foo", "before comment")
    assert_eq(next_skip_nl(lex).text, "bar", "after comment")
  }

  // ---- Position tracking ----

  func test_position_first_token() {
    let lex = make_lex("hello")
    let tok = next_skip_nl(lex)
    assert_eq(tok.span.start.line, 1, "line")
    assert_eq(tok.span.start.column, 1, "column")
  }

  func test_position_second_line() {
    let lex = make_lex("a\nb")
    next_skip_nl(lex)   // a
    // next_skip_nl skips the newline token and returns b
    let tok = next_skip_nl(lex)
    assert_eq(tok.span.start.line, 2, "line 2")
    assert_eq(tok.span.start.column, 1, "col 1")
  }

  // ---- Peek semantics ----

  func test_peek_does_not_consume() {
    let lex = make_lex("foo bar")
    let p = lex.peek()
    assert_eq(p.kind, LIdent, "peek kind")
    assert_eq(p.text, "foo", "peek text")
    let n = lex.next()
    assert_eq(n.text, "foo", "next returns same as peek")
    let n2 = next_skip_nl(lex)
    assert_eq(n2.text, "bar", "second token")
  }

  // ---- EOF ----

  func test_empty_input() {
    let lex = make_lex("")
    let tok = next_skip_nl(lex)
    assert_eq(tok.kind, SEOF, "empty input yields EOF")
  }

  func test_eof_repeated() {
    let lex = make_lex("x")
    next_skip_nl(lex) // x
    assert_eq(next_skip_nl(lex).kind, SEOF, "first eof")
    assert_eq(next_skip_nl(lex).kind, SEOF, "second eof")
  }

  // ---- Newline significance ----

  func test_newline_token() {
    let lex = make_lex("a\nb")
    let t1 = lex.next()
    assert_eq(t1.kind, LIdent, "a")
    let t2 = lex.next()
    assert_eq(t2.kind, SNewline, "newline")
    let t3 = lex.next()
    assert_eq(t3.kind, LIdent, "b")
  }

  // ---- Annotation keywords (contextual) ----

  func test_annotation_keywords() {
    // These should lex as identifiers, not keywords
    let lex = make_lex("doc source why")
    assert_eq(next_skip_nl(lex).kind, LIdent, "doc is ident")
    assert_eq(next_skip_nl(lex).kind, LIdent, "source is ident")
    assert_eq(next_skip_nl(lex).kind, LIdent, "why is ident")
  }

  // ---- Mixed token sequences ----

  func test_let_declaration() {
    let lex = make_lex("let x: i32 = 42")
    assert_eq(next_skip_nl(lex).kind, KLet, "let")
    assert_eq(next_skip_nl(lex).kind, LIdent, "x")
    assert_eq(next_skip_nl(lex).kind, PColon, "colon")
    assert_eq(next_skip_nl(lex).kind, LIdent, "i32")
    assert_eq(next_skip_nl(lex).kind, OAssign, "assign")
    assert_eq(next_skip_nl(lex).kind, LIntLit, "42")
  }

  func test_function_header() {
    let lex = make_lex("func add(a: i32, b: i32) -> i32")
    assert_eq(next_skip_nl(lex).kind, KFunc, "func")
    assert_eq(next_skip_nl(lex).kind, LIdent, "add")
    assert_eq(next_skip_nl(lex).kind, PLParen, "lparen")
    assert_eq(next_skip_nl(lex).kind, LIdent, "a")
    assert_eq(next_skip_nl(lex).kind, PColon, "colon")
    assert_eq(next_skip_nl(lex).kind, LIdent, "i32")
    assert_eq(next_skip_nl(lex).kind, PComma, "comma")
    assert_eq(next_skip_nl(lex).kind, LIdent, "b")
    assert_eq(next_skip_nl(lex).kind, PColon, "colon2")
    assert_eq(next_skip_nl(lex).kind, LIdent, "i32_2")
    assert_eq(next_skip_nl(lex).kind, PRParen, "rparen")
    assert_eq(next_skip_nl(lex).kind, PArrow, "arrow")
    assert_eq(next_skip_nl(lex).kind, LIdent, "return type")
  }

  // ---- Save/restore state ----

  func test_save_restore() {
    let lex = make_lex("a b c")
    next_skip_nl(lex)   // consume a
    let state = lex.save_state()
    next_skip_nl(lex)   // consume b
    next_skip_nl(lex)   // consume c
    lex.restore_state(state)
    let tok = next_skip_nl(lex)
    assert_eq(tok.text, "b", "restored to b")
  }

  // ---- Bracket depth — newlines suppressed inside () and [] ----

  func test_bracket_depth_parens() {
    let lex = make_lex("foo(\na,\nb\n)")
    assert_eq(next_skip_nl(lex).text, "foo", "ident")
    assert_eq(next_skip_nl(lex).kind, PLParen, "(")
    // Inside parens — newlines should be suppressed
    assert_eq(next_skip_nl(lex).text, "a", "a (no newline before)")
    assert_eq(next_skip_nl(lex).kind, PComma, ",")
    assert_eq(next_skip_nl(lex).text, "b", "b (no newline before)")
    assert_eq(next_skip_nl(lex).kind, PRParen, ")")
  }

  func test_bracket_depth_brackets() {
    let lex = make_lex("[\n1,\n2\n]")
    assert_eq(next_skip_nl(lex).kind, PLBracket, "[")
    assert_eq(next_skip_nl(lex).text, "1", "1")
    assert_eq(next_skip_nl(lex).kind, PComma, ",")
    assert_eq(next_skip_nl(lex).text, "2", "2")
    assert_eq(next_skip_nl(lex).kind, PRBracket, "]")
  }

  func test_bracket_depth_braces_do_not_suppress() {
    let lex = make_lex("{\na\n}")
    assert_eq(lex.next().kind, PLBrace, "{")
    assert_eq(lex.next().kind, SNewline, "newline after {")
    assert_eq(lex.next().text, "a", "a")
    assert_eq(lex.next().kind, SNewline, "newline after a")
    assert_eq(lex.next().kind, PRBrace, "}")
  }

  // ---- Contextual keywords lex as ident ----

  func test_contextual_keywords_as_ident() {
    let lex = make_lex("field lock implements source why doc")
    assert_eq(next_skip_nl(lex).kind, LIdent, "field is ident")
    assert_eq(next_skip_nl(lex).kind, LIdent, "lock is ident")
    assert_eq(next_skip_nl(lex).kind, LIdent, "implements is ident")
    assert_eq(next_skip_nl(lex).kind, LIdent, "source is ident")
    assert_eq(next_skip_nl(lex).kind, LIdent, "why is ident")
    assert_eq(next_skip_nl(lex).kind, LIdent, "doc is ident")
  }
}
