package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/waywardgeek/forge/pkg/ast"
	"github.com/waywardgeek/forge/pkg/parser"
)

func cmdFmt(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: forge fmt <file.forge> [...]")
	}
	for _, path := range args {
		if err := fmtFile(path); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		fmt.Printf("formatted %s\n", path)
	}
	return nil
}

func fmtFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	zone1, zones23 := splitAtMarker(string(raw), "// --- index ---")

	f, err := parser.ParseFile(zone1, path)
	if err != nil {
		return err
	}

	var b strings.Builder

	// Comments not inside any block (before first block, between blocks)
	commentIdx := 0
	comments := f.Comments

	for bi, block := range f.Blocks {
		// Emit comments before this block
		commentIdx = emitCommentsBefore(&b, comments, commentIdx, block.Span.Start.Line, "")

		fmtBlock(&b, &block, comments, &commentIdx)

		// Blank line between blocks
		if bi < len(f.Blocks)-1 {
			b.WriteString("\n")
		}
	}

	// Trailing comments after last block
	for commentIdx < len(comments) {
		b.WriteString(comments[commentIdx].Text)
		b.WriteString("\n")
		commentIdx++
	}

	// Append zones 2+3 unchanged
	if zones23 != "" {
		b.WriteString(zones23)
	}

	return os.WriteFile(path, []byte(b.String()), 0644)
}

// emitCommentsBefore writes comments with Pos.Line < beforeLine.
// Returns the new commentIdx.
func emitCommentsBefore(b *strings.Builder, comments []ast.Comment, idx, beforeLine int, indent string) int {
	for idx < len(comments) && comments[idx].Pos.Line < beforeLine {
		b.WriteString(indent)
		b.WriteString(comments[idx].Text)
		b.WriteString("\n")
		idx++
	}
	return idx
}

// decl is a union wrapper for sorting declarations by source position.
type decl struct {
	line int
	kind string // "struct", "enum", "interface", "class", "func", "relation", "type", "doc", "invariant", "import"
	idx  int    // index into the typed slice
}

func fmtBlock(b *strings.Builder, block *ast.ForgeBlock, comments []ast.Comment, commentIdx *int) {
	b.WriteString(fmt.Sprintf("forge %s {\n", block.Name))

	// Why
	if block.Why != "" {
		b.WriteString(fmt.Sprintf("  why: %q\n", block.Why))
		b.WriteString("\n")
	}

	// Collect all declarations with their source positions for ordered emission
	var decls []decl
	for i, d := range block.Imports {
		decls = append(decls, decl{line: d.Span.Start.Line, kind: "import", idx: i})
	}
	for i, d := range block.Docs {
		decls = append(decls, decl{line: d.Span.Start.Line, kind: "doc", idx: i})
	}
	for i, d := range block.Invariants {
		decls = append(decls, decl{line: d.Span.Start.Line, kind: "invariant", idx: i})
	}
	for i, d := range block.Structs {
		decls = append(decls, decl{line: d.Span.Start.Line, kind: "struct", idx: i})
	}
	for i, d := range block.Enums {
		decls = append(decls, decl{line: d.Span.Start.Line, kind: "enum", idx: i})
	}
	for i, d := range block.Interfaces {
		decls = append(decls, decl{line: d.Span.Start.Line, kind: "interface", idx: i})
	}
	for i, d := range block.Classes {
		decls = append(decls, decl{line: d.Span.Start.Line, kind: "class", idx: i})
	}
	for i, d := range block.Functions {
		decls = append(decls, decl{line: d.Span.Start.Line, kind: "func", idx: i})
	}
	for i, d := range block.Relations {
		decls = append(decls, decl{line: d.Span.Start.Line, kind: "relation", idx: i})
	}
	for i, d := range block.TypeAliases {
		decls = append(decls, decl{line: d.Span.Start.Line, kind: "type", idx: i})
	}

	sort.Slice(decls, func(i, j int) bool { return decls[i].line < decls[j].line })

	for di, d := range decls {
		// Emit comments before this declaration
		*commentIdx = emitCommentsBefore(b, comments, *commentIdx, d.line, "  ")

		switch d.kind {
		case "import":
			fmtImport(b, &block.Imports[d.idx])
		case "doc":
			fmtDoc(b, &block.Docs[d.idx])
		case "invariant":
			fmtInvariant(b, &block.Invariants[d.idx])
		case "struct":
			fmtStruct(b, &block.Structs[d.idx])
		case "enum":
			fmtEnum(b, &block.Enums[d.idx])
		case "interface":
			fmtInterface(b, &block.Interfaces[d.idx])
		case "class":
			fmtClass(b, &block.Classes[d.idx])
		case "func":
			fmtFunc(b, &block.Functions[d.idx], "  ")
		case "relation":
			fmtRelation(b, &block.Relations[d.idx])
		case "type":
			fmtTypeAlias(b, &block.TypeAliases[d.idx])
		}

		// Blank line between declarations
		if di < len(decls)-1 {
			b.WriteString("\n")
		}
	}

	// Source annotation
	if len(block.Source) > 0 {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  source: [%s]\n", formatSourceList(block.Source)))
	}

	// Emit comments before closing brace
	*commentIdx = emitCommentsBefore(b, comments, *commentIdx, block.Span.End.Line+1, "  ")

	b.WriteString("}\n")
}

