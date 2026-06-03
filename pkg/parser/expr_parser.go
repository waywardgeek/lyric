package parser

import (
	"fmt"

	"github.com/waywardgeek/grok/pkg/ast"
)

// --- Precedence levels for Pratt parsing ---
const (
	precNone       = 0
	precOr         = 1  // ||
	precAnd        = 2  // &&
	precBitOr      = 3  // |
	precBitXor     = 4  // ^
	precBitAnd     = 5  // &
	precEquality   = 6  // == !=
	precComparison = 7  // < > <= >=
	precShift      = 8  // << >>
	precAdditive   = 9  // + -
	precMult       = 10 // * / %
	precUnary      = 11 // - !
	precPostfix    = 12 // . () []
)

func binaryPrec(kind TokenKind) int {
	switch kind {
	case TPipePipe:
		return precOr
	case TAmpAmp:
		return precAnd
	case TPipe:
		return precBitOr
	case TCaret:
		return precBitXor
	case TAmp:
		return precBitAnd
	case TEqEq, TBangEq:
		return precEquality
	case TLt, TGt, TLtEq, TGtEq:
		return precComparison
	case TShl, TShr:
		return precShift
	case TPlus, TMinus:
		return precAdditive
	case TStar, TSlash, TPercent:
		return precMult
	default:
		return precNone
	}
}

func tokenToBinaryOp(kind TokenKind) ast.BinaryOp {
	switch kind {
	case TPlus:
		return ast.OpAdd
	case TMinus:
		return ast.OpSub
	case TStar:
		return ast.OpMul
	case TSlash:
		return ast.OpDiv
	case TPercent:
		return ast.OpMod
	case TEqEq:
		return ast.OpEq
	case TBangEq:
		return ast.OpNeq
	case TLt:
		return ast.OpLt
	case TLtEq:
		return ast.OpLe
	case TGt:
		return ast.OpGt
	case TGtEq:
		return ast.OpGe
	case TAmpAmp:
		return ast.OpAnd
	case TPipePipe:
		return ast.OpOr
	case TAmp:
		return ast.OpBitAnd
	case TPipe:
		return ast.OpBitOr
	case TCaret:
		return ast.OpBitXor
	case TShl:
		return ast.OpShl
	case TShr:
		return ast.OpShr
	default:
		return ast.OpAdd // unreachable
	}
}

// parseExpr parses an expression using Pratt/precedence climbing.
func (p *Parser) parseExpr() (*ast.Expr, error) {
	return p.parsePrecExpr(precNone + 1)
}

func (p *Parser) parsePrecExpr(minPrec int) (*ast.Expr, error) {
	left, err := p.parseUnaryExpr()
	if err != nil {
		return nil, err
	}

	for {
		prec := binaryPrec(p.peek().Kind)
		if prec < minPrec {
			break
		}
		op := p.next()
		right, err := p.parsePrecExpr(prec + 1) // left-associative
		if err != nil {
			return nil, err
		}
		left = &ast.Expr{
			Kind: ast.ExprBinary,
			Data: &ast.BinaryExpr{Left: *left, Op: tokenToBinaryOp(op.Kind), Right: *right},
			Span: ast.Span{Start: left.Span.Start, End: right.Span.End},
		}
	}
	return left, nil
}

func (p *Parser) parseUnaryExpr() (*ast.Expr, error) {
	tok := p.peek()
	if tok.Kind == TBang {
		p.next()
		operand, err := p.parseUnaryExpr()
		if err != nil {
			return nil, err
		}
		return &ast.Expr{
			Kind: ast.ExprUnary,
			Data: &ast.UnaryExpr{Op: ast.OpNot, Operand: *operand},
			Span: ast.Span{Start: tok.Span.Start, End: operand.Span.End},
		}, nil
	}
	if tok.Kind == TMinus {
		p.next()
		operand, err := p.parseUnaryExpr()
		if err != nil {
			return nil, err
		}
		return &ast.Expr{
			Kind: ast.ExprUnary,
			Data: &ast.UnaryExpr{Op: ast.OpNeg, Operand: *operand},
			Span: ast.Span{Start: tok.Span.Start, End: operand.Span.End},
		}, nil
	}
	return p.parsePostfixExpr()
}

