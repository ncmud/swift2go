// Copies of an array share its storage until one of them is written. Each check copies an array one
// way, writes through one side, and prints both, so a write that leaks into another copy shows up.

struct CWBag {
    var items: [Int]
    var names: [String]
}

final class CWBox {
    var items: [Int] = [1, 2, 3]
    var bag = CWBag(items: [4, 5], names: ["a"])
}

private func cwBump(_ xs: inout [Int]) {
    xs[0] += 100
    xs.append(9)
}

private func cwKeep(_ xs: [Int], into box: CWBox) {
    box.items = xs
}

private func cwMade() -> [Int] {
    var xs = [7, 8]
    xs.append(9)
    return xs
}

public func copyOnWriteArrays() {
    // Copies in variables, written on either side.
    var a = [1, 2, 3]
    var b = a
    b[0] = 10
    print("vars", a, b)
    a[1] = 20
    print("vars back", a, b)

    // A copy of a copy, then each written in turn.
    let c0 = [1, 2]
    var c1 = c0
    var c2 = c1
    c1[0] = 5
    c2[1] = 6
    print("chain", c0, c1, c2)

    // An array that owns spare capacity, copied, then appended to and written on both sides.
    var grown: [Int] = []
    for i in 0..<5 { grown.append(i) }
    var copy = grown
    grown.append(5)
    copy.append(50)
    grown[0] = -1
    print("grown", grown, copy)

    // Shrinking a shared array, then appending, must not write into the other copy's elements.
    var s1 = [1, 2, 3, 4]
    let s2 = s1
    s1.removeLast()
    s1.append(40)
    print("removeLast", s1, s2)
    var s3 = s2
    s3.removeLast(2)
    s3.append(30)
    print("removeLast(n)", s2, s3)
    var s4 = s2
    _ = s4.popLast()
    s4.append(99)
    print("popLast", s2, s4)
    var s5 = s2
    s5.removeAll(keepingCapacity: true)
    s5.append(7)
    print("removeAll(keepingCapacity:)", s2, s5)

    // Library mutations of a shared array.
    let base = [5, 3, 1, 4, 2]
    var sorted = base
    sorted.sort()
    var by = base
    by.sort(by: >)
    var rev = base
    rev.reverse()
    var swapped = base
    swapped.swapAt(0, 4)
    var removed = base
    removed.remove(at: 1)
    var inserted = base
    inserted.insert(0, at: 2)
    var filtered = base
    filtered.removeAll { $0 > 3 }
    var ranged = base
    ranged.removeSubrange(1..<3)
    var first = base
    first.removeFirst()
    first[0] = 77
    print("library", base)
    print("library", sorted, by, rev, swapped, removed, inserted, filtered, ranged, first)

    // Struct fields: copying the struct shares the arrays; writing one field copies only it.
    var bag1 = CWBag(items: [1, 2], names: ["x", "y"])
    var bag2 = bag1
    bag2.items[0] = 100
    bag1.names[1] = "z"
    print("struct", bag1.items, bag1.names, bag2.items, bag2.names)

    // A class property copied out, and an array stored into one.
    let box = CWBox()
    var fromBox = box.items
    fromBox[0] = -5
    box.items[2] = 33
    print("class", box.items, fromBox)
    var local = [1, 1]
    cwKeep(local, into: box)
    local[0] = 2
    box.items[1] = 3
    print("stored", local, box.items)
    var bagCopy = box.bag
    box.bag.items[0] = 44
    bagCopy.names[0] = "b"
    print("class struct", box.bag.items, box.bag.names, bagCopy.items, bagCopy.names)

    // inout: the callee writes the caller's array, and only it.
    var io = [1, 2]
    let ioCopy = io
    cwBump(&io)
    print("inout", io, ioCopy)
    var elems = [[1], [2]]
    let elemsCopy = elems
    cwBump(&elems[1])
    print("inout element", elems, elemsCopy)

    // Closures: a captured constant keeps the value it had; a captured var is the variable.
    var captured = [1, 2]
    let fixed = captured
    let readFixed = { fixed }
    let readVar = { captured }
    let writeVar = { captured[0] = 9 }
    captured[1] = 8
    writeVar()
    print("closures", readFixed(), readVar(), captured, fixed)
    var made: [() -> [Int]] = []
    var counter = [0]
    for i in 0..<3 {
        counter[0] = i
        let snapshot = counter
        made.append { snapshot }
    }
    print("made", made.map { $0() }, counter)

    // A loop over an array the body writes iterates the array as it was.
    var looped = [1, 2, 3]
    for x in looped {
        looped[0] += x
        looped.append(x)
    }
    print("loop", looped)

    // Results of the program's functions and library calls, written after a copy.
    var r1 = cwMade()
    let r2 = r1
    r1[0] = 0
    var mapped = base.map { $0 * 2 }
    let mappedCopy = mapped
    mapped[0] = -1
    var repeated = Array(repeating: 0, count: 3)
    let repeatedCopy = repeated
    repeated[1] = 1
    print("results", r1, r2, mapped, mappedCopy, repeated, repeatedCopy)

    // reduce(into:) writes its accumulator in place.
    let seed = [0, 0]
    let reduced = [1, 2, 3].reduce(into: seed) { acc, x in acc[x % 2] += x }
    print("reduce", seed, reduced)

    // Many copies, each written once.
    var all: [[Int]] = []
    var proto = [0, 0, 0]
    for i in 0..<4 {
        var c = proto
        c[i % 3] = i
        all.append(c)
        proto[0] += 1
    }
    print("many", proto, all)
}