// --- Declaration formatters ---

func fmtImport(b *strings.Builder, imp *ast.ImportDecl) {
	if imp.Alias != "" {
		b.WriteString(fmt.Sprintf("  import %s from %q\n", imp.Alias, imp.Path))
	} else {
		b.WriteString(fmt.Sprintf("  import %q\n", imp.Path))
	}
}

func fmtDoc(b *strings.Builder, doc *ast.DocBlock) {
	b.WriteString(fmt.Sprintf("  doc %q: \"\"\"\n", doc.Section))
	// Indent each line of content with 4 spaces
	for _, line := range strings.Split(doc.Content, "\n") {
		if strings.TrimSpace(line) == "" {
			b.WriteString("\n")
		} else {
			b.WriteString("    ")
			b.WriteString(strings.TrimLeft(line, " "))
			b.WriteString("\n")
		}
	}
	b.WriteString("  \"\"\"\n")
}

func fmtInvariant(b *strings.Builder, inv *ast.InvariantDecl) {
	b.WriteString(fmt.Sprintf("  invariant %q", inv.Claim))
	if inv.VerifiedAt != "" {
		b.WriteString(fmt.Sprintf(" verified_at %q", inv.VerifiedAt))
	}
	b.WriteString("\n")
}

func fmtStruct(b *strings.Builder, s *ast.StructDecl) {
	b.WriteString("  ")
	if s.IsPublic {
		b.WriteString("pub ")
	}
	b.WriteString(fmt.Sprintf("struct %s", s.Name))
	fmtTypeParams(b, s.TypeParams)
	b.WriteString(" {\n")
	if s.Why != "" {
		b.WriteString(fmt.Sprintf("    why: %q\n", s.Why))
	}
	for _, f := range s.Fields {
		fmtField(b, &f)
	}
	b.WriteString("  }\n")
}

func fmtEnum(b *strings.Builder, e *ast.EnumDecl) {
	b.WriteString("  ")
	if e.IsPublic {
		b.WriteString("pub ")
	}
	b.WriteString(fmt.Sprintf("enum %s", e.Name))
	fmtTypeParams(b, e.TypeParams)
	b.WriteString(" {")
	if e.Why != "" {
		b.WriteString(fmt.Sprintf("\n    why: %q\n", e.Why))
	}

	// Check if all variants are simple (no fields) — emit on one line
	allSimple := true
	for _, v := range e.Variants {
		if len(v.Fields) > 0 {
			allSimple = false
			break
		}
	}

	if allSimple && len(e.Variants) <= 5 {
		b.WriteString(" ")
		for i, v := range e.Variants {
			if i > 0 {
				b.WriteString(" ")
			}
			b.WriteString(v.Name)
		}
		b.WriteString(" }\n")
	} else {
		b.WriteString("\n")
		for _, v := range e.Variants {
			b.WriteString(fmt.Sprintf("    %s", v.Name))
			if len(v.Fields) > 0 {
				b.WriteString("(")
				for i, f := range v.Fields {
					if i > 0 {
						b.WriteString(", ")
					}
					if f.Name != "" {
						b.WriteString(fmt.Sprintf("%s: ", f.Name))
					}
					b.WriteString(fmtType(&f.Type))
				}
				b.WriteString(")")
			}
			b.WriteString("\n")
		}
		b.WriteString("  }\n")
	}
}

