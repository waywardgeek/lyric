package checker

import (
	"strings"
	"testing"

	"github.com/waywardgeek/grok/pkg/parser"
)

func parseAndCheck(t *testing.T, source string) *Checker {
	t.Helper()
	file, err := parser.ParseString(source)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	c := New()
	c.CheckFile(file)
	return c
}

func expectNoErrors(t *testing.T, c *Checker) {
	t.Helper()
	if len(c.Errors()) > 0 {
		for _, e := range c.Errors() {
			t.Errorf("unexpected error: %v", e)
		}
	}
}

func expectErrors(t *testing.T, c *Checker, count int) {
	t.Helper()
	if len(c.Errors()) != count {
		t.Errorf("expected %d errors, got %d:", count, len(c.Errors()))
		for _, e := range c.Errors() {
			t.Errorf("  %v", e)
		}
	}
}

func TestTypeInferenceFromLiteral(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x = 42
			let y = "hello"
			let z = true
			let w = 3.14
		}
	}`)
	expectNoErrors(t, c)
}

func TestTypeAnnotationMatches(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x: i32 = 42
			let y: string = "hello"
		}
	}`)
	expectNoErrors(t, c)
}

func TestTypeMismatchInLetDecl(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x: string = 42
		}
	}`)
	expectErrors(t, c, 1)
}

func TestNoTypeNoInit(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x
		}
	}`)
	expectErrors(t, c, 1) // no type and no initializer
}

func TestArithmetic(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x: i32 = 1
			let y: i32 = 2
			let z = x + y
		}
	}`)
	expectNoErrors(t, c)
}

func TestArithmeticTypeMismatch(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x: i32 = 1
			let y: f64 = 2.0
			let z = x + y
		}
	}`)
	expectErrors(t, c, 1) // i32 + f64
}

func TestStringConcat(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x = "hello" + " world"
		}
	}`)
	expectNoErrors(t, c)
}

func TestBooleanLogic(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x = true && false
			let y = true || false
			let z = !true
		}
	}`)
	expectNoErrors(t, c)
}

func TestComparisonReturnsBool(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x: i32 = 1
			let y: i32 = 2
			let z = x < y
		}
	}`)
	expectNoErrors(t, c)
}

func TestUndefinedVariable(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x = y
		}
	}`)
	expectErrors(t, c, 1) // y undefined
}

func TestIfConditionMustBeBool(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			if 42 {
				let x = 1
			}
		}
	}`)
	expectErrors(t, c, 1) // condition is i32, not bool
}

func TestIfConditionBoolOk(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let done = true
			if done {
				let x = 1
			}
		}
	}`)
	expectNoErrors(t, c)
}

func TestWhileConditionMustBeBool(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			while 1 {
				let x = 1
			}
		}
	}`)
	expectErrors(t, c, 1)
}

func TestForLoopInfersElemType(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let items: [i32] = [1, 2, 3]
			for item in items {
				let x: i32 = item
			}
		}
	}`)
	expectNoErrors(t, c)
}

func TestForLoopElemTypeMismatch(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let items: [i32] = [1, 2, 3]
			for item in items {
				let x: string = item
			}
		}
	}`)
	expectErrors(t, c, 1)
}

func TestFunctionCall(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func add(x: i32, y: i32) -> i32 {
			return x + y
		}
		func main() {
			let result = add(1, 2)
		}
	}`)
	expectNoErrors(t, c)
}

func TestFunctionCallWrongArgCount(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func add(x: i32, y: i32) -> i32 {
			return x + y
		}
		func main() {
			let result = add(1)
		}
	}`)
	expectErrors(t, c, 1) // wrong arg count
}

func TestListLiteralHomogeneous(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let xs = [1, 2, 3]
		}
	}`)
	expectNoErrors(t, c)
}

