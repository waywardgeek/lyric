lyric slab_test {

class Node {
    name: string
    children: [Node?]
}

func greet(n: Node?) {
    println(n.name)
}

func main() {
    let parent = Node { name: "root", children: [] }
    let child = Node { name: "leaf", children: [] }
    append(parent.children, child)
    println(parent.name)
    println(len(parent.children))
    let c = parent.children[0]
    greet(c)
}

}