func (p *Parser) parsePostfixExpr() (*ast.Expr, error) {
	expr, err := p.parsePrimaryExpr()
	if err != nil {
		return nil, err
	}

	for {
		switch p.peek().Kind {
		case TDot:
			p.next()
			name, err := p.expectIdentLike()
			if err != nil {
				return nil, err
			}
			// Check for method call: obj.method(args)
			if p.peek().Kind == TLParen {
				p.next()
				args, end, err := p.parseArgList()
				if err != nil {
					return nil, err
				}
				expr = &ast.Expr{
					Kind: ast.ExprMethodCall,
					Data: &ast.MethodCallExpr{Receiver: *expr, Method: name.Text, Args: args},
					Span: ast.Span{Start: expr.Span.Start, End: end},
				}
			} else {
				expr = &ast.Expr{
					Kind: ast.ExprFieldAccess,
					Data: &ast.FieldAccessExpr{Receiver: *expr, Field: name.Text},
					Span: ast.Span{Start: expr.Span.Start, End: name.Span.End},
				}
			}
		case TLParen:
			p.next()
			args, end, err := p.parseArgList()
			if err != nil {
				return nil, err
			}
			expr = &ast.Expr{
				Kind: ast.ExprCall,
				Data: &ast.CallExpr{Func: *expr, Args: args},
				Span: ast.Span{Start: expr.Span.Start, End: end},
			}
		case TLBracket:
			p.next()
			index, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			end, err := p.expect(TRBracket)
			if err != nil {
				return nil, err
			}
			expr = &ast.Expr{
				Kind: ast.ExprIndex,
				Data: &ast.IndexExpr{Receiver: *expr, Index: *index},
				Span: ast.Span{Start: expr.Span.Start, End: end.Span.End},
			}
		default:
			return expr, nil
		}
	}
}

func (p *Parser) parseArgList() ([]ast.Expr, ast.Pos, error) {
	var args []ast.Expr
	for p.peek().Kind != TRParen && p.peek().Kind != TEOF {
		p.skipNewlines()
		arg, err := p.parseExpr()
		if err != nil {
			return nil, ast.Pos{}, err
		}
		args = append(args, *arg)
		if p.peek().Kind == TComma {
			p.next()
		}
		p.skipNewlines()
	}
	end, err := p.expect(TRParen)
	if err != nil {
		return nil, ast.Pos{}, err
	}
	return args, end.Span.End, nil
}

func (p *Parser) parsePrimaryExpr() (*ast.Expr, error) {
	tok := p.peek()
	switch tok.Kind {
	case TIntLit:
		p.next()
		return &ast.Expr{
			Kind: ast.ExprIntLit,
			Data: &ast.IntLitExpr{Value: tok.Text},
			Span: tok.Span,
		}, nil
	case TFloatLit:
		p.next()
		return &ast.Expr{
			Kind: ast.ExprFloatLit,
			Data: &ast.FloatLitExpr{Value: tok.Text},
			Span: tok.Span,
		}, nil
	case TStringLit:
		p.next()
		return &ast.Expr{
			Kind: ast.ExprStringLit,
			Data: &ast.StringLitExpr{Value: tok.Text},
			Span: tok.Span,
		}, nil
	case TTrue:
		p.next()
		return &ast.Expr{
			Kind: ast.ExprBoolLit,
			Data: &ast.BoolLitExpr{Value: true},
			Span: tok.Span,
		}, nil
	case TFalse:
		p.next()
		return &ast.Expr{
			Kind: ast.ExprBoolLit,
			Data: &ast.BoolLitExpr{Value: false},
			Span: tok.Span,
		}, nil
	case TNil:
		p.next()
		return &ast.Expr{
			Kind: ast.ExprNil,
			Span: tok.Span,
		}, nil
	case TIdent:
		p.next()
		// Check for map literal: map[K]V{...}
		if tok.Text == "map" && p.peek().Kind == TLBracket {
			return p.parseMapLit(tok)
		}
		return &ast.Expr{
			Kind: ast.ExprIdent,
			Data: &ast.IdentExpr{Name: tok.Text},
			Span: tok.Span,
		}, nil
	case TLParen:
		// Parenthesized expr or tuple literal
		p.next()
		if p.peek().Kind == TRParen {
			// Empty tuple / unit
			end := p.next()
			return &ast.Expr{
				Kind: ast.ExprTupleLit,
				Data: &ast.TupleLitExpr{},
				Span: ast.Span{Start: tok.Span.Start, End: end.Span.End},
			}, nil
		}
		first, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peek().Kind == TComma {
			// Tuple
			elems := []ast.Expr{*first}
			for p.peek().Kind == TComma {
				p.next()
				if p.peek().Kind == TRParen {
					break
				}
				elem, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				elems = append(elems, *elem)
			}
			end, err := p.expect(TRParen)
			if err != nil {
				return nil, err
			}
			return &ast.Expr{
				Kind: ast.ExprTupleLit,
				Data: &ast.TupleLitExpr{Elems: elems},
				Span: ast.Span{Start: tok.Span.Start, End: end.Span.End},
			}, nil
		}
		// Parenthesized expression
		if _, err := p.expect(TRParen); err != nil {
			return nil, err
		}
		return first, nil
	case TLBracket:
		// List literal: [1, 2, 3]
		p.next()
		var elems []ast.Expr
		for p.peek().Kind != TRBracket && p.peek().Kind != TEOF {
			p.skipNewlines()
			elem, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			elems = append(elems, *elem)
			if p.peek().Kind == TComma {
				p.next()
			}
			p.skipNewlines()
		}
		end, err := p.expect(TRBracket)
		if err != nil {
			return nil, err
		}
		return &ast.Expr{
			Kind: ast.ExprListLit,
			Data: &ast.ListLitExpr{Elems: elems},
			Span: ast.Span{Start: tok.Span.Start, End: end.Span.End},
		}, nil
	case TMatch:
		return p.parseMatchExpr()
	default:
		return nil, &ParseError{
			Message: fmt.Sprintf("expected expression, got %s (%q)", tokenNames[tok.Kind], tok.Text),
			Span:    tok.Span,
		}
	}
}