func TestListLiteralHeterogeneous(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let xs = [1, "two", 3]
		}
	}`)
	expectErrors(t, c, 1) // string in i32 list
}

func TestStructFieldAccess(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		struct Point {
			x: f64
			y: f64
		}
		func f() {
			let p: Point = nil
			let x = p.x
		}
	}`)
	// p.x should resolve to f64
	expectNoErrors(t, c)
}

func TestStructFieldNotFound(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		struct Point {
			x: f64
			y: f64
		}
		func f() {
			let p: Point = nil
			let z = p.z
		}
	}`)
	expectErrors(t, c, 1) // no field z
}

func TestNegateNonNumeric(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x = -"hello"
		}
	}`)
	expectErrors(t, c, 1)
}

func TestNotNonBool(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x = !42
		}
	}`)
	expectErrors(t, c, 1)
}

func TestListIndexing(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let xs: [i32] = [1, 2, 3]
			let x = xs[0]
		}
	}`)
	expectNoErrors(t, c)
}

func TestAssignmentTypeCheck(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let mut x: i32 = 1
			x = 2
		}
	}`)
	expectNoErrors(t, c)
}

func TestAssignmentTypeMismatch(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let mut x: i32 = 1
			x = "hello"
		}
	}`)
	expectErrors(t, c, 1)
}

func TestScopeIsolation(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			if true {
				let x = 42
			}
			let y = x
		}
	}`)
	expectErrors(t, c, 1) // x not in scope after if block
}

func TestTypeEquality(t *testing.T) {
	if !TypeI32.Equal(IntType(32)) {
		t.Error("i32 should equal i32")
	}
	if TypeI32.Equal(TypeI64) {
		t.Error("i32 should not equal i64")
	}
	if !TypeBool.Equal(TypeBool) {
		t.Error("bool should equal bool")
	}
	if TypeBool.Equal(TypeString) {
		t.Error("bool should not equal string")
	}
	l1 := ListType(TypeI32)
	l2 := ListType(TypeI32)
	if !l1.Equal(l2) {
		t.Error("[i32] should equal [i32]")
	}
	l3 := ListType(TypeString)
	if l1.Equal(l3) {
		t.Error("[i32] should not equal [string]")
	}
}

func TestTypeString(t *testing.T) {
	tests := []struct {
		typ *Type
		str string
	}{
		{TypeI32, "i32"},
		{TypeBool, "bool"},
		{TypeString, "string"},
		{TypeUnit, "unit"},
		{ListType(TypeI32), "[i32]"},
		{MapType(TypeString, TypeI32), "map[string]i32"},
		{OptionalType(TypeString), "string?"},
	}
	for _, tt := range tests {
		if got := tt.typ.String(); got != tt.str {
			t.Errorf("expected %q, got %q", tt.str, got)
		}
	}
}


func TestStructLiteralTypeCheck(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		struct Point {
			X: f64
			Y: f64
		}
		func test() {
			let p = Point{X: 3.0, Y: 4.0}
			let _ = p
		}
	}`)
	expectNoErrors(t, c)
}

func TestStructLiteralBadField(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		struct Point {
			X: f64
			Y: f64
		}
		func test() {
			let p = Point{X: 3.0, Z: 4.0}
			let _ = p
		}
	}`)
	expectErrors(t, c, 1) // no field Z
}

func TestReturnTypeCheck(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func add(x: i32, y: i32) -> i32 {
			return x + y
		}
	}`)
	expectNoErrors(t, c)
}

func TestReturnTypeMismatch(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func bad() -> i32 {
			return true
		}
	}`)
	expectErrors(t, c, 1) // return bool, expected i32
}

func TestMissingReturnValue(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func bad() -> i32 {
			return
		}
	}`)
	expectErrors(t, c, 1) // missing return value
}

func TestMutabilityEnforcement(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func test() {
			let x: i32 = 1
			x = 2
		}
	}`)
	expectErrors(t, c, 1) // immutable variable
}

func TestMutabilityAllowed(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func test() {
			let mut x: i32 = 1
			x = 2
		}
	}`)
	expectNoErrors(t, c)
}

