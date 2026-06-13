// Test I/O builtins: list_dir, file_exists, mkdtemp

func test_list_dir() {
  let entries = list_dir(".")
  assert(len(entries) > 0, "current dir should have entries")
}

func test_file_exists() {
  assert(file_exists("runtime/lyric_runtime.h"), "runtime header should exist")
  assert(!file_exists("nonexistent_file_xyz.txt"), "nonexistent file should not exist")
}

func test_mkdtemp() {
  let dir = mkdtemp("lyric-test")
  assert(len(dir) > 0, "mkdtemp should return a path")
  assert(file_exists(dir), "created temp dir should exist")
}

func test_os_getwd() {
  let cwd = os_getwd()
  assert(len(cwd) > 0, "working directory should not be empty")
}
