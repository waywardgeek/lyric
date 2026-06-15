package checker

import (
	"os"
	"strings"
	"testing"

	"github.com/waywardgeek/forge/pkg/parser"
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
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
		func f() {
			let x: i32 = 42
			let y: string = "hello"
		}
	}`)
	expectNoErrors(t, c)
}

func TestTypeMismatchInLetDecl(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x: string = 42
		}
	}`)
	expectErrors(t, c, 1)
}

func TestNoTypeNoInit(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x
		}
	}`)
	expectErrors(t, c, 1) // no type and no initializer
}

func TestArithmetic(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x: i32 = 1
			let y: i32 = 2
			let z = x + y
		}
	}`)
	expectNoErrors(t, c)
}

func TestArithmeticTypeMismatch(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x: i32 = 1
			let y: f64 = 2.0
			let z = x + y
		}
	}`)
	expectErrors(t, c, 1) // i32 + f64
}

func TestStringConcat(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x = "hello" + " world"
		}
	}`)
	expectNoErrors(t, c)
}

func TestBooleanLogic(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x = true && false
			let y = true || false
			let z = !true
		}
	}`)
	expectNoErrors(t, c)
}

func TestComparisonReturnsBool(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x: i32 = 1
			let y: i32 = 2
			let z = x < y
		}
	}`)
	expectNoErrors(t, c)
}

func TestUndefinedVariable(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x = y
		}
	}`)
	expectErrors(t, c, 1) // y undefined
}

func TestIfConditionMustBeBool(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			if 42 {
				let x = 1
			}
		}
	}`)
	expectErrors(t, c, 1) // condition is i32, not bool
}

func TestIfConditionBoolOk(t *testing.T) {
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
		func f() {
			while 1 {
				let x = 1
			}
		}
	}`)
	expectErrors(t, c, 1)
}

func TestForLoopInfersElemType(t *testing.T) {
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
		func f() {
			let xs = [1, 2, 3]
		}
	}`)
	expectNoErrors(t, c)
}

func TestListLiteralHeterogeneous(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let xs = [1, "two", 3]
		}
	}`)
	expectErrors(t, c, 1) // string in i32 list
}

func TestStructFieldAccess(t *testing.T) {
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
		func f() {
			let x = -"hello"
		}
	}`)
	expectErrors(t, c, 1)
}

func TestNotNonBool(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x = !42
		}
	}`)
	expectErrors(t, c, 1)
}

func TestListIndexing(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let xs: [i32] = [1, 2, 3]
			let x = xs[0]
		}
	}`)
	expectNoErrors(t, c)
}

func TestAssignmentTypeCheck(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let mut x: i32 = 1
			x = 2
		}
	}`)
	expectNoErrors(t, c)
}

func TestAssignmentTypeMismatch(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let mut x: i32 = 1
			x = "hello"
		}
	}`)
	expectErrors(t, c, 1)
}

func TestScopeIsolation(t *testing.T) {
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
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

func TestExternalQualifiedStructLiteral(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		import sync from "sync"

		func test() {
			let mut mu = sync.Mutex{}
			lock(mu) {
				println("ok")
			}
		}
	}`)
	expectNoErrors(t, c)
}

func TestReturnTypeCheck(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func add(x: i32, y: i32) -> i32 {
			return x + y
		}
	}`)
	expectNoErrors(t, c)
}

func TestReturnTypeMismatch(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func bad() -> i32 {
			return true
		}
	}`)
	expectErrors(t, c, 1) // return bool, expected i32
}

func TestMissingReturnValue(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func bad() -> i32 {
			return
		}
	}`)
	expectErrors(t, c, 1) // missing return value
}

func TestMutabilityEnforcement(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func test() {
			let x: i32 = 1
			x = 2
		}
	}`)
	expectErrors(t, c, 1) // immutable variable
}

func TestMutabilityAllowed(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func test() {
			let mut x: i32 = 1
			x = 2
		}
	}`)
	expectNoErrors(t, c)
}

func TestBreakOutsideLoop(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			break
		}
	}`)
	expectErrors(t, c, 1)
}

func TestContinueOutsideLoop(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			continue
		}
	}`)
	expectErrors(t, c, 1)
}

func TestBreakInsideLoop(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			while true {
				break
			}
		}
	}`)
	expectNoErrors(t, c)
}

func TestContinueInsideForLoop(t *testing.T) {
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
		func f() {
			let x: i32 = 1
			let y: i64 = x
		}
	}`)
	expectNoErrors(t, c) // i32 widens to i64
}

