package parser

import (
	"os"
	"testing"

	"github.com/waywardgeek/grok/pkg/ast"
)

func TestParseMinimalGrok(t *testing.T) {
	input := `grok Foo {
  why: "A test module."
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(file.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(file.Blocks))
	}
	if file.Blocks[0].Name != "Foo" {
		t.Errorf("expected name Foo, got %s", file.Blocks[0].Name)
	}
	if file.Blocks[0].Why != "A test module." {
		t.Errorf("expected why, got %q", file.Blocks[0].Why)
	}
}

func TestParseStruct(t *testing.T) {
	input := `grok Test {
  struct Point {
    x: f64
    y: f64
  }
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(file.Blocks[0].Structs) != 1 {
		t.Fatalf("expected 1 struct, got %d", len(file.Blocks[0].Structs))
	}
	s := file.Blocks[0].Structs[0]
	if s.Name != "Point" {
		t.Errorf("expected Point, got %s", s.Name)
	}
	if len(s.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(s.Fields))
	}
}

func TestParseEnum(t *testing.T) {
	input := `grok Test {
  enum Direction { North South East West }
  enum Shape {
    Circle(radius: f64)
    Rectangle(width: f64, height: f64)
  }
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(file.Blocks[0].Enums) != 2 {
		t.Fatalf("expected 2 enums, got %d", len(file.Blocks[0].Enums))
	}
	d := file.Blocks[0].Enums[0]
	if len(d.Variants) != 4 {
		t.Errorf("expected 4 Direction variants, got %d", len(d.Variants))
	}
	s := file.Blocks[0].Enums[1]
	if len(s.Variants) != 2 {
		t.Errorf("expected 2 Shape variants, got %d", len(s.Variants))
	}
	if len(s.Variants[1].Fields) != 2 {
		t.Errorf("expected 2 Rectangle fields, got %d", len(s.Variants[1].Fields))
	}
}

func TestParseInterface(t *testing.T) {
	input := `grok Test {
  interface Writer {
    func write(mut self, data: [u8]) -> (u64, error)
    func close(mut self) -> unit
  }
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(file.Blocks[0].Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(file.Blocks[0].Interfaces))
	}
	iface := file.Blocks[0].Interfaces[0]
	if iface.Name != "Writer" {
		t.Errorf("expected Writer, got %s", iface.Name)
	}
	if len(iface.Methods) != 2 {
		t.Errorf("expected 2 methods, got %d", len(iface.Methods))
	}
}