func fmtInterface(b *strings.Builder, iface *ast.InterfaceDecl) {
	b.WriteString("  ")
	if iface.IsPublic {
		b.WriteString("pub ")
	}
	b.WriteString(fmt.Sprintf("interface %s", iface.Name))
	fmtTypeParams(b, iface.TypeParams)
	b.WriteString(" {\n")
	if iface.Why != "" {
		b.WriteString(fmt.Sprintf("    why: %q\n", iface.Why))
	}
	for _, parent := range iface.Implements {
		b.WriteString(fmt.Sprintf("    %s\n", parent))
	}
	for _, emb := range iface.Embeds {
		b.WriteString(fmt.Sprintf("    embed %s", emb.Name))
		if len(emb.TypeArgs) > 0 {
			b.WriteString("<")
			for i, ta := range emb.TypeArgs {
				if i > 0 {
					b.WriteString(", ")
				}
				if ta.Kind == ast.TypeNamed {
					nt := ta.Data.(*ast.NamedType)
					b.WriteString(nt.Name)
				}
			}
			b.WriteString(">")
		}
		b.WriteString("\n")
	}
	for _, f := range iface.Fields {
		b.WriteString(fmt.Sprintf("    field %s.%s: %s\n", f.TypeParam, f.Name, fmtType(&f.Type)))
	}
	for _, d := range iface.Destructors {
		b.WriteString(fmt.Sprintf("    destructor %s { ... }\n", d.TypeParam))
	}
	for _, m := range iface.Methods {
		fmtFunc(b, &m, "    ")
	}
	b.WriteString("  }\n")
}

func fmtClass(b *strings.Builder, c *ast.ClassDecl) {
	b.WriteString("  ")
	if c.IsPublic {
		b.WriteString("pub ")
	}
	b.WriteString(fmt.Sprintf("class %s", c.Name))
	fmtTypeParams(b, c.TypeParams)
	b.WriteString("(")
	for i, p := range c.CtorParams {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%s: %s", p.Name, fmtType(&p.Type)))
	}
	b.WriteString(")")
	if len(c.Implements) > 0 {
		b.WriteString(" implements ")
		b.WriteString(strings.Join(c.Implements, ", "))
	}
	b.WriteString(" {\n")
	if c.Why != "" {
		b.WriteString(fmt.Sprintf("    why: %q\n", c.Why))
	}
	for _, f := range c.Fields {
		fmtField(b, &f)
	}
	for _, m := range c.Methods {
		fmtFunc(b, &m, "    ")
	}
	b.WriteString("  }\n")
}

func fmtFunc(b *strings.Builder, fn *ast.FuncDecl, indent string) {
	b.WriteString(indent)
	if fn.IsPublic {
		b.WriteString("pub ")
	}
	b.WriteString(fmt.Sprintf("func %s", fn.Name))
	fmtTypeParams(b, fn.TypeParams)
	b.WriteString("(")
	for i, p := range fn.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		if p.IsSelf {
			if p.IsMut {
				b.WriteString("mut self")
			} else {
				b.WriteString("self")
			}
		} else {
			b.WriteString(fmt.Sprintf("%s: %s", p.Name, fmtType(&p.Type)))
		}
	}
	b.WriteString(")")
	if fn.ReturnType != nil {
		b.WriteString(" -> ")
		b.WriteString(fmtType(fn.ReturnType))
	}
	if len(fn.Where) > 0 {
		for _, w := range fn.Where {
			b.WriteString(fmt.Sprintf("\n%s  where %s: %s", indent, w.Variable, w.Constraint))
		}
	}
	b.WriteString("\n")
	fmtAnnotations(b, &fn.Annotations, indent)
}

func fmtRelation(b *strings.Builder, r *ast.RelationDecl) {
	b.WriteString("  ")
	if r.Hint != "" {
		b.WriteString(fmt.Sprintf("[%s] ", r.Hint))
	}
	b.WriteString(r.Parent.TypeName)
	if r.Parent.Label != "" {
		b.WriteString(fmt.Sprintf(".%s", r.Parent.Label))
	}
	switch r.Kind {
	case ast.Owns:
		b.WriteString(" owns ")
	case ast.Refs:
		b.WriteString(" refs ")
	}
	if r.IsMany {
		b.WriteString("[")
	}
	b.WriteString(r.Child.TypeName)
	if r.Child.Label != "" {
		b.WriteString(fmt.Sprintf(".%s", r.Child.Label))
	}
	if r.IsMany {
		b.WriteString("]")
	}
	b.WriteString("\n")
}