func TestNumericWideningInArithmetic(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x: i32 = 1
			let y: i64 = 2
			let z = x + y
		}
	}`)
	expectNoErrors(t, c) // i32 + i64 -> i64
}

func TestNumericNoNarrow(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x: i64 = 1
			let y: i32 = x
		}
	}`)
	expectErrors(t, c, 1) // i64 does NOT narrow to i32
}

func TestNumericCrossKindNoCoerce(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x: i32 = 1
			let y: f64 = x
		}
	}`)
	expectErrors(t, c, 1) // int -> float requires explicit cast
}

func TestNumericWideningInReturn(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() -> i64 {
			let x: i32 = 1
			return x
		}
	}`)
	expectNoErrors(t, c) // i32 return widens to i64
}

func TestNumericWideningInFuncArgs(t *testing.T) {
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
		func f(x: f32) {
			let y: f64 = 1.0
			let z = x + y
		}
	}`)
	expectNoErrors(t, c) // f32 + f64 -> f64 via widening
}

func TestInterfaceRegistration(t *testing.T) {
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
  interface Greeter {
    func greet(self) -> string
  }

  class Dog implements Greeter {
    func greet(self) -> string {
      return "woof"
    }
  }
}`)
	expectNoErrors(t, c)
}

func TestInterfaceMissingMethod(t *testing.T) {
	c := parseAndCheck(t, `forge test {
  interface Greeter {
    func greet(self) -> string
  }

  class Dog implements Greeter {
    func bark(self) -> string {
      return "woof"
    }
  }
}`)
	expectErrors(t, c, 1)
}

func TestInterfaceWrongReturnType(t *testing.T) {
	c := parseAndCheck(t, `forge test {
  interface Greeter {
    func greet(self) -> string
  }

  class Dog implements Greeter {
    func greet(self) -> i32 {
      return 42
    }
  }
}`)
	expectErrors(t, c, 1)
}

func TestInterfaceWrongParamCount(t *testing.T) {
	c := parseAndCheck(t, `forge test {
  interface Greeter {
    func greet(self) -> string
  }

  class Dog implements Greeter {
    func greet(self, name: string) -> string {
      return "woof"
    }
  }
}`)
	expectErrors(t, c, 1)
}

func TestInterfaceComposition(t *testing.T) {
	c := parseAndCheck(t, `forge test {
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

  class File implements ReadWriter {
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
	c := parseAndCheck(t, `forge test {
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

  class File implements ReadWriter {
    func read(self) -> string {
      return ""
    }
  }
}`)
	expectErrors(t, c, 1) // missing write
}

func TestInterfaceSubtyping(t *testing.T) {
	c := parseAndCheck(t, `forge test {
  interface Greeter {
    func greet(self) -> string
  }

  class Dog implements Greeter {
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
	c := parseAndCheck(t, `forge test {
  interface Greeter {
    func greet(self) -> string
  }

  class Cat {
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
	c := parseAndCheck(t, `forge test {
  func main() {
    let name = "world"
    let greeting = f"hello {name}!"
  }
}`)
	expectNoErrors(t, c)
}
func TestCastNumeric(t *testing.T) {
	c := parseAndCheck(t, `forge test {
  func f() {
    let x: i32 = 42
    let y: i64 = x as i64
    let z: i32 = y as i32
  }
}`)
	expectNoErrors(t, c)
}

func TestCastInvalidNonNumeric(t *testing.T) {
	c := parseAndCheck(t, `forge test {
  func f() {
    let s: string = "hello"
    let x = s as i32
  }
}`)
	expectErrors(t, c, 1)
	if len(c.Errors()) > 0 && !strings.Contains(c.Errors()[0].Error(), "cannot cast") {
		t.Errorf("expected cast error, got: %s", c.Errors()[0])
	}
}

func TestCastPlatformInt(t *testing.T) {
	c := parseAndCheck(t, `forge test {
  func f() {
    let x: i32 = 42
    let y = x as int
  }
}`)
	expectNoErrors(t, c)
}


func TestEnumVariantConstructor(t *testing.T) {
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
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
	c := parseAndCheck(t, `forge test {
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

func TestTupleReturnType(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func divide(a: i32, b: i32) -> (i32, error) {
			return (0, nil)
		}
	}`)
	expectNoErrors(t, c)
}

func TestTupleDestructuring(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func getTwo() -> (i32, string) {
			return (42, "hello")
		}
		func main() {
			let (x, y) = getTwo()
		}
	}`)
	expectNoErrors(t, c)
}

func TestTupleReturnMismatch(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func bad() -> (i32, string) {
			return (true, 42)
		}
	}`)
	expectErrors(t, c, 1) // tuple type mismatch
}

func TestErrorType(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func mayFail() -> (i32, error) {
			if true {
				return (0, nil)
			}
			return (42, nil)
		}
	}`)
	expectNoErrors(t, c)
}

func TestGenericFuncTypeCheck(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func identity<T>(x: T) -> T {
			return x
		}
		func main() {
			let a = identity<i32>(42)
		}
	}`)
	expectNoErrors(t, c)
}

func TestGenericFuncTypeMismatch(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func first<T>(a: T, b: T) -> T {
			return a
		}
		func main() {
			let a = first<i32>(42, "hello")
		}
	}`)
	expectErrors(t, c, 1) // arg 2: expected i32, got string
}