func (p *Parser) parseMapLit(mapTok Token) (*ast.Expr, error) {
	// Already consumed "map", peek is [
	p.next() // [
	// We need to skip the type part: map[K]V{...}
	// For now, just consume tokens until we hit {
	// This is a simplification — in a full type checker we'd parse types
	depth := 1
	for depth > 0 && p.peek().Kind != TEOF {
		switch p.peek().Kind {
		case TLBracket:
			depth++
			p.next()
		case TRBracket:
			depth--
			p.next()
		default:
			p.next()
		}
	}
	// Consume the value type until {
	for p.peek().Kind != TLBrace && p.peek().Kind != TEOF {
		p.next()
	}
	if _, err := p.expect(TLBrace); err != nil {
		return nil, err
	}
	p.skipNewlines()
	var entries []ast.MapEntry
	for p.peek().Kind != TRBrace && p.peek().Kind != TEOF {
		key, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TColon); err != nil {
			return nil, err
		}
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		entries = append(entries, ast.MapEntry{Key: *key, Value: *val})
		if p.peek().Kind == TComma {
			p.next()
		}
		p.skipNewlines()
	}
	end, err := p.expect(TRBrace)
	if err != nil {
		return nil, err
	}
	return &ast.Expr{
		Kind: ast.ExprMapLit,
		Data: &ast.MapLitExpr{Entries: entries},
		Span: ast.Span{Start: mapTok.Span.Start, End: end.Span.End},
	}, nil
}

func (p *Parser) parseMatchExpr() (*ast.Expr, error) {
	start := p.peek().Span.Start
	p.next() // consume 'match'
	value, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TLBrace); err != nil {
		return nil, err
	}
	p.skipNewlines()
	stmt, err := p.parseMatchArms()
	if err != nil {
		return nil, err
	}
	end := p.peek().Span.End
	return &ast.Expr{
		Kind: ast.ExprMatch,
		Data: &ast.MatchStmt{Value: *value, Arms: stmt},
		Span: ast.Span{Start: start, End: end},
	}, nil
}

// --- Statement parsing ---

// parseBlock parses { stmts... }
func (p *Parser) parseBlock() (*ast.Block, error) {
	start := p.peek().Span.Start
	if _, err := p.expect(TLBrace); err != nil {
		return nil, err
	}
	p.skipNewlines()
	var stmts []ast.Stmt
	for p.peek().Kind != TRBrace && p.peek().Kind != TEOF {
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, *stmt)
		p.skipNewlines()
	}
	end, err := p.expect(TRBrace)
	if err != nil {
		return nil, err
	}
	return &ast.Block{
		Stmts: stmts,
		Span:  ast.Span{Start: start, End: end.Span.End},
	}, nil
}

func (p *Parser) parseStmt() (*ast.Stmt, error) {
	tok := p.peek()
	switch tok.Kind {
	case TLet:
		return p.parseVarDecl()
	case TReturn:
		return p.parseReturn()
	case TIf:
		return p.parseIf()
	case TFor:
		return p.parseFor()
	case TWhile:
		return p.parseWhile()
	case TMatch:
		return p.parseMatchStmt()
	case TBreak:
		p.next()
		return &ast.Stmt{Kind: ast.StmtBreak, Span: tok.Span}, nil
	case TContinue:
		p.next()
		return &ast.Stmt{Kind: ast.StmtContinue, Span: tok.Span}, nil
	case TCascadeKw:
		return p.parseCascade()
	case TLBrace:
		blk, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		return &ast.Stmt{Kind: ast.StmtBlock, Data: blk, Span: blk.Span}, nil
	default:
		// Expression statement or assignment
		return p.parseExprOrAssign()
	}
}