func TestBreakOutsideLoop(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			break
		}
	}`)
	expectErrors(t, c, 1)
}

func TestContinueOutsideLoop(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			continue
		}
	}`)
	expectErrors(t, c, 1)
}

func TestBreakInsideLoop(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			while true {
				break
			}
		}
	}`)
	expectNoErrors(t, c)
}

func TestContinueInsideForLoop(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let xs: [i32] = [1, 2, 3]
			for x in xs {
				continue
			}
		}
	}`)
	expectNoErrors(t, c)
}

func TestBreakNestedLoop(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			while true {
				while true {
					break
				}
				break
			}
		}
	}`)
	expectNoErrors(t, c)
}

func TestPatternBindingInMatch(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x: i32 = 42
			match x {
				y => {
					let z: i32 = y
				}
			}
		}
	}`)
	expectNoErrors(t, c)
}

func TestPatternBindingTypeMismatch(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x: i32 = 42
			match x {
				y => {
					let z: string = y
				}
			}
		}
	}`)
	expectErrors(t, c, 1) // y is i32, z declared string
}

func TestNumericWidening(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x: i32 = 1
			let y: i64 = x
		}
	}`)
	expectNoErrors(t, c) // i32 widens to i64
}

func TestNumericWideningInArithmetic(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x: i32 = 1
			let y: i64 = 2
			let z = x + y
		}
	}`)
	expectNoErrors(t, c) // i32 + i64 -> i64
}

func TestNumericNoNarrow(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x: i64 = 1
			let y: i32 = x
		}
	}`)
	expectErrors(t, c, 1) // i64 does NOT narrow to i32
}

func TestNumericCrossKindNoCoerce(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() {
			let x: i32 = 1
			let y: f64 = x
		}
	}`)
	expectErrors(t, c, 1) // int -> float requires explicit cast
}

func TestNumericWideningInReturn(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f() -> i64 {
			let x: i32 = 1
			return x
		}
	}`)
	expectNoErrors(t, c) // i32 return widens to i64
}

func TestNumericWideningInFuncArgs(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func take64(x: i64) -> i64 {
			return x
		}
		func f() {
			let x: i32 = 1
			let y = take64(x)
		}
	}`)
	expectNoErrors(t, c) // i32 arg widens to i64 param
}

func TestFloatWidening(t *testing.T) {
	c := parseAndCheck(t, `grok test {
		func f(x: f32) {
			let y: f64 = 1.0
			let z = x + y
		}
	}`)
	expectNoErrors(t, c) // f32 + f64 -> f64 via widening
}

func TestInterfaceRegistration(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  interface Greeter {
    func greet(self) -> string
  }
}`)
	expectNoErrors(t, c)
	info := c.registry.Lookup("Greeter")
	if info == nil {
		t.Fatal("Greeter not registered")
	}
	if info.Type.Kind != TyInterface {
		t.Errorf("expected TyInterface, got %v", info.Type.Kind)
	}
	if _, ok := info.Methods["greet"]; !ok {
		t.Error("method greet not found")
	}
}

func TestInterfaceImplementsSatisfied(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  interface Greeter {
    func greet(self) -> string
  }

  class Dog() implements Greeter {
    func greet(self) -> string {
      return "woof"
    }
  }
}`)
	expectNoErrors(t, c)
}

func TestInterfaceMissingMethod(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  interface Greeter {
    func greet(self) -> string
  }

  class Dog() implements Greeter {
    func bark(self) -> string {
      return "woof"
    }
  }
}`)
	expectErrors(t, c, 1)
}

func TestInterfaceWrongReturnType(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  interface Greeter {
    func greet(self) -> string
  }

  class Dog() implements Greeter {
    func greet(self) -> i32 {
      return 42
    }
  }
}`)
	expectErrors(t, c, 1)
}

func TestInterfaceWrongParamCount(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  interface Greeter {
    func greet(self) -> string
  }

  class Dog() implements Greeter {
    func greet(self, name: string) -> string {
      return "woof"
    }
  }
}`)
	expectErrors(t, c, 1)
}

