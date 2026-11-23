# Copy-on-write for arrays

Swift arrays are values, but copying one costs nothing: copies share one buffer, and a mutation first
checks `isKnownUniquelyReferenced` and copies the buffer only when another copy still holds it. Go has
no reference counts, so swift2go cannot ask that question. This note describes what it does instead.

## Before

The emitter keeps value semantics by ownership (`internal/emit/value.go`): every place owns its storage,
so a mutation writes in place, and `swiftrt.Copy` deep-copies a value whenever it moves into another
place. The effects pass (`internal/emit/effects.go`) removes the copies Swift never makes: a value lent
to a callee or a loop is copied only if the place it came from can change during the loan. The copies
that remain are real copies in Swift too, but in Swift they cost a retain. In swift2go they cost an
allocation and a walk over every element.

## Representation

Nothing changes in the Go types. An `Array`, `ArraySlice` or `ContiguousArray` is still a Go slice, so
reads, `len`, `range` and `append` are the same Go code as before.

A slice header carries a length and a capacity, and the two now say who may write the storage:

- **Owned**: `len < cap`. The holder of this header is the only one that can write the storage. Spare
  capacity is the mark.
- **Possibly shared**: `len == cap`. Copies may share this storage, so no one writes it in place.

The mark can be wrong in one direction only. A new array with no spare capacity (a literal, the result
of `map`, Go's `append` filling its capacity exactly) is treated as shared, although it is not; its
first write copies it once. That is safe, and the only cost is the extra copy.

## The rules

1. **Writes go through `swiftrt.Mut`.** Every in-place write to an array's storage is
   `swiftrt.Mut(&xs)[i] = v`. `Mut` returns the slice unchanged when it owns spare capacity, and
   otherwise moves it to new storage with one spare element first. Element assignment, compound
   assignment, mutating a struct element, passing an element `inout` and calling a mutating method on one
   all reach the element through `lvalue`, which emits `Mut` at each array on the path:
   `swiftrt.Mut(&swiftrt.Mut(&grid)[y])[x] = 1`. An array inside a dictionary entry is written through a
   copy of the entry that the statement stores back, since Go cannot address a map element. The
   runtime's in-place mutators (`sort`, `swapAt`, `reverse`, `shuffle`, `remove(at:)`,
   `removeAll(where:)`, `removeSubrange`, `withUnsafeMutableBufferPointer`, and the ordered collections'
   removals) call `Mut` first.
2. **Appends need nothing.** `append` writes past `len` only when the header owns spare capacity;
   otherwise it reallocates. No copy can see past its own length, so neither case reaches another copy.
3. **Shrinking keeps the mark.** `removeLast`, `removeLast(n)`, `popLast` and
   `removeAll(keepingCapacity:)` keep a shared header shared (`xs[:n:n]`, or new storage for
   `keepingCapacity`). Without this, `xs[:n-1]` would gain spare capacity over elements another copy
   still holds, and the next append would overwrite them.
4. **`Copy` shares what is frozen.** A slice is frozen when it has no spare capacity and every slice
   inside its elements is frozen too, all the way down. `Copy` keeps a frozen slice as it is. It copies
   any other slice and gives the copy no spare capacity, after copying, with the same rule, the slices
   inside the copied elements, so that later copies of the copy share it.
5. **The runtime never hands out a part of an array marked wrongly.** A sub-array the runtime returns
   (`Array[a..<b]`, `prefix`, `prefix(while:)`, the parts of `split`) is marked shared when the array is
   (`xs[a:b:b]`), and otherwise keeps the array's capacity past its end (`xs[a:b]`), so copying it
   copies it. A part of shared storage that looked owned would be written in place once it was moved
   out of a function without a `Copy`; a part of owned storage that looked shared would be shared by
   `Copy` while its owner kept writing it.

Rule 4 has to look all the way down because rule 1's first write is shallow. When `Mut` moves an array
of arrays, the new storage holds the same inner headers as the old one. That is safe only if those inner
headers are themselves marked shared, which is what frozen means. An array whose elements own spare
capacity is never shared by `Copy`, so the case cannot arise.

## Which arrays can be shared

Sharing depends on every write going through `Mut`. Go code that writes storage some other way must not
see shared storage, so `Copy` keeps copying, as before, any array whose element type holds:

- a non-class pointer (a value type's Optional), a map (a Dictionary or Set), or an interface (a protocol
  value or an enum with associated values), whose contents can be written without `Mut`;
- an exported field without a `swift` tag in a named struct: a field of a runtime or overlay type, such
  as `OrderedSet.Items`, which Go code writes in place.

The copy of such an array gets one spare element of capacity, so `Mut` never looks it up. When `Mut`
meets one without spare capacity, it checks the element type (a map lookup by type word) and leaves the
array where it is. Arrays of numbers, strings, class references, closures, and structs and tuples of
those, nested in arrays to any depth, are shared.

Dictionaries, sets and value-type optionals are still copied whole by `Copy`. The values they hold
follow the array rules, so a `[String: [Int]]` copy allocates a new map but shares frozen arrays.

## What the effects pass still decides

The effects pass and the origin and sink rules in `value.go` are unchanged. They decide *where* a value
needs `Copy`: on a store, a return, a capture, and on a loan whose source can change during the loan.
Copy-on-write changes what a `Copy` costs, not where one goes. A `Copy` of frozen arrays is now a walk
over the slice headers with no allocation, and a copy that is never written is never paid for. The
effects pass still saves the walk itself, and it still decides alone for the storage that is not shared:
dictionaries, sets, optionals and protocol values.

## Soundness

The invariant is: **no two headers that can be used to write share storage, unless all of them are
marked shared and the storage is frozen.**

- Copies in variables, struct fields, class properties and captured closures all come from `Copy`, so
  they hold either new storage or frozen storage that every holder sees as shared.
- `inout` parameters and mutating methods get a pointer to the caller's place, so they write the
  caller's header, through `Mut`.
- A captured `var` is the Go variable itself, shared as Swift shares it.
- Nested arrays: frozen storage holds only shared headers (rule 4), so a holder writing an inner array
  moves the outer array first (`Mut` at each level) and then the inner one.
- A loop snapshot is a `Copy` (rule 4); the body's writes to the source go through `Mut` and move it.
- Library results built from elements of another array (`filter`, `+`, `reversed`) may hold inner
  headers that another array owns. They are never written before a `Copy`, which copies whatever is not
  frozen. This is the same reasoning that already made them safe under eager copying.
- A loan (an argument or receiver passed without `Copy`) duplicates a header without marking it.
  The effects pass already guarantees that the lender does not write during the loan, and the borrower
  cannot write through a loan in Swift, so the duplicate is only read.

`Copy` and `Mut` never write into storage that may be shared: `Copy` only reads the value it copies, and
`Mut` replaces only its own header. Concurrent readers of shared storage therefore race with nothing.

What it cannot do is notice that a copy died. A Swift array whose other copy has been released is
uniquely referenced again; in Go both sides keep the shared mark, so each side's first write after the
copy moves its storage, where Swift would move only one. A copy that is written once costs the same as
before; a copy whose original is also written afterwards costs one extra copy.

## Cost

Reads are unchanged Go slice indexing. A write to an owned array costs one comparison of length and
capacity, which `Mut` inlines; in a tight loop of writes this keeps the compiler from hoisting the bounds
check, about 0.6 ns per write on an M-series core (`BenchmarkWriteOwnedArrayMut` against
`BenchmarkWriteOwnedArrayDirect`).
