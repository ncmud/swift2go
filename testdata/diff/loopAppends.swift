struct LAItem {
    var name: String
    var parts: [Int]
}

func laCollect(_ items: [LAItem], twice: Bool) -> ([LAItem], [String]) {
    var kept: [LAItem] = []
    var names: [String] = []
    let before = kept
    for (i, item) in items.enumerated() {
        guard i != 1 else { continue }
        kept.append(item)
        names.append(item.name)
        if twice { names.append(item.name + "!") }
    }
    print("before:", before.count)
    return (kept, names)
}

public func loopAppends() {
    let items = (0..<5).map { LAItem(name: "i\($0)", parts: [$0, $0 * 2]) }
    var (kept, names) = laCollect(items, twice: true)
    kept[0].parts.append(99)
    print(kept.map(\.parts), items[0].parts, names)
    print(laCollect([], twice: false).1)

    // An array that already holds elements, or shares them, keeps them.
    var log = ["start"]
    let alias = log
    for word in ["a", "b"] { log.append(word) }
    print(log, alias)

    // A loop that declares its own array of the same name appends to that one.
    var outer: [Int] = []
    for n in [1, 2, 3] {
        var outer: [Int] = []
        outer.append(n)
        print(outer)
    }
    outer.append(7)
    print(outer)

    // Nested loops append to the outer array many times per iteration.
    var grid: [String] = []
    let rows = ["r0", "r1"]
    for row in rows {
        for col in 0..<3 {
            grid.append("\(row)c\(col)")
        }
    }
    print(grid, grid.count)

    // An array emptied by removeAll before the loop is filled again.
    var reused = [1, 2, 3]
    reused.removeAll()
    for n in [4, 5] { reused.append(n) }
    print(reused)
}