func TestInterfaceComposition(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  interface Reader {
    func read(self) -> string
  }

  interface Writer {
    func write(self, data: string)
  }

  interface ReadWriter {
    implements Reader
    implements Writer
  }

  class File() implements ReadWriter {
    func read(self) -> string {
      return ""
    }
    func write(self, data: string) {
    }
  }
}`)
	expectNoErrors(t, c)
	// ReadWriter should have both read and write methods
	info := c.registry.Lookup("ReadWriter")
	if info == nil {
		t.Fatal("ReadWriter not registered")
	}
	if len(info.Methods) != 2 {
		t.Errorf("expected 2 methods, got %d", len(info.Methods))
	}
}

func TestInterfaceCompositionMissingMethod(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  interface Reader {
    func read(self) -> string
  }

  interface Writer {
    func write(self, data: string)
  }

  interface ReadWriter {
    implements Reader
    implements Writer
  }

  class File() implements ReadWriter {
    func read(self) -> string {
      return ""
    }
  }
}`)
	expectErrors(t, c, 1) // missing write
}

func TestInterfaceSubtyping(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  interface Greeter {
    func greet(self) -> string
  }

  class Dog() implements Greeter {
    func greet(self) -> string {
      return "woof"
    }
  }

  func say_hello(g: Greeter) -> string {
    return g.greet()
  }

  func main() {
    let d = Dog{}
    let result = say_hello(d)
  }
}`)
	expectNoErrors(t, c)
}

func TestInterfaceSubtypingFails(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  interface Greeter {
    func greet(self) -> string
  }

  class Cat() {
    func meow(self) -> string {
      return "meow"
    }
  }

  func say_hello(g: Greeter) -> string {
    return g.greet()
  }

  func main() {
    let c = Cat{}
    let result = say_hello(c)
  }
}`)
	expectErrors(t, c, 1) // Cat doesn't implement Greeter
}


func TestFStringTypeChecks(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  func main() {
    let name = "world"
    let greeting = f"hello {name}!"
  }
}`)
	expectNoErrors(t, c)
}
func TestCastNumeric(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  func f() {
    let x: i32 = 42
    let y: i64 = <i64>x
    let z: i32 = <i32>y
  }
}`)
	expectNoErrors(t, c)
}

func TestCastInvalidNonNumeric(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  func f() {
    let s: string = "hello"
    let x = <i32>s
  }
}`)
	expectErrors(t, c, 1)
	if len(c.Errors()) > 0 && !strings.Contains(c.Errors()[0].Error(), "cannot cast") {
		t.Errorf("expected cast error, got: %s", c.Errors()[0])
	}
}

func TestCastPlatformInt(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  func f() {
    let x: i32 = 42
    let y = <int>x
  }
}`)
	expectNoErrors(t, c)
}


func TestEnumVariantConstructor(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  enum Color {
    Red
    Green
    RGB(r: i32, g: i32, b: i32)
  }
  func f() {
    let a = Red
    let b = RGB(255, 128, 0)
    let _ = a
    let _ = b
  }
}`)
	expectNoErrors(t, c)
}

func TestEnumVariantConstructorWrongArgs(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  enum Color {
    RGB(r: i32, g: i32, b: i32)
  }
  func f() {
    let _ = RGB(255, 128)
  }
}`)
	expectErrors(t, c, 1)
}

func TestEnumVariantPatternTyped(t *testing.T) {
	c := parseAndCheck(t, `grok test {
  enum Shape {
    Circle(radius: f64)
    Empty
  }
  func f(s: Shape) -> f64 {
    return match s {
      Circle(r) => { r * r }
      Empty => { 0.0 }
    }
  }
}`)
	expectNoErrors(t, c)
}
