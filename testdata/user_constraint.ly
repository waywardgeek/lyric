// Test: user-defined constraint validation

lyric constraint_test {

pub interface Printable {
  func to_string(self) -> string
}

class Dog {
  name: string

  pub func to_string(self) -> string {
    return self.name
  }
}

func print_it<T: Printable>(item: T) -> string {
  return item.to_string()
}

func main() {
  let d = Dog { name: "Rex" }
  let result = print_it<Dog>(d)
  println(result)
}

}
