// generators.ly — tests gen T and yield
lyric generators {

    func count_up(n: i32) -> gen i32 {
        let mut i: i32 = 0
        while i < n {
            yield i
            i = i + 1
        }
    }

    func fibonacci(limit: i32) -> gen i32 {
        let mut a: i32 = 0
        let mut b: i32 = 1
        while a < limit {
            yield a
            let tmp = a + b
            a = b
            b = tmp
        }
    }

    func main() {
        // Basic generator
        println("count:")
        for x in count_up(5) {
            println(f"{x}")
        }

        // Fibonacci generator
        println("fib:")
        for f in fibonacci(50) {
            println(f"{f}")
        }
    }
}
