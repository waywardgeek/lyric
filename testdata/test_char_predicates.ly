// test_char_predicates.ly — exercise the full char_is_* family listed in
// spec §Built-in Functions. Each predicate is u8 -> bool. We cover every
// predicate with both a true case and a false case so a missing C-backend
// arm (or a wrong dispatch) fails loudly.

lyric test_char_predicates {
  pub func main() {
    // char_is_digit
    assert_eq(char_is_digit('0'), true,  "char_is_digit('0')")
    assert_eq(char_is_digit('9'), true,  "char_is_digit('9')")
    assert_eq(char_is_digit('a'), false, "char_is_digit('a')")
    assert_eq(char_is_digit(' '), false, "char_is_digit(' ')")

    // char_is_alpha
    assert_eq(char_is_alpha('a'), true,  "char_is_alpha('a')")
    assert_eq(char_is_alpha('Z'), true,  "char_is_alpha('Z')")
    assert_eq(char_is_alpha('0'), false, "char_is_alpha('0')")
    assert_eq(char_is_alpha(' '), false, "char_is_alpha(' ')")

    // char_is_alnum
    assert_eq(char_is_alnum('a'), true,  "char_is_alnum('a')")
    assert_eq(char_is_alnum('Z'), true,  "char_is_alnum('Z')")
    assert_eq(char_is_alnum('5'), true,  "char_is_alnum('5')")
    assert_eq(char_is_alnum(' '), false, "char_is_alnum(' ')")
    assert_eq(char_is_alnum('!'), false, "char_is_alnum('!')")

    // char_is_space
    assert_eq(char_is_space(' '),  true,  "char_is_space(' ')")
    assert_eq(char_is_space('\t'), true,  "char_is_space('\\t')")
    assert_eq(char_is_space('\n'), true,  "char_is_space('\\n')")
    assert_eq(char_is_space('a'),  false, "char_is_space('a')")
    assert_eq(char_is_space('0'),  false, "char_is_space('0')")

    // char_is_upper
    assert_eq(char_is_upper('A'), true,  "char_is_upper('A')")
    assert_eq(char_is_upper('Z'), true,  "char_is_upper('Z')")
    assert_eq(char_is_upper('a'), false, "char_is_upper('a')")
    assert_eq(char_is_upper('0'), false, "char_is_upper('0')")

    // char_is_lower
    assert_eq(char_is_lower('a'), true,  "char_is_lower('a')")
    assert_eq(char_is_lower('z'), true,  "char_is_lower('z')")
    assert_eq(char_is_lower('A'), false, "char_is_lower('A')")
    assert_eq(char_is_lower('0'), false, "char_is_lower('0')")

    // Sanity: works on a non-literal u8 too (covers the Ch 4 tokenizer
    // use case where a variable holds a byte). Not str_char_at — that
    // returns string, not u8.
    let b: u8 = 'Q'
    assert_eq(char_is_upper(b), true, "char_is_upper(var)")
    assert_eq(char_is_alpha(b), true, "char_is_alpha(var)")
    assert_eq(char_is_digit(b), false, "char_is_digit(var)")

    println("char_is_* OK")
  }
}