func fmtTypeAlias(b *strings.Builder, t *ast.TypeAliasDecl) {
	b.WriteString("  ")
	if t.IsPublic {
		b.WriteString("pub ")
	}
	b.WriteString(fmt.Sprintf("type %s = %s\n", t.Name, fmtType(&t.Type)))
}

// --- Helpers ---

func fmtField(b *strings.Builder, f *ast.Field) {
	b.WriteString(fmt.Sprintf("    %s: %s", f.Name, fmtType(&f.Type)))
	if f.GuardedBy != "" {
		b.WriteString(fmt.Sprintf(" guarded_by(%s)", f.GuardedBy))
	}
	b.WriteString("\n")
	if f.Why != "" {
		b.WriteString(fmt.Sprintf("      why: %q\n", f.Why))
	}
}

func fmtAnnotations(b *strings.Builder, a *ast.Annotations, indent string) {
	if a.Why != "" {
		b.WriteString(fmt.Sprintf("%s  why: %q\n", indent, a.Why))
	}
	if a.Pure {
		b.WriteString(fmt.Sprintf("%s  pure\n", indent))
	}
	if a.Spawns {
		b.WriteString(fmt.Sprintf("%s  spawns\n", indent))
	}
	if a.Concurrent != nil {
		if *a.Concurrent {
			b.WriteString(fmt.Sprintf("%s  concurrent: true\n", indent))
		} else {
			b.WriteString(fmt.Sprintf("%s  concurrent: false\n", indent))
		}
	}
	for _, l := range a.RequiresLock {
		b.WriteString(fmt.Sprintf("%s  requires_lock(%s)\n", indent, l))
	}
	for _, l := range a.ExcludesLock {
		b.WriteString(fmt.Sprintf("%s  excludes_lock(%s)\n", indent, l))
	}
	for _, r := range a.Requires {
		b.WriteString(fmt.Sprintf("%s  requires %q\n", indent, r))
	}
	for _, e := range a.Ensures {
		b.WriteString(fmt.Sprintf("%s  ensures %q\n", indent, e))
	}
}

func fmtTypeParams(b *strings.Builder, tps []ast.TypeParam) {
	if len(tps) == 0 {
		return
	}
	b.WriteString("<")
	for i, tp := range tps {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(tp.Name)
		if tp.Constraint != "" {
			b.WriteString(": ")
			b.WriteString(tp.Constraint)
		}
	}
	b.WriteString(">")
}

func fmtType(t *ast.TypeExpr) string {
	switch t.Kind {
	case ast.TypeNamed:
		nt := t.Data.(ast.NamedType)
		if len(nt.Args) == 0 {
			return nt.Name
		}
		var args []string
		for _, a := range nt.Args {
			args = append(args, fmtType(&a))
		}
		return fmt.Sprintf("%s<%s>", nt.Name, strings.Join(args, ", "))
	case ast.TypeOptional:
		ot := t.Data.(ast.OptionalType)
		return fmtType(&ot.Inner) + "?"
	case ast.TypeUnion:
		ut := t.Data.(ast.UnionType)
		var parts []string
		for _, v := range ut.Variants {
			parts = append(parts, fmtType(&v))
		}
		return strings.Join(parts, " | ")
	case ast.TypeSequence:
		st := t.Data.(ast.SequenceType)
		return "[" + fmtType(&st.Elem) + "]"
	case ast.TypeMap:
		mt := t.Data.(ast.MapType)
		return fmt.Sprintf("map[%s]%s", fmtType(&mt.Key), fmtType(&mt.Value))
	case ast.TypeTuple:
		tt := t.Data.(ast.TupleType)
		var parts []string
		for _, f := range tt.Fields {
			if f.Name != "" {
				parts = append(parts, fmt.Sprintf("%s: %s", f.Name, fmtType(&f.Type)))
			} else {
				parts = append(parts, fmtType(&f.Type))
			}
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case ast.TypeFunc:
		ft := t.Data.(ast.FuncType)
		var params []string
		for _, p := range ft.Params {
			params = append(params, fmtType(&p))
		}
		return fmt.Sprintf("(%s) -> %s", strings.Join(params, ", "), fmtType(&ft.Return))
	case ast.TypeChannel:
		ct := t.Data.(ast.ChannelType)
		return fmt.Sprintf("channel<%s>", fmtType(&ct.Elem))
	case ast.TypeLock:
		return "lock"
	case ast.TypeUnit:
		return "unit"
	default:
		return "???"
	}
}