func (p *Parser) parseVarDecl() (*ast.Stmt, error) {
	start := p.peek().Span.Start
	p.next() // consume 'let'
	isMut := false
	if p.peek().Kind == TMut {
		isMut = true
		p.next()
	}
	name, err := p.expectIdentLike()
	if err != nil {
		return nil, err
	}
	decl := &ast.VarDeclStmt{Name: name.Text, IsMut: isMut}

	// Optional type annotation
	if p.peek().Kind == TColon {
		p.next()
		typ, err := p.parseTypeExpr()
		if err != nil {
			return nil, err
		}
		decl.Type = typ
	}

	// Optional initializer
	if p.peek().Kind == TAssign {
		p.next()
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		decl.Value = val
	}

	return &ast.Stmt{
		Kind: ast.StmtVarDecl,
		Data: decl,
		Span: ast.Span{Start: start, End: p.peek().Span.Start},
	}, nil
}

func (p *Parser) parseReturn() (*ast.Stmt, error) {
	start := p.peek().Span.Start
	p.next() // consume 'return'
	ret := &ast.ReturnStmt{}
	// Check if there's an expression following (not newline/EOF/})
	if p.peek().Kind != TNewline && p.peek().Kind != TEOF && p.peek().Kind != TRBrace {
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		ret.Value = val
	}
	return &ast.Stmt{
		Kind: ast.StmtReturn,
		Data: ret,
		Span: ast.Span{Start: start, End: p.peek().Span.Start},
	}, nil
}

func (p *Parser) parseIf() (*ast.Stmt, error) {
	start := p.peek().Span.Start
	p.next() // consume 'if'
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	then, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	ifStmt := &ast.IfStmt{Condition: *cond, Then: *then}

	// else if / else
	for p.peek().Kind == TElse {
		p.next()
		if p.peek().Kind == TIf {
			p.next()
			elifCond, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			elifBody, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			ifStmt.ElseIfs = append(ifStmt.ElseIfs, ast.ElseIf{
				Condition: *elifCond,
				Body:      *elifBody,
				Span:      elifBody.Span,
			})
		} else {
			elseBody, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			ifStmt.Else = elseBody
			break
		}
	}

	return &ast.Stmt{
		Kind: ast.StmtIf,
		Data: ifStmt,
		Span: ast.Span{Start: start, End: p.peek().Span.Start},
	}, nil
}

func (p *Parser) parseFor() (*ast.Stmt, error) {
	start := p.peek().Span.Start
	p.next() // consume 'for'
	varName, err := p.expectIdentLike()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TIn); err != nil {
		return nil, err
	}
	collection, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.Stmt{
		Kind: ast.StmtFor,
		Data: &ast.ForStmt{Var: varName.Text, Collection: *collection, Body: *body},
		Span: ast.Span{Start: start, End: body.Span.End},
	}, nil
}

func (p *Parser) parseWhile() (*ast.Stmt, error) {
	start := p.peek().Span.Start
	p.next() // consume 'while'
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.Stmt{
		Kind: ast.StmtWhile,
		Data: &ast.WhileStmt{Condition: *cond, Body: *body},
		Span: ast.Span{Start: start, End: body.Span.End},
	}, nil
}

func (p *Parser) parseMatchStmt() (*ast.Stmt, error) {
	start := p.peek().Span.Start
	p.next() // consume 'match'
	value, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TLBrace); err != nil {
		return nil, err
	}
	p.skipNewlines()
	arms, err := p.parseMatchArms()
	if err != nil {
		return nil, err
	}
	return &ast.Stmt{
		Kind: ast.StmtMatch,
		Data: &ast.MatchStmt{Value: *value, Arms: arms},
		Span: ast.Span{Start: start, End: p.peek().Span.Start},
	}, nil
}

func (p *Parser) parseMatchArms() ([]ast.MatchArm, error) {
	var arms []ast.MatchArm
	for p.peek().Kind != TRBrace && p.peek().Kind != TEOF {
		pat, err := p.parsePattern()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TFatArrow); err != nil {
			return nil, err
		}
		body, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		arms = append(arms, ast.MatchArm{Pattern: *pat, Body: *body, Span: body.Span})
		p.skipNewlines()
	}
	if _, err := p.expect(TRBrace); err != nil {
		return nil, err
	}
	return arms, nil
}

