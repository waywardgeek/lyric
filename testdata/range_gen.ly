// Test: range() generator function from stdlib
// Verifies that for..in range(start, end) works correctly.

func main() {
  // Basic range
  for i in range(0, 5) {
    println(i)
  }

  // Range with non-zero start
  let mut sum = 0
  for i in range(3, 7) {
    sum = sum + i
  }
  println(sum)  // 3+4+5+6 = 18
}
