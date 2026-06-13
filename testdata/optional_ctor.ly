// optional_ctor.ly — test optional fields in class constructors auto-wrap

lyric optional_ctor {

class User {
    name: string
    email: string?

    func get_name(self) -> string {
        return self.name
    }
}

func main() {
    let u1 = User { name: "Alice", email: "alice@example.com" }
    let u2 = User { name: "Bob", email: null }
    println(u1.get_name())
    if !isnull(u2.email) {
        println("has email")
    }
}

}