func (p *Parser) parsePattern() (*ast.Pattern, error) {
	tok := p.peek()
	switch tok.Kind {
	case TIdent:
		p.next()
		// Wildcard: _
		if tok.Text == "_" {
			return &ast.Pattern{Kind: ast.PatWildcard, Span: tok.Span}, nil
		}
		// Check for variant pattern: Name(bindings)
		if p.peek().Kind == TLParen {
			p.next()
			var bindings []ast.Pattern
			for p.peek().Kind != TRParen && p.peek().Kind != TEOF {
				b, err := p.parsePattern()
				if err != nil {
					return nil, err
				}
				bindings = append(bindings, *b)
				if p.peek().Kind == TComma {
					p.next()
				}
			}
			if _, err := p.expect(TRParen); err != nil {
				return nil, err
			}
			return &ast.Pattern{
				Kind: ast.PatVariant,
				Data: &ast.VariantPattern{Name: tok.Text, Bindings: bindings},
				Span: ast.Span{Start: tok.Span.Start, End: p.peek().Span.End},
			}, nil
		}
		// Simple ident binding
		return &ast.Pattern{
			Kind: ast.PatIdent,
			Data: &ast.IdentPattern{Name: tok.Text},
			Span: tok.Span,
		}, nil
	case TIntLit, TStringLit, TTrue, TFalse:
		p.next()
		expr := ast.Expr{Span: tok.Span}
		switch tok.Kind {
		case TIntLit:
			expr.Kind = ast.ExprIntLit
			expr.Data = &ast.IntLitExpr{Value: tok.Text}
		case TStringLit:
			expr.Kind = ast.ExprStringLit
			expr.Data = &ast.StringLitExpr{Value: tok.Text}
		case TTrue:
			expr.Kind = ast.ExprBoolLit
			expr.Data = &ast.BoolLitExpr{Value: true}
		case TFalse:
			expr.Kind = ast.ExprBoolLit
			expr.Data = &ast.BoolLitExpr{Value: false}
		}
		return &ast.Pattern{
			Kind: ast.PatLiteral,
			Data: &ast.LiteralPattern{Expr: expr},
			Span: tok.Span,
		}, nil
	default:
		return nil, &ParseError{
			Message: fmt.Sprintf("expected pattern, got %s (%q)", tokenNames[tok.Kind], tok.Text),
			Span:    tok.Span,
		}
	}
}

func (p *Parser) parseCascade() (*ast.Stmt, error) {
	start := p.peek().Span.Start
	p.next() // consume 'cascade'
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.Stmt{
		Kind: ast.StmtCascade,
		Data: &ast.CascadeStmt{Body: *body},
		Span: ast.Span{Start: start, End: body.Span.End},
	}, nil
}

func (p *Parser) parseExprOrAssign() (*ast.Stmt, error) {
	start := p.peek().Span.Start
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	// Check for assignment: = += -= *= /=
	switch p.peek().Kind {
	case TAssign:
		p.next()
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		return &ast.Stmt{
			Kind: ast.StmtAssign,
			Data: &ast.AssignStmt{Target: *expr, Value: *val},
			Span: ast.Span{Start: start, End: p.peek().Span.Start},
		}, nil
	case TPlusEq, TMinusEq, TStarEq, TSlashEq:
		// Desugar x += y into x = x + y
		opTok := p.next()
		val, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		var binOp ast.BinaryOp
		switch opTok.Kind {
		case TPlusEq:
			binOp = ast.OpAdd
		case TMinusEq:
			binOp = ast.OpSub
		case TStarEq:
			binOp = ast.OpMul
		case TSlashEq:
			binOp = ast.OpDiv
		}
		return &ast.Stmt{
			Kind: ast.StmtAssign,
			Data: &ast.AssignStmt{
				Target: *expr,
				Value: ast.Expr{
					Kind: ast.ExprBinary,
					Data: &ast.BinaryExpr{Left: *expr, Op: binOp, Right: *val},
					Span: ast.Span{Start: expr.Span.Start, End: val.Span.End},
				},
			},
			Span: ast.Span{Start: start, End: p.peek().Span.Start},
		}, nil
	}

	// Expression statement
	return &ast.Stmt{
		Kind: ast.StmtExpr,
		Data: &ast.ExprStmt{Expr: *expr},
		Span: ast.Span{Start: start, End: expr.Span.End},
	}, nil
}
