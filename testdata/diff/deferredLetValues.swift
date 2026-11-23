final class DLHolder {
    var table: [Int: [Int]] = [1: [10, 11]]
}

struct DLTables {
    var weapon: [Int: [Int]] = [1: [1, 2], 2: [3]]
    var armor: [Int: [Int]] = [1: [4]]

    func values(armor wantArmor: Bool, vnum: Int) -> [Int] {
        let dict: [Int: [Int]]
        if wantArmor {
            dict = armor
        } else {
            dict = weapon
        }
        return dict[vnum] ?? [0]
    }
}

func dlFromLocal() -> String {
    var source = [1: [1]]
    let copy: [Int: [Int]]
    copy = source
    source[1]?.append(2)
    source[2] = [3]
    return "\(copy[1]!) \(copy.count) \(source[1]!) \(source.count)"
}

func dlFromClass(_ h: DLHolder) -> String {
    let snapshot: [Int: [Int]]
    snapshot = h.table
    h.table[1]?.append(12)
    return "\(snapshot[1]!) \(h.table[1]!)"
}

func dlReturned(_ t: DLTables) -> [Int: [Int]] {
    let chosen: [Int: [Int]]
    chosen = t.weapon
    return chosen
}

public func deferredLetValues() {
    let t = DLTables()
    print(t.values(armor: false, vnum: 1), t.values(armor: true, vnum: 1), t.values(armor: true, vnum: 9))
    print(dlFromLocal())
    print(dlFromClass(DLHolder()))
    var r = dlReturned(t)
    r[1]?.append(99)
    print(r[1]!, t.weapon[1]!)
}
