// classes.ly — end-to-end test for classes, methods, generic classes

lyric classes {

class Counter {
    count: i32

    func increment(self) {
        self.count = self.count + 1
    }

    func get(self) -> i32 {
        return self.count
    }
}

class Pair<T> {
    first: T
    second: T

    func swap(self) {
        let tmp = self.first
        self.first = self.second
        self.second = tmp
    }
}

class Stack<T> {
    items: [T]

    func push(self, item: T) {
        self.items = append(self.items, item)
    }

    func pop(self) -> T? {
        if len(self.items) == 0 {
            return null
        }
        let last = self.items[len(self.items) - 1]
        self.items = self.items[:len(self.items) - 1]
        return last
    }

    func size(self) -> i32 {
        return len(self.items)
    }
}

func main() {
    let c = Counter { count: 0 }
    c.increment()
    c.increment()
    c.increment()
    println(c.get())

    let p = Pair<string> { first: "hello", second: "world" }
    println(p.first)
    p.swap()
    println(p.first)

    let s = Stack<i32> {}
    s.push(10)
    s.push(20)
    s.push(30)
    println(s.size())
    let v = s.pop()
    if !isnull(v) {
        println(v!)
    }
    println(s.size())
}

}
