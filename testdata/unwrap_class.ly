// Test: unwrapping a nullable class should preserve the class type
class Foo {
  name: string
}

func main() {
  let f: Foo? = Foo { name: "hello" }
  let g = f!
  print(g.name)
}