func TestGenericFuncWrongTypeArgCount(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func pair<T, U>(a: T, b: U) -> T {
			return a
		}
		func main() {
			let a = pair<i32>(42, "hello")
		}
	}`)
	expectErrors(t, c, 1) // expected 2 type arguments, got 1
}

func TestGenericReturnTypeSubstitution(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func identity<T>(x: T) -> T {
			return x
		}
		func main() {
			let a: i32 = identity<i32>(42)
			let b: string = identity<string>("hello")
		}
	}`)
	expectNoErrors(t, c)
}

func TestUnwrapOptional(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f(x: i32?) -> i32 {
			return x!
		}
	}`)
	expectNoErrors(t, c)
}

func TestUnwrapNonOptionalError(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f(x: i32) -> i32 {
			return x!
		}
	}`)
	if len(c.Errors()) == 0 {
		t.Error("expected error for unwrapping non-optional type")
	}
}

func TestAssignToOptional(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() -> i32? {
			return 42
		}
	}`)
	expectNoErrors(t, c)
}

func TestIsnullBuiltin(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f(x: i32?) -> bool {
			return isnull(x)
		}
	}`)
	expectNoErrors(t, c)
}

func TestLenBuiltin(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f(xs: [i32]) -> i32 {
			return len(xs)
		}
	}`)
	expectNoErrors(t, c)
}


func TestBuiltinStringMethods(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let s = "hello"
			let l = s.len()
			let u = s.to_upper()
			let lo = s.to_lower()
			let b = s.contains("ell")
			let p = s.has_prefix("he")
			let sx = s.has_suffix("lo")
			let r = s.replace("l", "r")
			let parts = s.split(",")
			let idx = s.index_of("ll")
			let t = s.trim()
			let rep = s.repeat(3)
		}
	}`)
	expectNoErrors(t, c)
}

func TestBuiltinStringMethodBadArgs(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let s = "hello"
			let _ = s.contains(42)
		}
	}`)
	expectErrors(t, c, 1)
}

func TestBuiltinListMethods(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let mut xs: [i32] = []
			xs.push(10)
			let l = xs.len()
			let b = xs.contains(10)
			let last = xs.pop()
		}
	}`)
	expectNoErrors(t, c)
}

func TestBuiltinListJoin(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let words = ["a", "b", "c"]
			let joined = words.join(", ")
		}
	}`)
	expectNoErrors(t, c)
}

func TestBuiltinMapMethods(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let m = map[string]i32{"a": 1}
			let l = m.len()
			let b = m.contains_key("a")
			let ks = m.keys()
			let vs = m.values()
		}
	}`)
	expectNoErrors(t, c)
}


func TestTypeInferenceSingleParam(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func identity<T>(x: T) -> T { return x }
		func f() {
			let x = identity(42)
			let s = identity("hello")
		}
	}`)
	expectNoErrors(t, c)
}

func TestTypeInferenceFromList(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func first<T>(xs: [T]) -> T? {
			return nil
		}
		func f() {
			let nums: [i32] = [1, 2, 3]
			let f = first(nums)
		}
	}`)
	expectNoErrors(t, c)
}

