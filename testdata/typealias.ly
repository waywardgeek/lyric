// testdata/typealias.ly — type alias tests

lyric typealias {
    type StringList = [string]

    func test_aliases() {
        let names: StringList = ["alice", "bob"]
        println(names.len())
        println(f"first: {names[0]}")
    }

    func main() {
        test_aliases()
    }
}
