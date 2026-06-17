lyric iface_mangle {

interface Describable<T> {
  func T.describe(self) -> string
}

interface Sizeable<T> {
  func T.describe(self) -> i32
}

class Widget {
  name: string
  size: i32
  pub func get_name(self) -> string { return self.name }
  pub func get_size(self) -> i32 { return self.size }
}

impl Describable<Widget> {
  T.describe = Widget.get_name
}

impl Sizeable<Widget> {
  T.describe = Widget.get_size
}

func show_desc<T>(t: T) where Describable<T> {
  println(t.describe())
}

func show_size<T>(t: T) where Sizeable<T> {
  println(t.describe())
}

func main() {
  let w = Widget { name: "Button", size: 42 }
  show_desc(w)
  show_size(w)
}

}
