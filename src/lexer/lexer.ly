// lexer.ly — Lexer for the Lyric bootstrap compiler
// Ported from pkg/parser/lexer.go
//
// Design:
//   - TokenKind enum (simple variants, no associated data)
//   - Token as class (heap-allocated, passed by reference)
//   - HashedList for keyword → TokenKind lookup (Sym keys, O(1) lookup)
//   - u8 character operations (ASCII only for bootstrap)
//   - ArrayList relation for collected comments

lyric lexer {

  // ---- Token kinds ----

  enum TokenKind {
    // Keywords
    KLyric KFunc KClass KStruct KEnum KInterface KRelation KField
    KDestructor KImport KImplements KImpl KExtends KAs KIs KType KWhere
    KOwns KRefs KMut KSelf KFrom KTrue KFalse KNil KPub
    // Statement keywords
    KLet KIf KElse KFor KIn KWhile KMatch KReturn KBreak KContinue
    KSpawn KSelect KCase KLock KYield
    // Literals
    LIdent LIntLit LFloatLit LStringLit LTripleString LFString LCharLit LBacktickSym
    // Punctuation
    PLParen PRParen PLBrace PRBrace PLBracket PRBracket
    PComma PColon PDot PArrow PFatArrow PBiArrow PPipe PQuestion PLt PGt
    // Operators
    OAssign OPlus OMinus OStar OSlash OPercent
    OEqEq OBangEq OLtEq OGtEq OAmpAmp OPipePipe
    OBang OAmp OCaret OShl OShr
    OPlusEq OMinusEq OStarEq OSlashEq
    // Special
    SNewline SEOF
  }

  // ---- Token ----

  permanent class Token {
    kind: TokenKind
    text: string
    span: Span
  }

  // ---- Keyword lookup (Dict-based, O(1) via hash) ----

  func init_keywords() -> Dict<Sym, TokenKind> {
    let d = Dict<Sym, TokenKind>()
    d.set(`lyric`, KLyric)
    d.set(`func`, KFunc)
    d.set(`class`, KClass)
    d.set(`struct`, KStruct)
    d.set(`enum`, KEnum)
    d.set(`interface`, KInterface)
    d.set(`relation`, KRelation)
    d.set(`destructor`, KDestructor)
    // "field" is contextual — parsed by parser.peek_annotation()
    d.set(`import`, KImport)
    // "implements" is contextual — parsed by parser.peek_annotation()
    d.set(`impl`, KImpl)
    d.set(`extends`, KExtends)
    d.set(`as`, KAs)
    d.set(`is`, KIs)
    d.set(`type`, KType)
    d.set(`where`, KWhere)
    d.set(`owns`, KOwns)
    d.set(`refs`, KRefs)
    d.set(`mut`, KMut)
    d.set(`self`, KSelf)
    d.set(`from`, KFrom)
    d.set(`true`, KTrue)
    d.set(`false`, KFalse)
    d.set(`null`, KNil)
    d.set(`let`, KLet)
    d.set(`if`, KIf)
    d.set(`else`, KElse)
    d.set(`for`, KFor)
    d.set(`in`, KIn)
    d.set(`while`, KWhile)
    d.set(`match`, KMatch)
    d.set(`return`, KReturn)
    d.set(`break`, KBreak)
    d.set(`continue`, KContinue)
    d.set(`spawn`, KSpawn)
    d.set(`select`, KSelect)
    d.set(`case`, KCase)
    // "lock" is contextual — parsed by parser.peek_annotation()
    d.set(`yield`, KYield)
    d.set(`pub`, KPub)
    return d
  }

  func lookup_keyword(lex: Lexer, text: string) -> TokenKind? {
    let entry = lex.keywords!.get(sym(text))
    if isnull(entry) {
      return null
    }
    return entry!.value
  }


  // ---- Lexer ----

  // LexerState: snapshot of lexer position for backtracking.
  // Value type — copied cheaply for save/restore.
  struct LexerState {
    pos: i32
    line: i32
    column: i32
    peeked_token: Token?
    bracket_depth: i32
  }

  permanent class Lexer {
    src: string
    filename: Sym?
    pos: i32
    line: i32 = 1
    column: i32 = 1
    peeked_token: Token?
    keywords: Dict<Sym, TokenKind>?
    bracket_depth: i32 = 0
  }
  relation ArrayList Lexer:lc owns [Comment:lc]

  func Lexer.save_state(self) -> LexerState {
    return LexerState {
      pos: self.pos,
      line: self.line,
      column: self.column,
      peeked_token: self.peeked_token,
      bracket_depth: self.bracket_depth
    }
  }

  func Lexer.restore_state(self, state: LexerState) {
    self.pos = state.pos
    self.line = state.line
    self.column = state.column
    self.peeked_token = state.peeked_token
    self.bracket_depth = state.bracket_depth
  }

  func new_lexer(src_text: string, filename: Sym) -> Lexer {
    let lex = Lexer {
      src: src_text,
      filename: filename,
      keywords: init_keywords()
    }
    return lex
  }

  // ---- Character helpers ----

  func is_letter(ch: u8) -> bool {
    return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
  }

  func is_digit(ch: u8) -> bool {
    return ch >= '0' && ch <= '9'
  }

  func is_ident_char(ch: u8) -> bool {
    return ch == '_' || is_letter(ch) || is_digit(ch)
  }

  func hex_val(ch: u8) -> u8 {
    if ch >= '0' && ch <= '9' {
      return ch - '0'
    }
    if ch >= 'a' && ch <= 'f' {
      return ch - 'a' + 10 as u8
    }
    if ch >= 'A' && ch <= 'F' {
      return ch - 'A' + 10 as u8
    }
    return 0 as u8
  }

  // ---- Token construction helpers ----

  func make_token(kind: TokenKind, text: string, start: Pos, end: Pos) -> Token {
    return Token { kind: kind, text: text, span: Span { start: start, end: end } }
  }

  // ---- Lexer methods ----

  func Lexer.current_pos(self) -> Pos {
    return Pos { file: self.filename, line: self.line, column: self.column }
  }

  func Lexer.advance(self) -> u8 {
    if self.pos >= len(self.src) {
      return 0 as u8
    }
    let ch = self.src[self.pos]
    self.pos = self.pos + 1
    if ch == '\n' {
      self.line = self.line + 1
      self.column = 1
    } else {
      self.column = self.column + 1
    }
    return ch
  }

  func Lexer.peek_char(self) -> u8 {
    if self.pos >= len(self.src) {
      return 0 as u8
    }
    return self.src[self.pos]
  }

  func Lexer.peek_at(self, offset: i32) -> u8 {
    let p = self.pos + offset
    if p >= len(self.src) {
      return 0 as u8
    }
    return self.src[p]
  }

  // Peek returns the next token without consuming it.
  func Lexer.peek(self) -> Token {
    if !isnull(self.peeked_token) {
      return self.peeked_token!
    }
    let tok = self.scan()
    self.peeked_token = tok
    return tok
  }

  // Next returns the next token, consuming it.
  func Lexer.next(self) -> Token {
    let p = self.peeked_token
    if !isnull(p) {
      self.peeked_token = null
      return p!
    }
    return self.scan()
  }

  // ---- Escape sequences ----

  // Handles a backslash escape. Backslash already consumed.
  func Lexer.scan_escape(self, buf: StringBuilder) -> bool {
    let ch = self.advance()
    if ch == 'n' {
      buf.write_byte('\n')
    } else if ch == 'r' {
      buf.write_byte('\r')
    } else if ch == 't' {
      buf.write_byte('\t')
    } else if ch == '\\' {
      buf.write_byte('\\')
    } else if ch == '\'' {
      buf.write_byte('\'')
    } else if ch == '"' {
      buf.write_byte('"')
    } else if ch == '0' {
      buf.write_byte(0 as u8)
    } else if ch == 'x' {
      let hi = self.advance()
      let lo = self.advance()
      let val = hex_val(hi) * 16 as u8 + hex_val(lo)
      buf.write_byte(val)
    } else {
      buf.write_byte('\\')
      buf.write_byte(ch)
      return false
    }
    return true
  }

  // ---- Literal scanners ----

  func Lexer.scan_char_lit(self, start: Pos) -> Token {
    self.advance()  // opening '
    let buf = new_string_builder()
    let ch = self.peek_char()
    if ch == '\\' {
      self.advance()
      self.scan_escape(buf)
    } else {
      buf.write_byte(self.advance())
    }
    if self.peek_char() == '\'' {
      self.advance()  // closing '
    }
    return make_token(LCharLit, buf.to_string(), start, self.current_pos())
  }

  func Lexer.scan_backtick_sym(self, start: Pos) -> Token {
    self.advance()  // opening `
    let buf = new_string_builder()
    while self.pos < len(self.src) {
      let ch = self.peek_char()
      if ch == '`' {
        self.advance()  // closing `
        break
      }
      if ch == '\n' {
        break  // unterminated
      }
      buf.write_byte(self.advance())
    }
    return make_token(LBacktickSym, buf.to_string(), start, self.current_pos())
  }

  func Lexer.scan_string(self, start: Pos) -> Token {
    self.advance()  // opening "

    // Check for triple-quoted string
    if self.peek_char() == '"' && self.peek_at(1) == '"' {
      self.advance()  // second "
      self.advance()  // third "
      return self.scan_triple_string(start)
    }

    let buf = new_string_builder()
    while self.pos < len(self.src) {
      let ch = self.advance()
      if ch == '"' {
        return make_token(LStringLit, buf.to_string(), start, self.current_pos())
      }
      if ch == '\\' {
        self.scan_escape(buf)
      } else {
        buf.write_byte(ch)
      }
    }
    // Unterminated string
    return make_token(LStringLit, buf.to_string(), start, self.current_pos())
  }

  func Lexer.scan_triple_string(self, start: Pos) -> Token {
    let buf = new_string_builder()
    while self.pos < len(self.src) {
      let ch = self.advance()
      if ch == '"' && self.peek_char() == '"' && self.peek_at(1) == '"' {
        self.advance()
        self.advance()
        // Trim leading/trailing whitespace lines (matches Go's strings.TrimSpace)
        return make_token(LTripleString, str_trim(buf.to_string()), start, self.current_pos())
      }
      buf.write_byte(ch)
    }
    return make_token(LTripleString, buf.to_string(), start, self.current_pos())
  }

  // Scan f"..." — stores raw content between quotes.
  // The parser splits on {expr} boundaries for interpolation.
  func Lexer.scan_fstring(self, start: Pos) -> Token {
    self.advance()  // opening "
    let buf = new_string_builder()
    let mut depth: i32 = 0
    while self.pos < len(self.src) {
      let ch = self.advance()
      if ch == '{' {
        // {{ is an escaped literal brace — store as sentinel \x01
        if depth == 0 && self.pos < len(self.src) && self.peek_char() == '{' {
          self.advance()
          buf.write_byte(1 as u8)
        } else {
          depth = depth + 1
          buf.write_byte(ch)
        }
      } else if ch == '}' {
        // }} is an escaped literal brace — store as sentinel \x02
        if depth == 0 && self.pos < len(self.src) && self.peek_char() == '}' {
          self.advance()
          buf.write_byte(2 as u8)
        } else {
          depth = depth - 1
          buf.write_byte(ch)
        }
      } else if ch == '"' && depth == 0 {
        return make_token(LFString, buf.to_string(), start, self.current_pos())
      } else if ch == '\\' && depth == 0 {
        // Escaped braces
        if self.peek_char() == '{' {
          buf.write_byte('{')
          self.advance()
        } else if self.peek_char() == '}' {
          buf.write_byte('}')
          self.advance()
        } else {
          self.scan_escape(buf)
        }
      } else {
        buf.write_byte(ch)
      }
    }
    return make_token(LFString, buf.to_string(), start, self.current_pos())
  }

  func Lexer.scan_number(self, start: Pos) -> Token {
    let buf = new_string_builder()
    let mut is_float = false
    while self.pos < len(self.src) {
      let ch = self.peek_char()
      if is_digit(ch) {
        buf.write_byte(ch)
        self.advance()
      } else if ch == '.' && !is_float {
        // Check it's not a method call
        if is_digit(self.peek_at(1)) {
          is_float = true
          buf.write_byte(ch)
          self.advance()
        } else {
          break
        }
      } else if ch == '_' {
        self.advance()  // skip underscores in number literals
      } else {
        break
      }
    }
    let mut kind = LIntLit
    if is_float {
      kind = LFloatLit
    }
    return make_token(kind, buf.to_string(), start, self.current_pos())
  }

  func Lexer.scan_ident(self, start: Pos) -> Token {
    let buf = new_string_builder()
    while self.pos < len(self.src) {
      let ch = self.peek_char()
      if is_ident_char(ch) {
        buf.write_byte(ch)
        self.advance()
      } else {
        break
      }
    }
    let text = buf.to_string()

    // f-string: f"..." triggers interpolated string scanning
    if text == "f" && self.pos < len(self.src) && self.peek_char() == '"' {
      return self.scan_fstring(start)
    }

    // Check keywords
    let kw = lookup_keyword(self, text)
    if !isnull(kw) {
      return make_token(kw!, text, start, self.current_pos())
    }

    // Annotation keywords are contextual — always lex as LIdent.
    // The parser resolves them via peekAnnotation().
    return make_token(LIdent, text, start, self.current_pos())
  }

  // ---- Main scan ----

  func Lexer.scan(self) -> Token {
    // Skip whitespace (not newlines) and line comments
    while self.pos < len(self.src) {
      let ch = self.peek_char()
      if ch == ' ' || ch == '\t' || ch == '\r' {
        self.advance()
      } else if ch == '/' && self.peek_at(1) == '/' {
        // Line comment — collect and skip to end of line
        let comment_pos = self.current_pos()
        let cbuf = new_string_builder()
        while self.pos < len(self.src) && self.peek_char() != '\n' {
          cbuf.write_byte(self.advance())
        }
        let comment = Comment { text: cbuf.to_string(), pos: comment_pos }
        array_append<Lexer, Comment>(self, comment)
      } else {
        break
      }
    }

    // EOF
    if self.pos >= len(self.src) {
      let p = self.current_pos()
      return make_token(SEOF, "", p, p)
    }

    let start = self.current_pos()
    let ch = self.peek_char()

    // Newline — collapse multiple
    if ch == '\n' {
      self.advance()
      while self.pos < len(self.src) {
        let c = self.peek_char()
        if c == '\n' {
          self.advance()
        } else if c == ' ' || c == '\t' || c == '\r' {
          // Only skip whitespace if followed by another newline
          let saved = self.pos
          while self.pos < len(self.src) {
            let w = self.peek_char()
            if w == ' ' || w == '\t' || w == '\r' {
              self.advance()
            } else {
              break
            }
          }
          if self.pos < len(self.src) && self.peek_char() == '\n' {
            self.advance()
          } else {
            self.pos = saved
            break
          }
        } else {
          break
        }
      }
      if self.bracket_depth > 0 {
        return self.scan()  // skip newline inside brackets
      }
      return make_token(SNewline, "\n", start, self.current_pos())
    }

    // String literal
    if ch == '"' {
      return self.scan_string(start)
    }

    // Character literal
    if ch == '\'' {
      return self.scan_char_lit(start)
    }

    // Backtick symbol literal: `name` → desugared to sym("name") in parser
    if ch == '`' {
      return self.scan_backtick_sym(start)
    }

    // Number
    if is_digit(ch) {
      return self.scan_number(start)
    }

    // Identifier or keyword
    if ch == '_' || is_letter(ch) {
      return self.scan_ident(start)
    }

    // Punctuation and operators
    self.advance()

    if ch == '(' {
      self.bracket_depth = self.bracket_depth + 1
      return make_token(PLParen, "(", start, self.current_pos())
    }
    if ch == ')' {
      self.bracket_depth = self.bracket_depth - 1
      return make_token(PRParen, ")", start, self.current_pos())
    }
    if ch == '{' { return make_token(PLBrace, "{", start, self.current_pos()) }
    if ch == '}' { return make_token(PRBrace, "}", start, self.current_pos()) }
    if ch == '[' {
      self.bracket_depth = self.bracket_depth + 1
      return make_token(PLBracket, "[", start, self.current_pos())
    }
    if ch == ']' {
      self.bracket_depth = self.bracket_depth - 1
      return make_token(PRBracket, "]", start, self.current_pos())
    }
    if ch == ',' { return make_token(PComma, ",", start, self.current_pos()) }
    if ch == ':' { return make_token(PColon, ":", start, self.current_pos()) }
    if ch == '.' { return make_token(PDot, ".", start, self.current_pos()) }
    if ch == '?' { return make_token(PQuestion, "?", start, self.current_pos()) }
    if ch == '^' { return make_token(OCaret, "^", start, self.current_pos()) }
    if ch == '%' { return make_token(OPercent, "%", start, self.current_pos()) }

    if ch == '!' {
      if self.peek_char() == '=' {
        self.advance()
        return make_token(OBangEq, "!=", start, self.current_pos())
      }
      return make_token(OBang, "!", start, self.current_pos())
    }

    if ch == '|' {
      if self.peek_char() == '|' {
        self.advance()
        return make_token(OPipePipe, "||", start, self.current_pos())
      }
      return make_token(PPipe, "|", start, self.current_pos())
    }

    if ch == '&' {
      if self.peek_char() == '&' {
        self.advance()
        return make_token(OAmpAmp, "&&", start, self.current_pos())
      }
      return make_token(OAmp, "&", start, self.current_pos())
    }

    if ch == '<' {
      if self.peek_char() == '=' {
        self.advance()
        return make_token(OLtEq, "<=", start, self.current_pos())
      }
      if self.peek_char() == '<' {
        self.advance()
        return make_token(OShl, "<<", start, self.current_pos())
      }
      if self.peek_char() == '-' && self.peek_at(1) == '>' {
        self.advance()
        self.advance()
        return make_token(PBiArrow, "<->", start, self.current_pos())
      }
      return make_token(PLt, "<", start, self.current_pos())
    }

    if ch == '>' {
      if self.peek_char() == '=' {
        self.advance()
        return make_token(OGtEq, ">=", start, self.current_pos())
      }
      if self.peek_char() == '>' {
        self.advance()
        return make_token(OShr, ">>", start, self.current_pos())
      }
      return make_token(PGt, ">", start, self.current_pos())
    }

    if ch == '-' {
      if self.peek_char() == '>' {
        self.advance()
        return make_token(PArrow, "->", start, self.current_pos())
      }
      if self.peek_char() == '=' {
        self.advance()
        return make_token(OMinusEq, "-=", start, self.current_pos())
      }
      return make_token(OMinus, "-", start, self.current_pos())
    }

    if ch == '+' {
      if self.peek_char() == '=' {
        self.advance()
        return make_token(OPlusEq, "+=", start, self.current_pos())
      }
      return make_token(OPlus, "+", start, self.current_pos())
    }

    if ch == '*' {
      if self.peek_char() == '=' {
        self.advance()
        return make_token(OStarEq, "*=", start, self.current_pos())
      }
      return make_token(OStar, "*", start, self.current_pos())
    }

    if ch == '/' {
      if self.peek_char() == '=' {
        self.advance()
        return make_token(OSlashEq, "/=", start, self.current_pos())
      }
      return make_token(OSlash, "/", start, self.current_pos())
    }

    if ch == '=' {
      if self.peek_char() == '=' {
        self.advance()
        return make_token(OEqEq, "==", start, self.current_pos())
      }
      if self.peek_char() == '>' {
        self.advance()
        return make_token(PFatArrow, "=>", start, self.current_pos())
      }
      return make_token(OAssign, "=", start, self.current_pos())
    }

    // Unknown character — return as ident
    return make_token(LIdent, char_to_string(ch), start, self.current_pos())
  }

}