func TestParseClass(t *testing.T) {
	input := `grok Test {
  class HttpClient(base_url: string, timeout: u32) {
    why: "Manages HTTP connections."

    pool: ConnectionPool guarded_by(mu)
    mu: lock

    func get(mut self, path: string) -> (Response, error)
      concurrent: true
      excludes_lock(mu)
      raises: Timeout, NotFound
  }
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(file.Blocks[0].Classes) != 1 {
		t.Fatalf("expected 1 class, got %d", len(file.Blocks[0].Classes))
	}
	cls := file.Blocks[0].Classes[0]
	if cls.Name != "HttpClient" {
		t.Errorf("expected HttpClient, got %s", cls.Name)
	}
	if len(cls.CtorParams) != 2 {
		t.Errorf("expected 2 ctor params, got %d", len(cls.CtorParams))
	}
	if len(cls.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(cls.Fields))
	}
	if cls.Fields[0].GuardedBy != "mu" {
		t.Errorf("expected guarded_by(mu), got %q", cls.Fields[0].GuardedBy)
	}
	if len(cls.Methods) != 1 {
		t.Errorf("expected 1 method, got %d", len(cls.Methods))
	}
	m := cls.Methods[0]
	if m.Annotations.Concurrent == nil || !*m.Annotations.Concurrent {
		t.Error("expected concurrent: true")
	}
	if len(m.Annotations.ExcludesLock) != 1 || m.Annotations.ExcludesLock[0] != "mu" {
		t.Errorf("expected excludes_lock(mu), got %v", m.Annotations.ExcludesLock)
	}
	if len(m.Annotations.Raises) != 2 {
		t.Errorf("expected 2 raises, got %v", m.Annotations.Raises)
	}
}

func TestParseRelation(t *testing.T) {
	input := `grok Test {
  relation DoublyLinked Node:outEdges owns [Edge:fromNode]
  relation Agent refs Config
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(file.Blocks[0].Relations) != 2 {
		t.Fatalf("expected 2 relations, got %d", len(file.Blocks[0].Relations))
	}
	r1 := file.Blocks[0].Relations[0]
	if r1.Hint != "DoublyLinked" {
		t.Errorf("expected hint DoublyLinked, got %q", r1.Hint)
	}
	if r1.Parent.TypeName != "Node" || r1.Parent.Label != "outEdges" {
		t.Errorf("unexpected parent: %+v", r1.Parent)
	}
	if r1.Kind != ast.Owns {
		t.Error("expected owns")
	}
	if !r1.IsMany {
		t.Error("expected one-to-many")
	}
	if r1.Child.TypeName != "Edge" || r1.Child.Label != "fromNode" {
		t.Errorf("unexpected child: %+v", r1.Child)
	}

	r2 := file.Blocks[0].Relations[1]
	if r2.Kind != ast.Refs {
		t.Error("expected refs")
	}
	if r2.IsMany {
		t.Error("expected one-to-one")
	}
}

func TestParseDoc(t *testing.T) {
	input := `grok Test {
  doc "Architecture": """
    This is the architecture.
    It spans lines.
  """
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(file.Blocks[0].Docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(file.Blocks[0].Docs))
	}
	if file.Blocks[0].Docs[0].Section != "Architecture" {
		t.Errorf("expected section Architecture, got %q", file.Blocks[0].Docs[0].Section)
	}
}

func TestParseInvariant(t *testing.T) {
	input := `grok Test {
  invariant: "count never exceeds max"
    verified_at: "a3f9c12"
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(file.Blocks[0].Invariants) != 1 {
		t.Fatalf("expected 1 invariant, got %d", len(file.Blocks[0].Invariants))
	}
	inv := file.Blocks[0].Invariants[0]
	if inv.Claim != "count never exceeds max" {
		t.Errorf("unexpected claim: %q", inv.Claim)
	}
	if inv.VerifiedAt != "a3f9c12" {
		t.Errorf("unexpected verified_at: %q", inv.VerifiedAt)
	}
}

func TestParseSource(t *testing.T) {
	input := `grok Test {
  source: ["pkg/foo.go", "pkg/bar.go"]
  fake: "pkg/fake_foo.go"
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(file.Blocks[0].Source) != 2 {
		t.Errorf("expected 2 source files, got %d", len(file.Blocks[0].Source))
	}
	if file.Blocks[0].Fake != "pkg/fake_foo.go" {
		t.Errorf("unexpected fake: %q", file.Blocks[0].Fake)
	}
}

func TestParseImport(t *testing.T) {
	input := `grok Test {
  import database from "database.grok"
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(file.Blocks[0].Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(file.Blocks[0].Imports))
	}
	if file.Blocks[0].Imports[0].Alias != "database" || file.Blocks[0].Imports[0].Path != "database.grok" {
		t.Errorf("unexpected import: %+v", file.Blocks[0].Imports[0])
	}
}

func TestParseGenericClass(t *testing.T) {
	input := `grok Test {
  class MutableStack<T>() {
    items: [T] guarded_by(mu)
    mu: lock

    func push(mut self, item: T) -> unit
      concurrent: true
      excludes_lock(mu)

    func pop(mut self) -> T?
      concurrent: true
      excludes_lock(mu)

    func peek(self) -> T?
      concurrent: true
      excludes_lock(mu)
  }
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	cls := file.Blocks[0].Classes[0]
	if cls.Name != "MutableStack" {
		t.Errorf("expected MutableStack, got %s", cls.Name)
	}
	if len(cls.TypeParams) != 1 || cls.TypeParams[0].Name != "T" {
		t.Errorf("expected type param T, got %v", cls.TypeParams)
	}
	if len(cls.Methods) != 3 {
		t.Errorf("expected 3 methods, got %d", len(cls.Methods))
	}
}

func TestParseWhereClause(t *testing.T) {
	input := `grok Test {
  func factorial(n: T) -> T
    where T: Integer
    requires: n >= 0
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	fn := file.Blocks[0].Functions[0]
	if len(fn.Where) != 1 {
		t.Fatalf("expected 1 where clause, got %d", len(fn.Where))
	}
	if fn.Where[0].Variable != "T" || fn.Where[0].Constraint != "Integer" {
		t.Errorf("unexpected where: %+v", fn.Where[0])
	}
	if len(fn.Annotations.Requires) != 1 {
		t.Errorf("expected 1 requires, got %v", fn.Annotations.Requires)
	}
}

func TestParseClassImplements(t *testing.T) {
	input := `grok Test {
  class MemBuf() implements Reader, Writer {
    data: [u8] guarded_by(mu)
    mu: lock
  }
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	cls := file.Blocks[0].Classes[0]
	if len(cls.Implements) != 2 {
		t.Errorf("expected 2 implements, got %v", cls.Implements)
	}
}

func TestParsePureFunc(t *testing.T) {
	input := `grok Test {
  func transform(xs: [T], f: T -> U) -> [U]
    pure:
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	fn := file.Blocks[0].Functions[0]
	if !fn.Annotations.Pure {
		t.Error("expected pure: true")
	}
}

// TestParseOwnGrokFile parses grok/parser.grok — the parser's own understanding.
func TestParseOwnGrokFile(t *testing.T) {
	data, err := os.ReadFile("../../grok/parser.grok")
	if err != nil {
		t.Skipf("skipping: %v", err)
	}
	file, err := ParseFile(string(data), "parser.grok")
	if err != nil {
		t.Fatalf("failed to parse own grok file: %v", err)
	}
	if len(file.Blocks) != 1 {
		t.Fatalf("expected 1 grok block, got %d", len(file.Blocks))
	}
	block := file.Blocks[0]
	if block.Name != "Parser" {
		t.Errorf("expected Parser, got %s", block.Name)
	}

	// Should have structs, enums, classes, and functions
	t.Logf("Parsed: %d structs, %d enums, %d classes, %d interfaces, %d functions, %d relations",
		len(block.Structs), len(block.Enums), len(block.Classes),
		len(block.Interfaces), len(block.Functions), len(block.Relations))

	if len(block.Structs) == 0 {
		t.Error("expected structs")
	}
	if len(block.Enums) == 0 {
		t.Error("expected enums")
	}
	if len(block.Classes) == 0 {
		t.Error("expected classes")
	}
}


func TestLexFString(t *testing.T) {
	input := `f"hello {name}!"`
	lex := NewLexer(input, "test.gk")
	tok := lex.Next()
	if tok.Kind != TFStringLit {
		t.Fatalf("expected TFStringLit, got %v", tokenNames[tok.Kind])
	}
	if tok.Text != "hello {name}!" {
		t.Errorf("expected raw text 'hello {name}!', got %q", tok.Text)
	}
}

func TestParseFString(t *testing.T) {
	input := `grok test {
  func main() {
    let x = f"hello {name}!"
  }
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	fn := file.Blocks[0].Functions[0]
	if fn.Body == nil || len(fn.Body.Stmts) == 0 {
		t.Fatal("expected function body")
	}
}


func TestParseCastExpr(t *testing.T) {
	input := `grok test {
  func f() {
    let x = <i64>42
    let y = <int>x
  }
}`
	file, err := ParseString(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	fn := file.Blocks[0].Functions[0]
	if fn.Body == nil || len(fn.Body.Stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(fn.Body.Stmts))
	}
	// First: let x = <i64>42
	decl := fn.Body.Stmts[0].Data.(*ast.VarDeclStmt)
	if decl.Value.Kind != ast.ExprCast {
		t.Fatalf("expected ExprCast, got %v", decl.Value.Kind)
	}
	cast := decl.Value.Data.(*ast.CastExpr)
	if cast.TargetType.Kind != ast.TypeNamed {
		t.Fatalf("expected TypeNamed, got %v", cast.TargetType.Kind)
	}
	nt := cast.TargetType.Data.(ast.NamedType)
	if nt.Name != "i64" {
		t.Errorf("expected target type i64, got %s", nt.Name)
	}
	if cast.Operand.Kind != ast.ExprIntLit {
		t.Errorf("expected IntLit operand, got %v", cast.Operand.Kind)
	}
}
