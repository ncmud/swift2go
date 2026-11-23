// Copies of arrays of arrays, of structs holding arrays, of dictionaries holding arrays, and of array
// slices. A write at any depth through one copy must not reach another.

struct CWNRow {
    var cells: [Int]
    var tag: String
}

struct CWNGrid {
    var rows: [CWNRow]
    var marks: [[Int]]
}

struct CWNLevel {
    var grids: [CWNGrid]
}

private func cwnMakeGrid() -> CWNGrid {
    var rows: [CWNRow] = []
    for i in 0..<2 {
        var cells: [Int] = []
        for j in 0..<3 { cells.append(i * 10 + j) }
        rows.append(CWNRow(cells: cells, tag: "r\(i)"))
    }
    return CWNGrid(rows: rows, marks: [[1], [2, 3]].map { $0 + [0] })
}

private func cwnShow(_ g: CWNGrid) -> String {
    "\(g.rows.map { "\($0.tag)\($0.cells)" }) \(g.marks)"
}

private func cwnSum(_ xs: ArraySlice<Int>) -> Int { xs.reduce(0, +) }

private func cwnHead(_ xs: [Int]) -> ArraySlice<Int> {
    let a = xs
    return a.prefix(2)
}

private func cwnRange(_ xs: [Int]) -> ArraySlice<Int> {
    let a = xs
    return a[0..<2]
}

public func copyOnWriteNested() {
    // Arrays of arrays: write an inner element, append to an inner array, replace a row.
    var m1 = [[1, 2], [3, 4]]
    var m2 = m1
    m2[0][0] = 10
    m1[1].append(5)
    print("nested", m1, m2)
    var m3 = m2
    m3[1] = [9]
    m2[1][1] = 40
    print("nested rows", m2, m3)
    // Inner arrays built by appending, inside an outer array built exactly.
    var built: [[Int]] = (0..<3).map { i in
        var row: [Int] = []
        for j in 0...i { row.append(j) }
        return row
    }
    let builtCopy = built
    built[2][0] = 100
    built[0].append(7)
    print("built", built, builtCopy)
    // Three levels.
    var deep = [[[1, 2]], [[3]]]
    let deepCopy = deep
    deep[0][0][1] = 20
    deep[1][0].append(4)
    print("deep", deep, deepCopy)

    // Structs holding arrays, inside arrays.
    var g1 = cwnMakeGrid()
    var g2 = g1
    g2.rows[0].cells[1] = -1
    g1.rows[1].tag = "changed"
    g1.marks[0][0] = 7
    print("grid", cwnShow(g1), "|", cwnShow(g2))
    var level = CWNLevel(grids: [g1, g2])
    let levelCopy = level
    level.grids[1].rows[1].cells.append(99)
    level.grids[0].marks[1].removeLast()
    print("level", level.grids.map(cwnShow), levelCopy.grids.map(cwnShow))
    var row = level.grids[0].rows[0]
    row.cells[0] = 555
    print("row", row.cells, level.grids[0].rows[0].cells)

    // Elements copied out by library calls, then written.
    var picked = m1.filter { $0.count > 2 }
    picked[0][0] = -3
    var joined = m1 + m2
    joined[0][1] = -4
    var reversed = Array(m1.reversed())
    reversed[0][0] = -5
    var extended = [[0]]
    extended.append(contentsOf: m1)
    extended[1][0] = -6
    print("library", m1, picked, joined, reversed, extended)

    // Dictionaries holding arrays.
    var d1: [String: [Int]] = ["a": [1, 2], "b": [3]]
    let d2 = d1
    d1["a"]![0] = 10
    d1["b", default: []].append(4)
    d1["c", default: []].append(5)
    print("dict", d1.sorted { $0.key < $1.key }.map { "\($0.key)=\($0.value)" }, d2.sorted { $0.key < $1.key }.map { "\($0.key)=\($0.value)" })
    var rowsByName: [String: CWNRow] = ["x": CWNRow(cells: [1], tag: "x")]
    let rowsCopy = rowsByName
    rowsByName["x"]!.cells[0] = 2
    print("dict struct", rowsByName["x"]!.cells, rowsCopy["x"]!.cells)
    var fromDict = d2["a"]!
    fromDict[1] = 0
    print("from dict", fromDict, d2["a"]!)

    // Array slices keep the array's indices and copy on write too.
    var whole = [1, 2, 3, 4, 5]
    let mid = whole[1..<4]
    whole[2] = 30
    var midArray = Array(whole[1..<4])
    midArray[1] = -2
    midArray.append(6)
    print("slices", whole, Array(mid), midArray, mid.startIndex, mid[2], cwnSum(mid))
    let tail = whole[2...]
    whole.append(6)
    whole[4] = 50
    print("slice tail", Array(tail), whole)
    let shared = [1, 2, 3, 4]
    var head = cwnHead(shared)
    head.append(77)
    var range = cwnRange(shared)
    range.append(66)
    print("parts", shared, Array(head), Array(range))
    var fromSlice = Array(whole[0..<2])
    fromSlice[0] = -1
    print("from slice", fromSlice, whole)
}
