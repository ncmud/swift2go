final class OSAMob {
    var log: [String] = []
    var ceiling = 50
    var hp = 10 {
        didSet {
            guard hp != oldValue else { return }
            if hp > oldValue, hp > ceiling { hp = max(ceiling, oldValue) }
            log.append("hp \(oldValue)->\(hp)")
        }
    }
    var level = 1 {
        didSet {
            if level > 5 { self.level = 5 }
            level += 0
            log.append("level \(oldValue)->\(level)")
        }
    }
    var mana = 0 {
        willSet { log.append("will mana \(mana)->\(newValue)") }
        didSet {
            if mana < 0 { mana = 0 }
            log.append("did mana \(oldValue)->\(mana)")
        }
    }
    var tags: [String] = [] {
        didSet {
            if tags.count > 2 { tags.removeFirst() }
            log.append("tags \(oldValue.count)->\(tags.count)")
        }
    }
    var buddy: OSAMob?
    var shared = 0 {
        didSet {
            log.append("shared \(oldValue)->\(shared)")
            if let buddy, buddy !== self, buddy.shared != shared { buddy.shared = shared }
        }
    }
}

public func observerSelfAssign() {
    let m = OSAMob()
    m.hp = 80
    m.hp = 20
    m.level = 9
    m.level += 1
    m.mana = -3
    m.tags.append("a")
    m.tags.append("b")
    m.tags.append("c")
    let other = OSAMob()
    m.buddy = other
    other.buddy = m
    m.shared = 7
    print((m.log + other.log).joined(separator: "\n"))
    print("\(m.hp) \(m.level) \(m.mana) \(m.tags)")
}