func TestTypeInferenceConstraint(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func max_val<T: Comparable>(a: T, b: T) -> T {
			if a > b { return a }
			return b
		}
		func f() {
			let m = max_val(10, 20)
		}
	}`)
	expectNoErrors(t, c)
}

func TestTypeInferenceExplicitStillWorks(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func identity<T>(x: T) -> T { return x }
		func f() {
			let x = identity<i64>(100)
		}
	}`)
	expectNoErrors(t, c)
}

func TestTypeInferenceNoArgsRequiresExplicit(t *testing.T) {
	// Zero-arg generic functions can't infer — uses TyVar passthrough
	c := parseAndCheck(t, `forge test {
		func make_list<T>() -> [T] {
			let xs: [T] = []
			return xs
		}
		func f() {
			let x = make_list<i32>()
		}
	}`)
	expectNoErrors(t, c)
}

func TestLambdaTypeChecking(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let add = |a: i32, b: i32| -> i32 { return a + b }
		}
	}`)
	expectNoErrors(t, c)
}

func TestLambdaInferenceMultiTypeParam(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func transform<T, U>(xs: [T], f: (T) -> U) -> [U] {
			let mut result: [U] = []
			return result
		}
		func f() {
			let nums: [i32] = [1, 2]
			let to_str = |n: i32| -> string { return "x" }
			let result = transform(nums, to_str)
		}
	}`)
	expectNoErrors(t, c)
}

func TestUnionTypeBasic(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let a: string | i32 = 42
			let b: string | i32 = "hello"
		}
	}`)
	expectNoErrors(t, c)
}

func TestUnionTypeParam(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func describe(val: string | i32) -> string {
			return "got a value"
		}
		func f() {
			describe(42)
			describe("hello")
		}
	}`)
	expectNoErrors(t, c)
}

func TestUnionTypeThreeVariants(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x: string | i32 | bool = true
		}
	}`)
	expectNoErrors(t, c)
}

func TestUnionTypeMismatch(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() {
			let x: string | i32 = true
		}
	}`)
	if len(c.Errors()) == 0 {
		t.Error("expected error for assigning bool to string | i32")
	}
}

func TestUnionMatchTypes(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f(val: string | i32) -> string {
			return match val {
				string => { "str" }
				i32 => { "int" }
			}
		}
	}`)
	if len(c.Errors()) > 0 {
		t.Errorf("unexpected errors: %v", c.Errors())
	}
}

func TestUnionMatchInvalidType(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f(val: string | i32) {
			match val {
				bool => { println("oops") }
			}
		}
	}`)
	if len(c.Errors()) == 0 {
		t.Error("expected error for matching non-member type 'bool'")
	}
}

func TestConstraintViolation(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func max<T: Comparable>(a: T, b: T) -> T {
			if a > b { return a }
			return b
		}
		func f() {
			let xs = [1, 2, 3]
			let r = max<[i32]>(xs, xs)
		}
	}`)
	if len(c.Errors()) == 0 {
		t.Error("expected error for [i32] not satisfying Comparable constraint")
	}
}

func TestConstraintSatisfied(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func max<T: Comparable>(a: T, b: T) -> T {
			if a > b { return a }
			return b
		}
		func f() {
			let r = max<i32>(1, 2)
		}
	}`)
	if len(c.Errors()) > 0 {
		t.Errorf("unexpected errors: %v", c.Errors())
	}
}

func TestConstraintInferred(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func max<T: Comparable>(a: T, b: T) -> T {
			if a > b { return a }
			return b
		}
		func f() {
			let r = max("hello", "world")
		}
	}`)
	if len(c.Errors()) > 0 {
		t.Errorf("unexpected errors: %v", c.Errors())
	}
}

func TestNullKeyword(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func main() {
			let x: string? = null
		}
	}`)
	expectNoErrors(t, c)
}

func TestNullWithoutTypeAnnotation(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func main() {
			let x = null
		}
	}`)
	if len(c.Errors()) == 0 {
		t.Fatal("expected error for null without type annotation")
	}
	found := false
	for _, e := range c.Errors() {
		if strings.Contains(e.Error(), "cannot infer type of null") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'cannot infer type of null' error, got: %v", c.Errors())
	}
}

func TestEnumMatchExhaustive(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		enum Shape {
			Circle(radius: f64)
			Square(side: f64)
		}
		func describe(s: Shape) {
			match s {
				Circle(r) => { println(r) }
				Square(s) => { println(s) }
			}
		}
	}`)
	expectNoErrors(t, c)
}

