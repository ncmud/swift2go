// A local keeps its object alive to the end of its scope, so a weak reference to it read later in that
// scope still sees the object, though the local's last use came before the read. Each case allocates
// enough to run many collections between the last use and the read.

final class KAPlayer {
    let name: String
    init(name: String) { self.name = name }
}

final class KADesc {
    weak var character: KAPlayer?
}

final class KARegistry {
    var descs: [KADesc] = []
}

@inline(never)
func kaAttach(_ r: KARegistry, _ p: KAPlayer) {
    let d = KADesc()
    d.character = p
    r.descs.append(d)
}

@inline(never)
func kaPlaying(_ r: KARegistry) -> String {
    r.descs.compactMap { $0.character?.name }.joined(separator: ",")
}

@inline(never)
func kaChurn() -> Int {
    var total = 0
    for round in 0..<300 {
        let junk = [Int](repeating: round, count: 50_000)
        total += junk.count
    }
    return total
}

@inline(never)
func kaMake(_ name: String) -> KAPlayer? { KAPlayer(name: name) }

@inline(never)
func kaReturned(_ early: Bool) -> String {
    let r = KARegistry()
    let p = KAPlayer(name: "returned")
    kaAttach(r, p)
    if early {
        _ = kaChurn()
        return "early " + kaPlaying(r)
    }
    _ = kaChurn()
    return "late " + kaPlaying(r)
}

@inline(never)
func kaGuarded(_ name: String) -> String {
    let r = KARegistry()
    guard let p = kaMake(name) else { return "none" }
    kaAttach(r, p)
    _ = kaChurn()
    return kaPlaying(r)
}

public func keepAliveToScopeEnd() {
    var out: [String] = []

    let r = KARegistry()
    let (player, tag) = (KAPlayer(name: "listener"), "tuple")
    kaAttach(r, player)
    _ = kaChurn()
    out.append("\(tag) " + kaPlaying(r))

    let single = KARegistry()
    let lone = KAPlayer(name: "lone")
    kaAttach(single, lone)
    _ = kaChurn()
    out.append("let " + kaPlaying(single))

    out.append(kaReturned(true))
    out.append(kaReturned(false))
    out.append("guard " + kaGuarded("guarded"))

    if let p = kaMake("bound") {
        let reg = KARegistry()
        kaAttach(reg, p)
        _ = kaChurn()
        out.append("if " + kaPlaying(reg))
    }

    for i in 0..<2 {
        let reg = KARegistry()
        let p = KAPlayer(name: "loop\(i)")
        kaAttach(reg, p)
        if i == 0 {
            _ = kaChurn()
            out.append("continue " + kaPlaying(reg))
            continue
        }
        _ = kaChurn()
        out.append("end " + kaPlaying(reg))
    }
    print(out.joined(separator: "\n"))
}
