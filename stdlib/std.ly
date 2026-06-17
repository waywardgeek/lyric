// std.ly — Lyric standard library
// Auto-imported into all Lyric programs

lyric std {

  // ArrayList: a parent P owns a dynamic array of children C.
  // Children know their parent and index for O(1) swap-remove.
  pub interface ArrayList<P, C> {
    // The parent's array of children
    field P.children: [C]

    // Child back-reference and index
    field C.parent: P?
    field C.index: i32

    // Append child to end of parent's array
    pub func array_append(parent: P, child: C) {
      let kids = parent.children()
      let num: i32 = len(kids)
      child.set_index(num)
      child.set_parent(parent)
      parent.set_children(append(kids, child))
    }

    // Remove child from parent's array using swap-remove (O(1))
    pub func array_remove(child: C) {
      let p = child.parent()
      if isnull(p) {
        return
      }
      let kids = p!.children()
      let idx = child.index()
      let last_idx: i32 = len(kids) - 1
      if idx < last_idx {
        // Swap with last element
        let last_child = kids[last_idx]
        last_child.set_index(idx)
        kids[idx] = last_child
      }
      // Pop last element
      p!.set_children(kids[0:last_idx])
      child.set_parent(null)
      child.set_index(0)
    }

    // Destructor for parent: cascade destroy all owned children
    destructor P {
      let kids = self.children()
      let mut i: i32 = len(kids) - 1
      while i >= 0 {
        kids[i].set_parent(null)
        kids[i].destroy()
        i = i - 1
      }
    }

    // Destructor for child: remove self from parent's array
    destructor C {
      array_remove<P, C>(self)
    }
  }

  // --- Doubly-linked list family ---

  // DoublyLinked: base interface for intrusive doubly-linked list.
  // Provides fields and traversal methods. No destruction semantics —
  // use OwningList or RefList which embed this and add appropriate destructors.
  pub interface DoublyLinked<P, C> {
    field P.first: C?
    field P.last: C?
    field C.next: C?
    field C.prev: C?
    field C.parent: P?

    // Append child to end of parent's list
    pub func dll_append(parent: P, child: C) {
      child.set_parent(parent)
      child.set_next(null)
      let old_last = parent.last()
      child.set_prev(old_last)
      if !isnull(old_last) {
        old_last!.set_next(child)
      }
      parent.set_last(child)
      if isnull(parent.first()) {
        parent.set_first(child)
      }
    }

    // Remove child from parent's list
    pub func dll_remove(child: C) {
      let p = child.parent()
      if isnull(p) {
        return
      }
      let prev_node = child.prev()
      let next_node = child.next()
      if !isnull(prev_node) {
        prev_node!.set_next(next_node)
      } else {
        p!.set_first(next_node)
      }
      if !isnull(next_node) {
        next_node!.set_prev(prev_node)
      } else {
        p!.set_last(prev_node)
      }
      child.set_parent(null)
      child.set_prev(null)
      child.set_next(null)
    }
  }

  // OwningList: parent owns children, cascade-destroys on parent death.
  pub interface OwningList<P, C> {
    embed DoublyLinked<P, C>

    destructor P {
      let mut cur = self.first()
      while !isnull(cur) {
        let next = cur!.next()
        cur!.set_parent(null)
        cur!.destroy()
        cur = next
      }
    }

    destructor C {
      dll_remove<P, C>(self)
    }
  }

  // RefList: parent references children but does not own their lifetime.
  // On parent death, children are unlinked but NOT destroyed.
  pub interface RefList<P, C> {
    embed DoublyLinked<P, C>

    destructor P {
      let mut cur = self.first()
      while !isnull(cur) {
        let next = cur!.next()
        cur!.set_parent(null)
        cur!.set_prev(null)
        cur!.set_next(null)
        cur = next
      }
      self.set_first(null)
      self.set_last(null)
    }

    destructor C {
      dll_remove<P, C>(self)
    }
  }

  // --- Hash table ---

  // HashedList: parent owns a hash table of children, keyed by hash_key().
  // Open-addressing with linear probing. Children stored in a dense array;
  // a parallel bucket index maps hash slots to array positions.
  // Child classes must implement: hash_key(self) -> u64
  //
  // Usage:
  //   relation HashedList MyMap owns [MyEntry]
  //   class MyEntry {
  //     key: string
  //     value: i32
  //     pub func hash_key(self) -> u64 { return hash_string(self.key) }
  //   }
  pub interface HashedList<P, C> {
    // Dense storage of children
    field P.children: [C]
    // Bucket array: each slot is an index into children[], or -1 (empty), -2 (tombstone)
    field P.buckets: [i32]
    // Current bucket array capacity
    field P.hash_cap: i32
    // Number of live entries (== len(children))
    field P.hash_count: i32

    // Child back-reference and position in children array
    field C.parent: P?
    field C.index: i32

    // Child must implement this method to provide its hash key
    func C.hash_key(self) -> u64

    // Initialize hash table with given capacity (call before first insert)
    pub func hash_init(parent: P, capacity: i32) {
      let mut cap = capacity
      if cap < 8 {
        cap = 8
      }
      let mut b: [i32] = []
      let mut i: i32 = 0
      while i < cap {
        b = append(b, -1)
        i = i + 1
      }
      parent.set_buckets(b)
      parent.set_hash_cap(cap)
      parent.set_hash_count(0)
    }

    // Find the bucket slot for a key, or the first empty/tombstone slot
    pub func hash_find_slot(parent: P, key: u64) -> i32 {
      let cap = parent.hash_cap()
      let buckets = parent.buckets()
      let kids = parent.children()
      let mut slot: i32 = (key % (cap as u64)) as i32
      let mut i: i32 = 0
      while i < cap {
        let idx = buckets[slot]
        if idx == -1 {
          return slot
        }
        if idx >= 0 {
          if kids[idx].hash_key() == key {
            return slot
          }
        }
        // idx == -2 (tombstone) or hash collision: continue probing
        slot = (slot + 1) % cap
        i = i + 1
      }
      // Table full (should never happen if we rehash properly)
      return -1
    }

    // Rehash into a larger bucket array
    pub func hash_rehash(parent: P) {
      let old_kids = parent.children()
      let old_count = parent.hash_count()
      let new_cap = parent.hash_cap() * 2
      // Build new bucket array
      let mut new_buckets: [i32] = []
      let mut i: i32 = 0
      while i < new_cap {
        new_buckets = append(new_buckets, -1)
        i = i + 1
      }
      parent.set_buckets(new_buckets)
      parent.set_hash_cap(new_cap)
      // Re-insert all children
      i = 0
      while i < old_count {
        let key = old_kids[i].hash_key()
        let mut slot: i32 = (key % (new_cap as u64)) as i32
        while parent.buckets()[slot] >= 0 {
          slot = (slot + 1) % new_cap
        }
        let mut b = parent.buckets()
        b[slot] = i
        parent.set_buckets(b)
        i = i + 1
      }
    }

    // Insert child into hash table
    pub func hash_insert(parent: P, child: C) {
      // Auto-init if needed
      if parent.hash_cap() == 0 {
        hash_init<P, C>(parent, 8)
      }
      // Rehash at 75% load
      let count = parent.hash_count()
      let cap = parent.hash_cap()
      if (count + 1) * 4 > cap * 3 {
        hash_rehash<P, C>(parent)
      }
      let key = child.hash_key()
      let slot = hash_find_slot<P, C>(parent, key)
      let bucket_val = parent.buckets()[slot]
      if bucket_val >= 0 {
        // Key already exists — replace value (swap child at that index)
        let old_child = parent.children()[bucket_val]
        old_child.set_parent(null)
        old_child.set_index(0)
        let mut kids = parent.children()
        child.set_parent(parent)
        child.set_index(bucket_val)
        kids[bucket_val] = child
        parent.set_children(kids)
        return
      }
      // New entry: append to children array, set bucket
      let idx = parent.hash_count()
      child.set_parent(parent)
      child.set_index(idx)
      parent.set_children(append(parent.children(), child))
      let mut b = parent.buckets()
      b[slot] = idx
      parent.set_buckets(b)
      parent.set_hash_count(idx + 1)
    }

    // Look up a child by hash key. Returns null if not found.
    pub func hash_lookup(parent: P, key: u64) -> C? {
      if parent.hash_cap() == 0 {
        return null
      }
      let cap = parent.hash_cap()
      let buckets = parent.buckets()
      let kids = parent.children()
      let mut slot: i32 = (key % (cap as u64)) as i32
      let mut i: i32 = 0
      while i < cap {
        let idx = buckets[slot]
        if idx == -1 {
          return null
        }
        if idx >= 0 {
          if kids[idx].hash_key() == key {
            return kids[idx]
          }
        }
        slot = (slot + 1) % cap
        i = i + 1
      }
      return null
    }

    // Remove a child by hash key. Returns true if found and removed.
    pub func hash_remove(parent: P, key: u64) -> bool {
      if parent.hash_cap() == 0 {
        return false
      }
      let cap = parent.hash_cap()
      let buckets = parent.buckets()
      let kids = parent.children()
      let mut slot: i32 = (key % (cap as u64)) as i32
      let mut i: i32 = 0
      while i < cap {
        let idx = buckets[slot]
        if idx == -1 {
          return false
        }
        if idx >= 0 {
          if kids[idx].hash_key() == key {
            // Found it — remove from children array using swap-remove
            let count = parent.hash_count()
            let last_idx = count - 1
            let child = kids[idx]
            child.set_parent(null)
            child.set_index(0)
            if idx < last_idx {
              // Swap with last child
              let last_child = kids[last_idx]
              last_child.set_index(idx)
              let mut k = parent.children()
              k[idx] = last_child
              parent.set_children(k)
              // Update bucket for swapped child
              let swapped_key = last_child.hash_key()
              let mut s: i32 = (swapped_key % (cap as u64)) as i32
              while buckets[s] != last_idx {
                s = (s + 1) % cap
              }
              let mut b2 = parent.buckets()
              b2[s] = idx
              parent.set_buckets(b2)
            }
            // Tombstone the removed slot
            let mut b = parent.buckets()
            b[slot] = -2
            parent.set_buckets(b)
            // Shrink children array
            parent.set_children(parent.children()[0:last_idx])
            parent.set_hash_count(last_idx)
            return true
          }
        }
        slot = (slot + 1) % cap
        i = i + 1
      }
      return false
    }

    // Destructor for parent: cascade destroy all owned children
    destructor P {
      let kids = self.children()
      let mut i: i32 = len(kids) - 1
      while i >= 0 {
        kids[i].set_parent(null)
        kids[i].destroy()
        i = i - 1
      }
    }

    // Destructor for child: remove self from parent's hash table
    destructor C {
      let p = self.parent()
      if !isnull(p) {
        hash_remove<P, C>(p!, self.hash_key())
      }
    }
  }

  // Sym: an interned symbol with pre-computed hash.
  // Hash is computed once at creation; all subsequent lookups use the u64.
  // Reference equality — sym("foo") == sym("foo") is always true.
  pub class Sym {
    name: string
    hash: u64

    pub func get_name(self) -> string { return self.name }
    pub func get_hash(self) -> u64 { return self.hash }
    pub func hash_key(self) -> u64 { return self.hash }
  }

  // SymTable: global intern table for Sym instances.
  // Uses HashedList for O(1) lookup by hash.
  pub permanent class SymTable { }
  relation HashedList SymTable:st owns [Sym:st]

  // Global symbol table instance.
  let mut _sym_table: SymTable? = null

  func _get_sym_table() -> SymTable {
    if isnull(_sym_table) {
      _sym_table = SymTable { }
    }
    return _sym_table!
  }

  // sym: intern a string as a Sym. Returns the same instance for the same string.
  // Hash computed once; subsequent calls with same string return cached Sym.
  pub func sym(name: string) -> Sym {
    let h = hash_string(name)
    let table = _get_sym_table()
    let existing = hash_lookup<SymTable, Sym>(table, h)
    if !isnull(existing) {
      return existing!
    }
    let s = Sym { name: name, hash: h }
    hash_insert<SymTable, Sym>(table, s)
    return s
  }
  // --- Hashable interface: required for Dict keys ---

  pub interface Hashable {
    func get_hash(self) -> u64
  }

  // Sym implements Hashable
  pub func Sym.equals(self, other: Sym) -> bool {
    return self == other
  }

  // Integer types implement Hashable
  pub func i8.get_hash(self) -> u64 { return self as u64 }
  pub func i16.get_hash(self) -> u64 { return self as u64 }
  pub func i32.get_hash(self) -> u64 { return self as u64 }
  pub func i64.get_hash(self) -> u64 { return self as u64 }
  pub func u8.get_hash(self) -> u64 { return self as u64 }
  pub func u16.get_hash(self) -> u64 { return self as u64 }
  pub func u32.get_hash(self) -> u64 { return self as u64 }
  pub func u64.get_hash(self) -> u64 { return self }

  // --- Dict: generic hash table with clean method API ---

  // DictEntry<K, V>: key-value pair stored in a Dict.
  pub class DictEntry<K, V> where K: Hashable {
    key: K
    value: V

    pub func hash_key(self) -> u64 {
      return self.key.get_hash()
    }
  }

  // Dict<K, V>: a hash table mapping keys of type K to values of type V.
  // K must satisfy Hashable (get_hash() -> u64, equals(other) -> bool).
  //
  // Usage:
  //   let d = dict_new<Sym, i32>()
  //   d.set(sym("x"), 42)
  //   let entry = d.get(sym("x"))
  //   if !isnull(entry) { println(itoa(entry!.value)) }
  pub class Dict<K, V> where K: Hashable { }
  relation HashedList Dict<K, V>:d owns [DictEntry<K, V>:d]

  // Core methods
  pub func Dict.set<K, V>(self, key: K, value: V) where K: Hashable {
    let entry = DictEntry<K, V> { key: key, value: value }
    hash_insert<Dict<K, V>, DictEntry<K, V>>(self, entry)
  }

  pub func Dict.get<K, V>(self, key: K) -> DictEntry<K, V>? where K: Hashable {
    let h = key.get_hash()
    return hash_lookup<Dict<K, V>, DictEntry<K, V>>(self, h)
  }

  pub func Dict.has<K, V>(self, key: K) -> bool where K: Hashable {
    return !isnull(self.get(key))
  }

  pub func Dict.remove<K, V>(self, key: K) -> bool where K: Hashable {
    let h = key.get_hash()
    return hash_remove<Dict<K, V>, DictEntry<K, V>>(self, h)
  }

  pub func Dict.keys<K, V>(self) -> [K] where K: Hashable {
    let mut result: [K] = []
    for entry in self.d_children() {
      append(result, entry.key)
    }
    return result
  }

  // --- Parsing ---

  // Parse a string as an integer. Returns 0 on failure.
  pub func parse_int(s: string) -> i64 {
    let mut result: i64 = 0
    let mut neg = false
    let mut i = 0
    if len(s) > 0 && s[0] == 45 as u8 {
      neg = true
      i = 1
    }
    while i < len(s) {
      let c = s[i]
      if c >= 48 as u8 && c <= 57 as u8 {
        result = result * 10 + (c - 48 as u8) as i64
      }
      i = i + 1
    }
    if neg { return -result }
    return result
  }

  // Parse a string as a float. Returns 0.0 on failure.
  // Named str_to_float to avoid collision with C backend builtin parse_float.
  pub func str_to_float(s: string) -> f64 {
    let mut result: f64 = 0.0
    let mut neg = false
    let mut i = 0
    if len(s) > 0 && s[0] == 45 as u8 {
      neg = true
      i = 1
    }
    // Integer part
    while i < len(s) && s[i] >= 48 as u8 && s[i] <= 57 as u8 {
      result = result * 10.0 + (s[i] - 48 as u8) as f64
      i = i + 1
    }
    // Fractional part
    if i < len(s) && s[i] == 46 as u8 {
      i = i + 1
      let mut frac: f64 = 0.1
      while i < len(s) && s[i] >= 48 as u8 && s[i] <= 57 as u8 {
        result = result + (s[i] - 48 as u8) as f64 * frac
        frac = frac * 0.1
        i = i + 1
      }
    }
    if neg { return -result }
    return result
  }

  // error: interface for (T, error) returns and ? operator.
  // Any class with message(self) -> string satisfies error.
  interface error {
    func error.message(self) -> string
  }

  // Error: default concrete error type satisfying the error interface.
  pub class Error {
    msg: string

    pub func message(self) -> string { return self.msg }
  }

  // StringBuilder: efficient string building via repeated append.
  // Uses string concatenation internally until string=[u8] is implemented.
  pub class StringBuilder {
    buf: string

    pub func write(self, s: string) {
      // Append bytes in-place with doubling growth via append.
      // Avoids O(n²) concat + memory leak from lyric_str_concat.
      let mut i = 0
      while i < len(s) {
        self.buf = append(self.buf, s[i])
        i = i + 1
      }
    }

    pub func write_byte(self, b: u8) {
      self.buf = append(self.buf, b)
    }

    pub func to_string(self) -> string {
      return self.buf
    }

    pub func len(self) -> i32 {
      return len(self.buf)
    }
  }

  // push_bytes: bulk-append bytes from src string to dst string in-place.
  // Uses doubling growth + memcpy. Much faster than string concat for StringBuilder.
  pub func push_bytes(mut dst: string, src: string) {
    let mut i = 0
    while i < len(src) {
      dst = append(dst, src[i])
      i = i + 1
    }
  }

  // new_string_builder: create an empty StringBuilder
  pub func new_string_builder() -> StringBuilder {
    return StringBuilder { buf: "" }
  }

  // range: generate integers from start (inclusive) to end (exclusive).
  // Usage: for i in range(0, n) { ... }
  pub func range(start: i32, end: i32) -> gen i32 {
    let mut i: i32 = start
    while i < end {
      yield i
      i = i + 1
    }
  }

}