func TestEnumMatchNonExhaustive(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		enum Shape {
			Circle(radius: f64)
			Square(side: f64)
			Triangle(base: f64, height: f64)
		}
		func describe(s: Shape) {
			match s {
				Circle(r) => { println(r) }
			}
		}
	}`)
	if len(c.Errors()) == 0 {
		t.Fatal("expected non-exhaustive match error")
	}
	found := false
	for _, e := range c.Errors() {
		if strings.Contains(e.Error(), "non-exhaustive match") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'non-exhaustive match' error, got: %v", c.Errors())
	}
}

func TestEnumMatchWildcardExhaustive(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		enum Shape {
			Circle(radius: f64)
			Square(side: f64)
		}
		func describe(s: Shape) {
			match s {
				Circle(r) => { println(r) }
				_ => { println("other") }
			}
		}
	}`)
	expectNoErrors(t, c)
}

func TestTypeAlias(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		type StringList = [string]
		func f() {
			let names: StringList = ["hello"]
		}
	}`)
	expectNoErrors(t, c)
}

func TestTypeAliasUnion(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		type Result = i32 | string
		func f() {
			let x: Result = 42
		}
	}`)
	expectNoErrors(t, c)
}

func TestTryOperatorValid(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		import errors from "errors"
		func divide(a: i32, b: i32) -> (i32, error) {
			if b == 0 {
				return (0, errors.New("div by zero"))
			}
			return (a / b, nil)
		}
		func compute(x: i32) -> (i32, error) {
			let result = divide(x, 2)?
			return (result, nil)
		}
	}`)
	expectNoErrors(t, c)
}

func TestTryOperatorNotErrorReturn(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		import errors from "errors"
		func divide(a: i32, b: i32) -> (i32, error) {
			return (a / b, nil)
		}
		func compute(x: i32) -> i32 {
			let result = divide(x, 2)?
			return result
		}
	}`)
	expectErrors(t, c, 1)
}

func TestTryOperatorNonTupleOperand(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		func f() -> (i32, error) {
			let x: i32 = 42
			let y = x?
			return (y, nil)
		}
	}`)
	expectErrors(t, c, 1)
}

func TestTryOperatorExprStmt(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		import errors from "errors"
		func side_effect() -> (i32, error) {
			return (0, nil)
		}
		func run() -> (i32, error) {
			side_effect()?
			return (1, nil)
		}
	}`)
	expectNoErrors(t, c)
}


