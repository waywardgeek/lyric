// expr_parser.ly — Pratt parser for expressions, statements, and patterns
// Ported from pkg/parser/expr_parser.go

lyric parser {

  // ---- Precedence levels ----

  let PREC_NONE: i32       = 0
  let PREC_OR: i32         = 1   // ||
  let PREC_AND: i32        = 2   // &&
  let PREC_BIT_OR: i32     = 3   // |
  let PREC_BIT_XOR: i32    = 4   // ^
  let PREC_BIT_AND: i32    = 5   // &
  let PREC_EQUALITY: i32   = 6   // == !=
  let PREC_COMPARISON: i32 = 7   // < > <= >=
  let PREC_SHIFT: i32      = 8   // << >>
  let PREC_ADDITIVE: i32   = 9   // + -
  let PREC_MULT: i32       = 10  // * / %
  let PREC_UNARY: i32      = 11  // - !
  let PREC_POSTFIX: i32    = 12  // . () []

  func binary_prec(kind: TokenKind) -> i32 {
    return match kind {
      OPipePipe => { PREC_OR }
      OAmpAmp   => { PREC_AND }
      PPipe     => { PREC_BIT_OR }
      OCaret    => { PREC_BIT_XOR }
      OAmp      => { PREC_BIT_AND }
      OEqEq     => { PREC_EQUALITY }
      OBangEq   => { PREC_EQUALITY }
      PLt       => { PREC_COMPARISON }
      PGt       => { PREC_COMPARISON }
      OLtEq     => { PREC_COMPARISON }
      OGtEq     => { PREC_COMPARISON }
      OShl      => { PREC_SHIFT }
      OShr      => { PREC_SHIFT }
      OPlus     => { PREC_ADDITIVE }
      OMinus    => { PREC_ADDITIVE }
      OStar     => { PREC_MULT }
      OSlash    => { PREC_MULT }
      OPercent  => { PREC_MULT }
      _         => { PREC_NONE }
    }
  }

  func token_to_binary_op(kind: TokenKind) -> BinaryOp {
    return match kind {
      OPlus     => { Add }
      OMinus    => { Sub }
      OStar     => { Mul }
      OSlash    => { Div }
      OPercent  => { Mod }
      OEqEq     => { Eq }
      OBangEq   => { Neq }
      PLt       => { Lt }
      OLtEq     => { Le }
      PGt       => { Gt }
      OGtEq     => { Ge }
      OAmpAmp   => { And }
      OPipePipe => { Or }
      OAmp      => { BitAnd }
      PPipe     => { BitOr }
      OCaret    => { BitXor }
      OShl      => { Shl }
      OShr      => { Shr }
      _         => { Add }
    }
  }

  func is_ident_kind(kind: ExprKind) -> bool {
    return match kind {
      Ident(_) => { true }
      _ => { false }
    }
  }

  // ---- Expression parsing (Pratt) ----

  func Parser.parse_expr(self) -> (Expr?, error) {
    return self.parse_prec_expr(PREC_NONE + 1)
  }

  // parse_expr_no_struct_lit parses an expression but suppresses struct literal
  // parsing. Used in for/while/if/match where the expression precedes a
  // mandatory block, so `Ident {` must be treated as variable + block start.
  func Parser.parse_expr_no_struct_lit(self) -> (Expr?, error) {
    let old = self.no_struct_lit
    self.no_struct_lit = true
    let result = self.parse_prec_expr(PREC_NONE + 1)
    self.no_struct_lit = old
    return result
  }

  func Parser.parse_prec_expr(self, min_prec: i32) -> (Expr?, error) {
    let mut left = self.parse_unary_expr()?

    while true {
      let prec = binary_prec(self.peek().kind)
      if prec < min_prec {
        break
      }
      let op = self.next()
      let right = self.parse_prec_expr(prec + 1)?
      left = Expr {
        kind: Binary(left!, token_to_binary_op(op.kind), right!),
        span: Span { start: left!.span.start, end: right!.span.end }
      }
    }
    return (left, null)
  }

  func Parser.parse_unary_expr(self) -> (Expr?, error) {
    let tok = self.peek()
    match tok.kind {
      OBang => {
        self.next()
        let operand = self.parse_unary_expr()?
        return (Expr {
          Unary(Not, operand!),
          span: Span { start: tok.span.start, end: operand!.span.end }
        }, null)
      }
      OMinus => {
        self.next()
        let operand = self.parse_unary_expr()?
        return (Expr {
          Unary(Neg, operand!),
          span: Span { start: tok.span.start, end: operand!.span.end }
        }, null)
      }
      _ => {
        return self.parse_postfix_expr()
      }
    }
  }

  // ---- Postfix expressions ----

  func Parser.parse_postfix_expr(self) -> (Expr?, error) {
    let mut expr = self.parse_primary_expr()?

    while true {
      match self.peek().kind {
        PDot => {
          self.next()
          let name = self.expect_ident()?
          match self.peek().kind {
            PLParen => {
              // obj.method(args)
              self.next()
              let args = self.parse_arg_list()?
              expr = Expr {
                kind: MethodCall(expr!, sym(name!.text), [], args!.elems, args!.mut_args),
                span: Span { start: expr!.span.start, end: args!.end_pos }
              }
            }
            PLt => {
              // Try obj.method<T>(args) — backtrack if not
              let saved = self.lex!.save_state()
              let type_args = self.try_parse_type_args()
              if type_args != null && self.peek().kind == PLParen {
                self.next()
                let args = self.parse_arg_list()?
                expr = Expr {
                  kind: MethodCall(expr!, sym(name!.text), type_args!, args!.elems, args!.mut_args),
                  span: Span { start: expr!.span.start, end: args!.end_pos }
                }
              } else {
                self.lex!.restore_state(saved)
                expr = Expr {
                  kind: FieldAccess(expr!, sym(name!.text)),
                  span: Span { start: expr!.span.start, end: name!.span.end }
                }
              }
            }
            PLBrace => {
              // Qualified struct literal: pkg.Type{} — only if expr is ident
              if is_ident_kind(expr!.kind) && (self.is_struct_lit_ahead() || self.expr_depth > 0) {
                let ident_name = match expr!.kind { Ident(n) => { n } _ => { sym("") } }
                let qual_name = sym(ident_name.get_name() + "." + name!.text)
                expr = self.parse_struct_lit(qual_name, [], expr!.span.start)?
              } else {
                expr = Expr {
                  kind: FieldAccess(expr!, sym(name!.text)),
                  span: Span { start: expr!.span.start, end: name!.span.end }
                }
              }
            }
            _ => {
              expr = Expr {
                kind: FieldAccess(expr!, sym(name!.text)),
                span: Span { start: expr!.span.start, end: name!.span.end }
              }
            }
          }
        }
        PLt => {
          // Try generic call: expr<TypeArgs>(args)
          let saved = self.lex!.save_state()
          let type_args = self.try_parse_type_args()
          if type_args != null && self.peek().kind == PLParen {
            self.next()
            let args = self.parse_arg_list()?
            expr = Expr {
              kind: Call(expr!, type_args!, args!.elems, args!.mut_args),
              span: Span { start: expr!.span.start, end: args!.end_pos }
            }
          } else if type_args != null && self.peek().kind == PLBrace && is_ident_kind(expr!.kind) && (self.is_struct_lit_ahead() || self.expr_depth > 0) {
            // Generic struct literal: TypeName<T> { ... }
            let ident_name = match expr!.kind { Ident(n) => { n } _ => { sym("") } }
            expr = self.parse_struct_lit(ident_name, type_args!, expr!.span.start)?
          } else {
            self.lex!.restore_state(saved)
            return (expr, null)
          }
        }
        PLParen => {
          self.next()
          let args = self.parse_arg_list()?
          expr = Expr {
            kind: Call(expr!, [], args!.elems, args!.mut_args),
            span: Span { start: expr!.span.start, end: args!.end_pos }
          }
        }
        PLBracket => {
          self.next()
          if self.peek().kind == PColon {
            // Slice with no low bound: [:high] or [:]
            self.next()
            let mut high: Expr? = null
            if self.peek().kind != PRBracket {
              high = self.parse_expr()?
            }
            let end = self.expect(PRBracket)?
            expr = Expr {
              kind: Slice(expr!, null, high),
              span: Span { start: expr!.span.start, end: end!.span.end }
            }
          } else {
            let index = self.parse_expr()?
            if self.peek().kind == PColon {
              // Slice: [low:high] or [low:]
              self.next()
              let mut high: Expr? = null
              if self.peek().kind != PRBracket {
                high = self.parse_expr()?
              }
              let end = self.expect(PRBracket)?
              expr = Expr {
                kind: Slice(expr!, index, high),
                span: Span { start: expr!.span.start, end: end!.span.end }
              }
            } else {
              let end = self.expect(PRBracket)?
              expr = Expr {
                kind: Index(expr!, index!),
                span: Span { start: expr!.span.start, end: end!.span.end }
              }
            }
          }
        }
        OBang => {
          let bang = self.next()
          expr = Expr {
            kind: Unwrap(expr!),
            span: Span { start: expr!.span.start, end: bang.span.end }
          }
        }
        PQuestion => {
          let q = self.next()
          expr = Expr {
            kind: Try(expr!),
            span: Span { start: expr!.span.start, end: q.span.end }
          }
        }
        KIs => {
          // Postfix is — variant type check: expr is VariantName
          self.next()
          let variant = self.expect_ident()?
          expr = Expr {
            kind: Is(expr!, sym(variant!.text)),
            span: Span { start: expr!.span.start, end: variant!.span.end }
          }
        }
        KAs => {
          // Postfix as — type cast: expr as Type
          self.next()
          let target_type = self.parse_type_expr()?
          expr = Expr {
            kind: Cast(target_type!, expr!),
            span: Span { start: expr!.span.start, end: target_type!.span.end }
          }
        }
        _ => {
          return (expr, null)
        }
      }
    }
    return (expr, null)
  }

  // ---- Argument list helper ----

  // Returns parsed args and end position
  struct ArgListResult {
    elems: [Expr]
    mut_args: [bool]
    end_pos: Pos
  }

  func Parser.parse_arg_list(self) -> (ArgListResult?, error) {
    self.expr_depth = self.expr_depth + 1
    let mut args: [Expr] = []
    let mut mut_flags: [bool] = []
    let mut has_mut: bool = false
    while self.peek().kind != PRParen && self.peek().kind != SEOF {
      self.skip_newlines()
      let mut is_mut: bool = false
      if self.peek().kind == KMut {
        is_mut = true
        has_mut = true
        self.next()
      }
      let arg = self.parse_expr()?
      args = append(args, arg!)
      mut_flags = append(mut_flags, is_mut)
      if self.peek().kind == PComma {
        self.next()
      }
      self.skip_newlines()
    }
    let end = self.expect(PRParen)?
    let mut result_mut_args: [bool] = []
    if has_mut {
      result_mut_args = mut_flags
    }
    self.expr_depth = self.expr_depth - 1
    return (ArgListResult { args, result_mut_args, end!.span.end }, null)
  }

  // ---- Primary expressions ----

  func Parser.parse_primary_expr(self) -> (Expr?, error) {
    let tok = self.peek()
    match tok.kind {
      LIntLit => {
        self.next()
        return (Expr { IntLit(tok.text, null), span: tok.span }, null)
      }
      LCharLit => {
        self.next()
        let mut val: i32 = 0
        if len(tok.text) > 0 {
          val = tok.text[0] as i32
        }
        return (Expr { IntLit(itoa(val), `u8`), span: tok.span }, null)
      }
      LFloatLit => {
        self.next()
        return (Expr { FloatLit(tok.text), span: tok.span }, null)
      }
      LStringLit | LTripleString => {
        self.next()
        return (Expr { StringLit(tok.text), span: tok.span }, null)
      }
      LFString => {
        self.next()
        return self.parse_fstring(tok)
      }
      LBacktickSym => {
        self.next()
        // Desugar `name` → sym("name")
        let sym_name = sym("sym")
        let sym_ident = Expr { kind: ExprKind.Ident(sym_name), span: tok.span }
        let str_arg = Expr { kind: StringLit(tok.text), span: tok.span }
        let args: [Expr] = [str_arg]
        let type_args: [TypeExpr] = []
        let mut_args: [bool] = []
        return (Expr { kind: Call(sym_ident, type_args, args, mut_args), span: tok.span }, null)
      }
      KTrue => {
        self.next()
        return (Expr { BoolLit(true), span: tok.span }, null)
      }
      KFalse => {
        self.next()
        return (Expr { BoolLit(false), span: tok.span }, null)
      }
      KNil => {
        self.next()
        return (Expr { ExprKind.Nil, tok.span }, null)
      }
      LIdent => {
        self.next()
        let name = tok.text
        // Check for struct literal: TypeName { field: value }
        if self.peek().kind == PLBrace && (self.is_struct_lit_ahead() || self.expr_depth > 0) {
          return self.parse_struct_lit(sym(name), [], tok.span.start)
        }
        // Check for map literal: map[K]V { ... }
        if name == "map" && self.peek().kind == PLBracket {
          return self.parse_map_lit(tok)
        }
        return (Expr { ExprKind.Ident(sym(name)), span: tok.span }, null)
      }
      KSelf => {
        self.next()
        return (Expr { ExprKind.Ident(`self`), span: tok.span }, null)
      }
      PLParen => {
        // Check for paren-lambda: (params) -> RetType { body }
        if self.is_paren_lambda_ahead() {
          return self.parse_paren_lambda(tok.span.start)
        }
        // Parenthesized expr or tuple literal
        self.next()
        self.expr_depth = self.expr_depth + 1
        if self.peek().kind == PRParen {
          let end = self.next()
          self.expr_depth = self.expr_depth - 1
          return (Expr { TupleLit([]), span: Span { start: tok.span.start, end: end.span.end } }, null)
        }
        let first = self.parse_expr()?
        if self.peek().kind == PComma {
          // Tuple
          let mut elems: [Expr] = [first!]
          while self.peek().kind == PComma {
            self.next()
            if self.peek().kind == PRParen { break }
            let elem = self.parse_expr()?
            elems = append(elems, elem!)
          }
          let end = self.expect(PRParen)?
          self.expr_depth = self.expr_depth - 1
          return (Expr { TupleLit(elems), span: Span { start: tok.span.start, end: end!.span.end } }, null)
        }
        // Parenthesized expression
        self.expect(PRParen)?
        self.expr_depth = self.expr_depth - 1
        return (first, null)
      }
      PLBracket => {
        // List literal: [1, 2, 3]
        self.next()
        self.expr_depth = self.expr_depth + 1
        let mut elems: [Expr] = []
        while self.peek().kind != PRBracket && self.peek().kind != SEOF {
          self.skip_newlines()
          let elem = self.parse_expr()?
          elems = append(elems, elem!)
          if self.peek().kind == PComma {
            self.next()
          }
          self.skip_newlines()
        }
        let end = self.expect(PRBracket)?
        self.expr_depth = self.expr_depth - 1
        return (Expr { ListLit(elems), span: Span { start: tok.span.start, end: end!.span.end } }, null)
      }
      KMatch => {
        return self.parse_match_expr()
      }
      PPipe => {
        return self.parse_lambda_expr()
      }
      KIf => {
        return self.parse_if_expr()
      }
      PLBrace => {
        // Dict literal: { "key": val, ... } or {}
        // tok = { (peeked but NOT yet consumed)
        // Lookahead: consume {, skip newlines, check what follows
        let saved = self.lex!.save_state()
        self.next()  // consume {
        self.skip_newlines()
        let next = self.peek()
        if next.kind == PRBrace {
          // Empty dict literal {}
          let end = self.next()
          return (Expr {
            MapLit([], []),
            span: Span { start: tok.span.start, end: end.span.end }
          }, null)
        }
        if next.kind == LStringLit || next.kind == LBacktickSym || next.kind == LIntLit {
          let saved2 = self.lex!.save_state()
          self.next()  // consume key token to peek at token after it
          let after = self.peek()
          self.lex!.restore_state(saved2)
          if after.kind == PColon {
            // Key literal followed by : → dict literal
            return self.parse_dict_lit(tok)
          }
        }
        // Not a dict literal — restore state (un-consume {) and return error
        self.lex!.restore_state(saved)
        return (null, self.make_error(tok.span, f"expected expression, got {tok.kind} ({tok.text})"))
      }
      _ => {
        return (null, self.make_error(tok.span, f"expected expression, got {tok.kind} ({tok.text})"))
      }
    }
  }

  // ---- Dict literal: { "key": val, ... } ----
  // Called after { has been consumed (tok = the { token)

  func Parser.parse_dict_lit(self, lbrace: Token) -> (Expr?, error) {
    self.skip_newlines()
    let mut keys: [Expr] = []
    let mut values: [Expr] = []
    while self.peek().kind != PRBrace && self.peek().kind != SEOF {
      let key = self.parse_expr()?
      self.expect(PColon)?
      let val = self.parse_expr()?
      keys = append(keys, key!)
      values = append(values, val!)
      if self.peek().kind == PComma {
        self.next()
      }
      self.skip_newlines()
    }
    let end = self.expect(PRBrace)?
    return (Expr {
      MapLit(keys, values),
      span: Span { start: lbrace.span.start, end: end!.span.end }
    }, null)
  }

  // ---- Map literal (legacy: map[K]V { ... }) ----

  func Parser.parse_map_lit(self, lbrace: Token) -> (Expr?, error) {
    // Already consumed "map", peek is [
    self.next()  // [
    let mut depth: i32 = 1
    while depth > 0 && self.peek().kind != SEOF {
      match self.peek().kind {
        PLBracket => { depth = depth + 1
          self.next() }
        PRBracket => { depth = depth - 1
          self.next() }
        _ => { self.next() }
      }
    }
    // Consume value type until {
    while self.peek().kind != PLBrace && self.peek().kind != SEOF {
      self.next()
    }
    self.expect(PLBrace)?
    self.skip_newlines()
    let mut keys: [Expr] = []
    let mut values: [Expr] = []
    while self.peek().kind != PRBrace && self.peek().kind != SEOF {
      let key = self.parse_expr()?
      self.expect(PColon)?
      let val = self.parse_expr()?
      keys = append(keys, key!)
      values = append(values, val!)
      if self.peek().kind == PComma {
        self.next()
      }
      self.skip_newlines()
    }
    let end = self.expect(PRBrace)?
    return (Expr {
      MapLit(keys, values),
      span: Span { start: lbrace.span.start, end: end!.span.end }
    }, null)
  }

  // ---- Match expression ----

  func Parser.parse_match_expr(self) -> (Expr?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'match'
    let value = self.parse_expr_no_struct_lit()?
    self.expect(PLBrace)?
    self.skip_newlines()
    let arms = self.parse_match_arms()?
    let end = self.peek().span.end
    return (Expr {
      ExprKind.Match(value!, arms),

      span: Span { start: start, end: end }
    }, null)
  }

  // ---- If expression ----

  func Parser.parse_if_expr(self) -> (Expr?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'if'
    let cond = self.parse_expr_no_struct_lit()?
    let then_block = self.parse_block()?

    let mut else_ifs: [ElseIf] = []
    let mut else_block: Block? = null

    while self.peek().kind == KElse {
      self.next()
      if self.peek().kind == KIf {
        self.next()
        let elif_cond = self.parse_expr_no_struct_lit()?
        let elif_body = self.parse_block()?
        else_ifs = append(else_ifs, ElseIf { elif_cond, elif_body })
      } else {
        else_block = self.parse_block()?
        break
      }
    }

    if else_block == null {
      return (null, self.make_error(Span { start: start, end: start }, "if expression requires an else branch"))
    }

    return (Expr {
      IfElse(cond!, then_block!, else_ifs, else_block!),
      span: Span { start: start, end: else_block!.span.end }
    }, null)
  }

  // ---- Lambda expression ----

  func Parser.parse_lambda_expr(self) -> (Expr?, error) {
    let start = self.peek().span.start
    self.next()  // consume opening |
    let mut params: [Param] = []
    while self.peek().kind != PPipe && self.peek().kind != SEOF {
      let name_tok = self.expect_ident()?
      self.expect(PColon)?
      let typ = self.parse_base_type()?
      let mut final_type = typ
      if self.peek().kind == PQuestion {
        self.next()
        final_type = TypeExpr {
          kind: TypeExprKind.Optional(typ!),
          span: typ!.span
        }
      }
      params = append(params, Param {
        name: sym(name_tok!.text),
        type_expr: final_type,
        is_mut: false,
        is_self: false,
        span: name_tok!.span
      })
      if self.peek().kind == PComma {
        self.next()
      }
    }
    self.expect(PPipe)?

    let mut ret_type: TypeExpr? = null
    if self.peek().kind == PArrow {
      self.next()
      ret_type = self.parse_type_expr()?
    }

    let body = self.parse_block()?
    return (Expr {
      Lambda(params, ret_type, body!),
      span: Span { start: start, end: body!.span.end }
    }, null)
  }

  // ---- Paren-lambda: (params) -> RetType { body } ----

  func Parser.is_paren_lambda_ahead(self) -> bool {
    let saved = self.lex!.save_state()
    let pushed_saved = self.pushed
    self.next()  // consume (
    self.skip_newlines()
    let mut looks_like_lambda = false
    if self.peek().kind == PRParen {
      // () -> ... is a zero-param lambda
      self.next()
      if self.peek().kind == PArrow {
        looks_like_lambda = true
      }
    } else if self.peek().kind == LIdent {
      self.next()
      if self.peek().kind == PColon {
        // (ident: — looks like param, skip to ) and check for ->
        let mut depth: i32 = 1
        while depth > 0 && self.peek().kind != SEOF {
          if self.peek().kind == PLParen { depth = depth + 1 }
          if self.peek().kind == PRParen { depth = depth - 1 }
          if depth > 0 { self.next() }
        }
        if self.peek().kind == PRParen {
          self.next()
          if self.peek().kind == PArrow {
            looks_like_lambda = true
          }
        }
      }
    }
    self.lex!.restore_state(saved)
    self.pushed = pushed_saved
    return looks_like_lambda
  }

  func Parser.parse_paren_lambda(self, start: Pos) -> (Expr?, error) {
    self.next()  // consume (
    let mut params: [Param] = []
    self.skip_newlines()
    while self.peek().kind != PRParen && self.peek().kind != SEOF {
      let param = self.parse_param()?
      params = append(params, param!)
      if self.peek().kind == PComma {
        self.next()
      }
      self.skip_newlines()
    }
    self.expect(PRParen)?

    let mut ret_type: TypeExpr? = null
    if self.peek().kind == PArrow {
      self.next()
      ret_type = self.parse_type_expr()?
    }

    let body = self.parse_block()?
    return (Expr {
      Lambda(params, ret_type, body!),
      span: Span { start: start, end: body!.span.end }
    }, null)
  }

  // ---- Block parsing ----

  func Parser.parse_block(self) -> (Block?, error) {
    let start = self.peek().span.start
    self.expect(PLBrace)?
    self.skip_newlines()
    let block = Block { span: Span { start: start, end: start } }
    while self.peek().kind != PRBrace && self.peek().kind != SEOF {
      let stmt = self.parse_stmt()?
      array_append<Block, Stmt>(block, stmt!)
      self.skip_newlines()
    }
    let end = self.expect(PRBrace)?
    block.span = Span { start: start, end: end!.span.end }
    return (block, null)
  }

  // ---- Statement parsing ----

  func Parser.parse_stmt(self) -> (Stmt?, error) {
    let tok = self.peek()
    match tok.kind {
      KLet      => { return self.parse_var_decl() }
      KReturn   => { return self.parse_return() }
      KIf       => { return self.parse_if() }
      KFor      => { return self.parse_for() }
      KWhile    => { return self.parse_while() }
      KMatch    => { return self.parse_match_stmt() }
      KBreak    => { self.next()
        return (Stmt { Break, tok.span }, null) }
      KContinue => { self.next()
        return (Stmt { Continue, tok.span }, null) }
      KCascade  => { return self.parse_cascade() }
      KSpawn    => { return self.parse_spawn() }
      KSelect   => { return self.parse_select() }
      KLock     => { return self.parse_lock() }
      KYield    => { return self.parse_yield() }
      PLBrace   => {
        let blk = self.parse_block()?
        return (Stmt { BlockStmt(blk!), span: blk!.span }, null)
      }
      _ => {
        // Check contextual keywords that the lexer emits as LIdent
        let ann = self.peek_annotation()
        if !isnull(ann) {
          match ann! {
            KLock => { return self.parse_lock() }
            _ => {}
          }
        }
        return self.parse_expr_or_assign()
      }
    }
  }

  // ---- Variable declaration ----

  func Parser.parse_var_decl(self) -> (Stmt?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'let'
    let mut is_mut = false
    if self.peek().kind == KMut {
      is_mut = true
      self.next()
    }

    // Check for pattern let: let Variant(x, y) = expr else { ... }
    // Detected by: uppercase Ident followed by '('
    if self.is_pattern_let_ahead() {
      return self.parse_let_else(start, is_mut)
    }

    // Tuple destructuring: let (a, b) = expr
    if self.peek().kind == PLParen {
      self.next()
      let mut names: [Sym] = []
      while true {
        let name = self.expect_ident()?
        names = append(names, sym(name!.text))
        if self.peek().kind == PComma {
          self.next()
        } else {
          break
        }
      }
      self.expect(PRParen)?
      self.expect(OAssign)?
      let val = self.parse_expr()?
      return (Stmt {
        VarDecl(sym(""), names, null, is_mut, val),
        span: Span { start: start, end: self.peek().span.start }
      }, null)
    }

    let name = self.expect_ident()?

    // Optional type annotation
    let mut type_ann: TypeExpr? = null
    if self.peek().kind == PColon {
      self.next()
      type_ann = self.parse_type_expr()?
    }

    // Optional initializer
    let mut val: Expr? = null
    if self.peek().kind == OAssign {
      self.next()
      val = self.parse_expr()?
    }

    return (Stmt {
      VarDecl(sym(name!.text), [], type_ann, is_mut, val),
      span: Span { start: start, end: self.peek().span.start }
    }, null)
  }

  // ---- Pattern Let helpers ----

  func Parser.is_pattern_let_ahead(self) -> bool {
    let tok = self.peek()
    if tok.kind != LIdent { return false }
    if len(tok.text) == 0 { return false }
    let first = tok.text[0] as i32
    if first < 65 || first > 90 { return false }  // Not uppercase A-Z
    // Save state and check for '(' after ident
    let saved = self.lex!.save_state()
    let pushed_saved = self.pushed
    self.next()  // consume ident
    let is_variant = self.peek().kind == PLParen
    self.lex!.restore_state(saved)
    self.pushed = pushed_saved
    return is_variant
  }

  func Parser.parse_let_else(self, start: Pos, is_mut: bool) -> (Stmt?, error) {
    let pat = self.parse_pattern()?
    self.expect(OAssign)?
    let val = self.parse_expr()?
    self.expect(KElse)?
    let else_block = self.parse_block()?
    return (Stmt {
      LetElse(pat!, val!, else_block!),
      span: Span { start: start, end: self.peek().span.start }
    }, null)
  }

  // ---- Return ----

  func Parser.parse_return(self) -> (Stmt?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'return'
    let mut val: Expr? = null
    if self.peek().kind != SNewline && self.peek().kind != SEOF && self.peek().kind != PRBrace {
      val = self.parse_expr()?
    }
    return (Stmt {
      Return(val),
      span: Span { start: start, end: self.peek().span.start }
    }, null)
  }

  // ---- Yield ----

  func Parser.parse_yield(self) -> (Stmt?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'yield'
    let val = self.parse_expr()?
    return (Stmt {
      Yield(val),
      span: Span { start: start, end: self.peek().span.start }
    }, null)
  }

  // ---- If ----

  func Parser.parse_if(self) -> (Stmt?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'if'

    // Check for 'if let Pattern = expr { ... }'
    if self.peek().kind == KLet {
      return self.parse_if_let(start)
    }

    let cond = self.parse_expr_no_struct_lit()?
    let then_block = self.parse_block()?

    let mut else_ifs: [ElseIf] = []
    let mut else_block: Block? = null

    while self.peek().kind == KElse {
      self.next()
      if self.peek().kind == KIf {
        self.next()
        let elif_cond = self.parse_expr_no_struct_lit()?
        let elif_body = self.parse_block()?
        else_ifs = append(else_ifs, ElseIf {
          elif_cond,
          elif_body,
          elif_body!.span
        })
      } else {
        else_block = self.parse_block()?
        break
      }
    }

    return (Stmt {
      If(cond!, then_block!, else_ifs, else_block),
      span: Span { start: start, end: self.peek().span.start }
    }, null)
  }

  // ---- If Let ----

  func Parser.parse_if_let(self, start: Pos) -> (Stmt?, error) {
    self.next()  // consume 'let'
    let pat = self.parse_pattern()?
    self.expect(OAssign)?
    let value = self.parse_expr_no_struct_lit()?
    let then_block = self.parse_block()?

    let mut else_block: Block? = null
    if self.peek().kind == KElse {
      self.next()
      else_block = self.parse_block()?
    }

    return (Stmt {
      IfLet(pat!, value!, then_block!, else_block),
      span: Span { start: start, end: self.peek().span.start }
    }, null)
  }

  // ---- For ----

  func Parser.parse_for(self) -> (Stmt?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'for'
    let var_name = self.expect_ident()?

    let mut index_var: Sym? = null
    if self.peek().kind == PComma {
      self.next()
      index_var = sym(var_name!.text)
      let actual_var = self.expect_ident()?
      // Swap: index_var has first name, var_name gets second
      self.expect(KIn)?
      let collection = self.parse_expr_no_struct_lit()?
      let body = self.parse_block()?
      return (Stmt {
        For(sym(actual_var!.text), index_var, collection!, body!),
        span: Span { start: start, end: body!.span.end }
      }, null)
    }

    self.expect(KIn)?
    let collection = self.parse_expr_no_struct_lit()?
    let body = self.parse_block()?
    return (Stmt {
      For(sym(var_name!.text), null, collection!, body!),
      span: Span { start: start, end: body!.span.end }
    }, null)
  }

  // ---- While ----

  func Parser.parse_while(self) -> (Stmt?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'while'
    let cond = self.parse_expr_no_struct_lit()?
    let body = self.parse_block()?
    return (Stmt {
      While(cond!, body!),
      span: Span { start: start, end: body!.span.end }
    }, null)
  }

  // ---- Match statement ----

  func Parser.parse_match_stmt(self) -> (Stmt?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'match'
    let value = self.parse_expr_no_struct_lit()?
    self.expect(PLBrace)?
    self.skip_newlines()
    let arms = self.parse_match_arms()?
    return (Stmt {
      StmtKind.Match(value!, arms),
      span: Span { start: start, end: self.peek().span.start }
    }, null)

  }

  func Parser.parse_match_arms(self) -> ([MatchArm], error) {
    let mut arms: [MatchArm] = []
    while self.peek().kind != PRBrace && self.peek().kind != SEOF {
      let pat = self.parse_pattern()?
      // Multi-pattern: pat1 | pat2 | pat3
      let mut alt_patterns: [Pattern] = []
      while self.peek().kind == PPipe {
        self.next()
        let alt = self.parse_pattern()?
        alt_patterns = append(alt_patterns, alt!)
      }
      let mut guard: Expr? = null
      if self.peek().kind == KIf {
        self.next()
        guard = self.parse_expr()?
      }
      self.expect(PFatArrow)?
      let body = self.parse_block()?
      arms = append(arms, MatchArm { pat, alt_patterns, guard, body, body!.span })
      self.skip_newlines()
    }
    self.expect(PRBrace)?
    return (arms, null)
  }

  // ---- Pattern parsing ----

  func Parser.parse_pattern(self) -> (Pattern?, error) {
    let tok = self.peek()
    match tok.kind {
      LIdent => {
        self.next()
        if tok.text == "_" {
          return (Pattern { Wildcard, tok.span }, null)
        }
        // Qualified variant pattern: EnumName.Variant or EnumName.Variant(bindings)
        if self.peek().kind == PDot {
          self.next()  // consume '.'
          let variant_tok = self.peek()
          if variant_tok.kind != LIdent {
            return (null, self.make_error(variant_tok.span, f"expected variant name after '.'"))          }
          self.next()  // consume variant name
          // EnumName.Variant(bindings)
          if self.peek().kind == PLParen {
            self.next()
            let mut bindings: [Pattern] = []
            while self.peek().kind != PRParen && self.peek().kind != SEOF {
              let b = self.parse_pattern()?
              bindings = append(bindings, b!)
              if self.peek().kind == PComma {
                self.next()
              }
            }
            self.expect(PRParen)?
            return (Pattern {
              Variant(sym(variant_tok.text), bindings),
              span: Span { start: tok.span.start, end: self.peek().span.end }
            }, null)
          }
          // EnumName.Variant (unit variant, no parens)
          return (Pattern { PatternKind.Ident(sym(variant_tok.text)), span: Span { start: tok.span.start, end: variant_tok.span.end } }, null)
        }
        // Variant pattern: Name(bindings)
        if self.peek().kind == PLParen {
          self.next()
          let mut bindings: [Pattern] = []
          while self.peek().kind != PRParen && self.peek().kind != SEOF {
            let b = self.parse_pattern()?
            bindings = append(bindings, b!)
            if self.peek().kind == PComma {
              self.next()
            }
          }
          self.expect(PRParen)?
          return (Pattern {
            Variant(sym(tok.text), bindings),
            span: Span { start: tok.span.start, end: self.peek().span.end }
          }, null)
        }
        // Simple ident binding
        return (Pattern { PatternKind.Ident(sym(tok.text)), span: tok.span }, null)
      }
      LIntLit => {
        self.next()
        let expr = Expr { kind: IntLit(tok.text, null), span: tok.span }
        return (Pattern { Literal(expr), span: tok.span }, null)
      }
      LCharLit => {
        self.next()
        let mut val: i32 = 0
        if len(tok.text) > 0 { val = tok.text[0] as i32 }
        let expr = Expr { kind: IntLit(itoa(val), `u8`), span: tok.span }
        return (Pattern { Literal(expr), span: tok.span }, null)
      }
      LStringLit | LTripleString => {
        self.next()
        let expr = Expr { kind: StringLit(tok.text), span: tok.span }
        return (Pattern { Literal(expr), span: tok.span }, null)
      }
      KTrue => {
        self.next()
        let expr = Expr { kind: BoolLit(true), span: tok.span }
        return (Pattern { Literal(expr), span: tok.span }, null)
      }
      KFalse => {
        self.next()
        let expr = Expr { kind: BoolLit(false), span: tok.span }
        return (Pattern { Literal(expr), span: tok.span }, null)
      }
      PLParen => {
        // Tuple pattern
        self.next()
        let mut elems: [Pattern] = []
        while self.peek().kind != PRParen && self.peek().kind != SEOF {
          let elem = self.parse_pattern()?
          elems = append(elems, elem!)
          if self.peek().kind == PComma {
            self.next()
          }
        }
        let end = self.peek().span.end
        self.expect(PRParen)?
        return (Pattern {
          PatternKind.Tuple(elems),
          span: Span { start: tok.span.start, end: end }
        }, null)
      }
      _ => {
        return (null, self.make_error(tok.span, f"expected pattern, got {tok.kind} ({tok.text})"))
      }
    }
  }

  // ---- Cascade ----

  func Parser.parse_cascade(self) -> (Stmt?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'cascade'
    let body = self.parse_block()?
    return (Stmt {
      Cascade(body!),
      span: Span { start: start, end: body!.span.end }
    }, null)
  }

  // ---- Spawn ----

  func Parser.parse_spawn(self) -> (Stmt?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'spawn'
    let body = self.parse_block()?
    return (Stmt {
      Spawn(body!),
      span: Span { start: start, end: body!.span.end }
    }, null)
  }

  // ---- Select ----

  func Parser.parse_select(self) -> (Stmt?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'select'
    self.expect(PLBrace)?
    self.skip_newlines()
    let mut cases: [SelectCase] = []
    while self.peek().kind != PRBrace && self.peek().kind != SEOF {
      let sc = self.parse_select_case()?
      cases = append(cases, sc!)
      self.skip_newlines()
    }
    self.expect(PRBrace)?
    return (Stmt {
      Select(cases),
      span: Span { start: start, end: self.peek().span.end }
    }, null)
  }

  func Parser.parse_select_case(self) -> (SelectCase?, error) {
    let start = self.peek().span.start
    // default case
    if self.peek().kind == LIdent && self.peek().text == "default" {
      self.next()
      self.expect(PFatArrow)?
      let body = self.parse_block()?
      return (SelectCase { is_default: true, body: body, span: Span { start, body!.span.end } }, null)
    }
    self.expect(KCase)?
    // Try: case ident = expr => body (receive with binding)
    let mut bind_var: Sym? = null
    if self.peek().kind == LIdent {
      let saved = self.lex!.save_state()
      let name_tok = self.next()
      if self.peek().kind == OAssign {
        self.next()
        bind_var = sym(name_tok.text)
      } else {
        self.lex!.restore_state(saved)
      }
    }
    let expr = self.parse_expr()?
    self.expect(PFatArrow)?
    let body = self.parse_block()?
    return (SelectCase {
      false,
      bind_var,
      expr,
      body,
      Span { start, body!.span.end }
    }, null)
  }

  // ---- Lock ----

  func Parser.parse_lock(self) -> (Stmt?, error) {
    let start = self.peek().span.start
    self.next()  // consume 'lock'
    self.expect(PLParen)?
    let mutex = self.parse_expr()?
    self.expect(PRParen)?
    let body = self.parse_block()?
    return (Stmt {
      StmtKind.Lock(mutex!, body!),
      span: Span { start: start, end: body!.span.end }
    }, null)
  }

  // ---- Expression statement or assignment ----

  func Parser.parse_expr_or_assign(self) -> (Stmt?, error) {
    let start = self.peek().span.start
    let expr = self.parse_expr()?

    match self.peek().kind {
      OAssign => {
        self.next()
        let val = self.parse_expr()?
        return (Stmt {
          Assign(expr!, val!),
          span: Span { start: start, end: self.peek().span.start }
        }, null)
      }
      OPlusEq => {
        self.next()
        let val = self.parse_expr()?
        return (Stmt {
          Assign(expr!, Expr {
            Binary(expr!, Add, val!),
            span: Span { start: expr!.span.start, end: val!.span.end }
          }),
          span: Span { start: start, end: self.peek().span.start }
        }, null)
      }
      OMinusEq => {
        self.next()
        let val = self.parse_expr()?
        return (Stmt {
          Assign(expr!, Expr {
            Binary(expr!, Sub, val!),
            span: Span { start: expr!.span.start, end: val!.span.end }
          }),
          span: Span { start: start, end: self.peek().span.start }
        }, null)
      }
      OStarEq => {
        self.next()
        let val = self.parse_expr()?
        return (Stmt {
          Assign(expr!, Expr {
            Binary(expr!, Mul, val!),
            span: Span { start: expr!.span.start, end: val!.span.end }
          }),
          span: Span { start: start, end: self.peek().span.start }
        }, null)
      }
      OSlashEq => {
        self.next()
        let val = self.parse_expr()?
        return (Stmt {
          Assign(expr!, Expr {
            Binary(expr!, Div, val!),
            span: Span { start: expr!.span.start, end: val!.span.end }
          }),
          span: Span { start: start, end: self.peek().span.start }
        }, null)
      }
      _ => {
        return (Stmt {
          ExprStmt(expr!),
          span: Span { start: start, end: expr!.span.end }
        }, null)
      }
    }
  }

  // ---- Struct literal ----

  func Parser.is_struct_lit_ahead(self) -> bool {
    if self.no_struct_lit {
      return false
    }
    let saved = self.lex!.save_state()
    let pushed_saved = self.pushed
    self.next()  // consume {
    self.skip_newlines()
    let mut result = false
    if self.peek().kind == PRBrace {
      result = true
    } else if self.peek().kind == LIdent {
      self.next()
      if self.peek().kind == PColon {
        result = true
      }
    } else if (self.peek().kind == LIntLit || self.peek().kind == LFloatLit ||
               self.peek().kind == LStringLit || self.peek().kind == KTrue ||
               self.peek().kind == KFalse || self.peek().kind == KNil ||
               self.peek().kind == OMinus || self.peek().kind == PLParen ||
               self.peek().kind == PLBracket) {
      // Positional struct literal: starts with a literal value or expression
      result = true
    }
    self.lex!.restore_state(saved)
    self.pushed = pushed_saved
    return result
  }

  func Parser.parse_struct_lit(self, name: Sym, type_args: [TypeExpr], start: Pos) -> (Expr?, error) {
    self.next()  // consume {
    let mut fields: [StructLitField] = []
    self.skip_newlines()
    while self.peek().kind != PRBrace && self.peek().kind != SEOF {
      // Try named field: ident ':'
      if self.peek().kind == LIdent {
        let saved = self.lex!.save_state()
        let pushed_saved = self.pushed
        let field_name = self.next()
        if self.peek().kind == PColon {
          self.next()  // consume :
          let val = self.parse_expr()?
          fields = append(fields, StructLitField { sym(field_name.text), value: val })
          if self.peek().kind == PComma {
            self.next()
          }
          self.skip_newlines()
          continue
        }
        // Not a named field — restore and parse as positional
        self.lex!.restore_state(saved)
        self.pushed = pushed_saved
      }
      // Positional field
      let val = self.parse_expr()?
      fields = append(fields, StructLitField { name: sym(""), value: val })
      if self.peek().kind == PComma {
        self.next()
      }
      self.skip_newlines()
    }
    let end = self.expect(PRBrace)?
    return (Expr {
      StructLit(name, type_args, fields),
      span: Span { start: start, end: end!.span.end }
    }, null)
  }

  // ---- F-string ----

  func Parser.parse_fstring(self, tok: Token) -> (Expr?, error) {
    let raw = tok.text
    let mut parts: [Expr] = []
    let mut buf = new_string_builder()
    let mut i: i32 = 0

    while i < len(raw) {
      if raw[i] == 1 as u8 {
        // Sentinel for escaped {{ → literal {
        buf.write_byte('{')
        i = i + 1
      } else if raw[i] == 2 as u8 {
        // Sentinel for escaped }} → literal }
        buf.write_byte('}')
        i = i + 1
      } else if raw[i] == '{' {
        parts = append(parts, Expr {
          StringLit(buf.to_string()),
          span: tok.span
        })
        buf = new_string_builder()
        i = i + 1  // skip {
        let mut depth: i32 = 1
        let expr_start = i
        while i < len(raw) && depth > 0 {
          if raw[i] == '{' { depth = depth + 1 }
          if raw[i] == '}' { depth = depth - 1 }
          if depth > 0 { i = i + 1 }
        }
        let expr_text = raw[expr_start:i]
        i = i + 1  // skip }

        let file_sym = tok.span.start.file
        let expr_lex = new_lexer(expr_text, file_sym!)
        let expr_parser = Parser { lex: expr_lex }
        let expr = expr_parser.parse_expr()?
        parts = append(parts, expr!)
      } else {
        buf.write_byte(raw[i])
        i = i + 1
      }
    }

    parts = append(parts, Expr {
      StringLit(buf.to_string()),
      span: tok.span
    })

    return (Expr {
      StringInterp(parts),
      span: tok.span
    }, null)
  }

  // ---- Type arg parsing (backtrackable) ----

  func Parser.try_parse_type_args(self) -> [TypeExpr]? {
    self.next()  // consume <
    let mut type_args: [TypeExpr] = []
    while self.peek().kind != PGt && self.peek().kind != OShr && self.peek().kind != SEOF {
      let te = self.parse_type_expr()
      if te._1 != null {
        return null
      }
      type_args = append(type_args, te._0!)
      if self.peek().kind == PComma {
        self.next()
      }
    }
    if self.peek().kind == OShr {
      // >> is two > tokens — consume one, push back the other
      let tok = self.next()
      self.push_back(Token {
        kind: PGt,
        text: ">",
        span: Span {
          start: Pos { file: tok.span.start.file, line: tok.span.start.line, column: tok.span.start.column + 1 },
          end: tok.span.end
        }
      })
      return type_args
    }
    if self.peek().kind != PGt {
      return null
    }
    self.next()  // consume >
    return type_args
  }

}
