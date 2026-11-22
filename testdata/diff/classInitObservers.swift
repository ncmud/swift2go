enum OBLog { nonisolated(unsafe) static var lines: [String] = [] }

class OBP {
    var hp = 0 { didSet { OBLog.lines.append("OBP.hp \(oldValue)->\(hp)") } }
    init(hp: Int) { self.hp = hp; self.hp += 1 }
    convenience init() { self.init(hp: 5); hp = 9 }
}

class OBQ: OBP {
    var mp = 0 { didSet { OBLog.lines.append("OBQ.mp \(oldValue)->\(mp)") } }
    init(mp: Int) {
        self.mp = mp
        super.init(hp: 1)
        hp = 2
        hp += 3
        self.mp = 3
    }
    override init(hp: Int) { super.init(hp: hp); self.hp = hp * 10 }
}

public func classInitObservers() {
    _ = OBP(hp: 1)
    OBLog.lines.append("--")
    _ = OBP()
    OBLog.lines.append("--")
    _ = OBQ(mp: 7)
    OBLog.lines.append("--")
    let q = OBQ()
    OBLog.lines.append("--")
    q.hp = 4
    q.mp = 6
    print(OBLog.lines.joined(separator: "\n"))
}
