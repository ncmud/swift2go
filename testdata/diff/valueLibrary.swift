// Results of standard library calls are values too: a slice, a filtered or concatenated array, a
// dictionary's values.

struct VLRow { var cells: [Int] }

final class VLTable {
    var rows: [VLRow] = [VLRow(cells: [1, 2]), VLRow(cells: [3])]
    var nums: [Int] = [5, 3, 4]
    var grid: [[Int]] = [[1], [2], [3]]
    var index: [String: [Int]] = ["a": [1], "b": [2]]
}

private func libraryViews(_ out: inout [String]) {
    let t = VLTable()
    let firstTwo = t.nums.prefix(2)
    let lastTwo = t.nums.suffix(2)
    let middle = t.nums[1..<3]
    let copy = Array(t.nums)
    let sorted = t.nums.sorted()
    t.nums[0] = 50
    t.nums[2] = 40
    t.nums.sort()
    out.append("\(Array(firstTwo)) \(Array(lastTwo)) \(Array(middle)) \(copy) \(sorted) \(t.nums)")
}

private func libraryElements(_ out: inout [String]) {
    let t = VLTable()
    var filtered = t.rows.filter { $0.cells.count > 1 }
    filtered[0].cells[0] = 9
    var first = t.rows.first!
    first.cells.append(8)
    var last = t.grid.last!
    last[0] = 7
    var joined = t.grid + [[4]]
    joined[0][0] = 6
    var reversed = Array(t.grid.reversed())
    reversed[0][0] = 5
    var values = t.index.values.sorted { $0[0] < $1[0] }
    values[0].append(4)
    var found = t.grid.first { $0[0] == 2 }!
    found[0] = 3
    var maxRow = t.grid.max { $0[0] < $1[0] }!
    maxRow.append(2)
    out.append("\(t.rows.map { $0.cells }) \(t.grid) \(t.index["a"]!)")
    out.append("\(filtered.map { $0.cells }) \(first.cells) \(last) \(joined) \(reversed) \(values) \(found) \(maxRow)")
}

private func libraryMutators(_ out: inout [String]) {
    let t = VLTable()
    var extra = [[10], [11]]
    t.grid.append(contentsOf: extra)
    t.grid += extra
    t.grid.insert(extra[0], at: 0)
    extra[0][0] = 99
    extra[1].append(98)
    var removed = t.grid.remove(at: 0)
    removed.append(97)
    t.index["c"] = extra[0]
    t.index.updateValue(extra[1], forKey: "d")
    extra[0].append(96)
    extra[1][0] = 95
    t.index["a", default: []].append(94)
    t.index["z", default: []].append(93)
    out.append("\(t.grid) \(removed) \(t.index["c"]!) \(t.index["d"]!) \(t.index["a"]!) \(t.index["z"]!) \(extra)")
    let total = t.grid.reduce(into: [Int]()) { $0.append(contentsOf: $1) }
    var seed = [0]
    let grown = t.nums.reduce(into: seed) { $0.append($1) }
    seed[0] = 1
    out.append("\(total) \(grown) \(seed)")
}

private func libraryOptionals(_ out: inout [String]) {
    let t = VLTable()
    var popped = t.grid.popLast()
    popped?.append(0)
    var looked = t.index["a"]
    looked?.append(0)
    let kept = t.index["b"]
    t.index["b"]!.append(1)
    t.index["b"]![0] = 20
    out.append("\(t.grid) \(popped!) \(t.index["a"]!) \(looked!) \(kept!) \(t.index["b"]!)")
}

public func valueLibrary() {
    var out: [String] = []
    libraryViews(&out)
    libraryElements(&out)
    libraryMutators(&out)
    libraryOptionals(&out)
    print(out.joined(separator: "\n"))
}