func TestUserDefinedConstraintSatisfied(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		interface Printable {
			func to_string(self) -> string
		}
		class Dog {
			name: string
			func to_string(self) -> string {
				return self.name
			}
		}
		func print_it<T: Printable>(item: T) -> string {
			return item.to_string()
		}
		func main() {
			let d = Dog { name: "Rex" }
			let r = print_it<Dog>(d)
		}
	}`)
	expectNoErrors(t, c)
}

func TestUserDefinedConstraintViolated(t *testing.T) {
	c := parseAndCheck(t, `forge test {
		interface Printable {
			func to_string(self) -> string
		}
		class Cat {
			name: string
		}
		func print_it<T: Printable>(item: T) -> string {
			return item.to_string()
		}
		func main() {
			let c = Cat { name: "Whiskers" }
			let r = print_it<Cat>(c)
		}
	}`)
	if len(c.Errors()) == 0 {
		t.Error("expected error for Cat not satisfying Printable constraint")
	}
}


func TestModuleImport(t *testing.T) {
	// Create a temporary .fg module file
	dir := t.TempDir()
	mathFile := dir + "/mathlib.fg"
	os.WriteFile(mathFile, []byte(`forge mathlib {
		pub struct Point {
			x: i32
			y: i32
		}
		pub func add(a: i32, b: i32) -> i32 {
			return a + b
		}
		func private_helper() -> i32 {
			return 42
		}
	}`), 0644)

	// Parse the main file that imports the module
	mainSrc := `forge main {
		import math from "` + mathFile + `"
		func test() -> i32 {
			let result = math.add(1, 2)
			return result
		}
	}`
	file, err := parser.ParseString(mainSrc)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	c := New()
	c.CheckFile(file)
	expectNoErrors(t, c)
}

func TestModuleImportUndefinedSymbol(t *testing.T) {
	dir := t.TempDir()
	mathFile := dir + "/mathlib.fg"
	os.WriteFile(mathFile, []byte(`forge mathlib {
		pub func add(a: i32, b: i32) -> i32 {
			return a + b
		}
	}`), 0644)

	mainSrc := `forge main {
		import math from "` + mathFile + `"
		func test() -> i32 {
			return math.nonexistent(1, 2)
		}
	}`
	file, err := parser.ParseString(mainSrc)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	c := New()
	c.CheckFile(file)
	if len(c.Errors()) == 0 {
		t.Error("expected error for undefined module symbol")
	}
}

func TestModuleImportPrivateNotExported(t *testing.T) {
	dir := t.TempDir()
	mathFile := dir + "/mathlib.fg"
	os.WriteFile(mathFile, []byte(`forge mathlib {
		func private_fn() -> i32 {
			return 42
		}
	}`), 0644)

	mainSrc := `forge main {
		import math from "` + mathFile + `"
		func test() -> i32 {
			return math.private_fn()
		}
	}`
	file, err := parser.ParseString(mainSrc)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	c := New()
	c.CheckFile(file)
	if len(c.Errors()) == 0 {
		t.Error("expected error for private function access")
	}
}

func TestModuleImportCycle(t *testing.T) {
	dir := t.TempDir()
	aFile := dir + "/a.fg"
	bFile := dir + "/b.fg"
	os.WriteFile(aFile, []byte(`forge a {
		import b from "`+bFile+`"
	}`), 0644)
	os.WriteFile(bFile, []byte(`forge b {
		import a from "`+aFile+`"
	}`), 0644)

	src, _ := os.ReadFile(aFile)
	file, err := parser.ParseFile(string(src), aFile)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	c := New()
	c.CheckFile(file)
	hasImportCycle := false
	for _, e := range c.Errors() {
		if strings.Contains(e.Error(), "import cycle") {
			hasImportCycle = true
		}
	}
	if !hasImportCycle {
		t.Error("expected import cycle error")
	}
}

func TestModuleImportFileNotFound(t *testing.T) {
	mainSrc := `forge main {
		import math from "/nonexistent/path/math.fg"
		func test() -> i32 {
			return math.add(1, 2)
		}
	}`
	file, err := parser.ParseString(mainSrc)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	c := New()
	c.CheckFile(file)
	if len(c.Errors()) == 0 {
		t.Error("expected error for missing module file")
	}
}

func TestGuardedByAccessInsideLock(t *testing.T) {
	c := parseAndCheck(t, `forge test {
  class Counter {
    count: i32 guarded_by(mu)
    mu: lock

    pub func increment(mut self) {
      lock(self.mu) {
        self.count = self.count + 1
      }
    }
  }
}`)
	expectNoErrors(t, c)
}

func TestGuardedByAccessOutsideLock(t *testing.T) {
	c := parseAndCheck(t, `forge test {
  class Counter {
    count: i32 guarded_by(mu)
    mu: lock

    pub func bad_read(self) -> i32 {
      return self.count
    }
  }
}`)
	expectErrors(t, c, 1)
	if len(c.Errors()) > 0 && !strings.Contains(c.Errors()[0].Error(), "guarded by") {
		t.Errorf("expected guarded_by error, got: %v", c.Errors()[0])
	}
}

func TestGuardedByWrongLock(t *testing.T) {
	c := parseAndCheck(t, `forge test {
  class TwoLocks {
    data: i32 guarded_by(mu1)
    mu1: lock
    mu2: lock

    pub func wrong_lock(mut self) {
      lock(self.mu2) {
        self.data = 42
      }
    }
  }
}`)
	expectErrors(t, c, 1)
}

func TestGuardedByFreeFieldAccess(t *testing.T) {
	c := parseAndCheck(t, `forge test {
  class Mixed {
    label: string
    count: i32 guarded_by(mu)
    mu: lock

    pub func get_label(self) -> string {
      return self.label
    }
  }
}`)
	expectNoErrors(t, c)
}

func TestGuardedByTopLevelLock(t *testing.T) {
	c := parseAndCheck(t, `forge test {
  class Counter {
    count: i32 guarded_by(mu)
    mu: lock
  }

  func main() {
    let c = Counter()
    lock(c.mu) {
      let val = c.count
      println(val)
    }
  }
}`)
	expectNoErrors(t, c)
}


func TestStructFieldDefaultHasResolvedType(t *testing.T) {
	c := parseAndCheck(t, `forge test {
  struct Point {
    x: i32 = 0
    y: i32 = 1
  }
  func main() {
    let p = Point{}
  }
}`)
	expectNoErrors(t, c)
}
