// An optional struct or scalar is a value: a copy does not follow later edits of the original.

struct OPBond { var level = 1; var names: [String] = [] }

final class OPSoul {
    var bond: OPBond? = OPBond()
    var charges: Int? = 3
    func currentBond() -> OPBond? { return bond }
    func currentCharges() -> Int? { charges }
}

private func optionalGetter(_ out: inout [String]) {
    let s = OPSoul()
    var mine = s.currentBond()
    mine?.level = 50
    let before = s.currentBond()
    s.bond?.level += 1
    out.append("\(s.bond!.level) \(before!.level) \(mine!.level)")
    let saved = s.charges
    let viaCall = s.currentCharges()
    s.charges! -= 1
    out.append("\(saved!) \(viaCall!) \(s.charges!)")
}

private func optionalStructCopy(_ out: inout [String]) {
    var a: OPBond? = OPBond(level: 1, names: ["a"])
    var b = a
    b?.level = 5
    b?.names.append("b")
    a?.names[0] = "z"
    out.append("\(a!.level) \(a!.names) \(b!.level) \(b!.names)")
    var i: Int? = 1
    var j = i
    j! += 1
    i = 10
    out.append("\(i!) \(j!)")
    let arr: [OPBond?] = [OPBond(level: 1, names: [])]
    var first = arr[0]
    first?.level = 99
    out.append("\(arr[0]!.level) \(first!.level)")
    if var u = a { u.level = 1234; _ = u }
    out.append("\(a!.level)")
}

private func optionalArgument(_ out: inout [String]) {
    let s = OPSoul()
    func watch(_ b: OPBond?, _ c: Int?) -> String {
        s.bond?.level = 9
        s.charges! += 5
        return "\(b!.level) \(c!)"
    }
    out.append("\(watch(s.bond, s.charges)) \(s.bond!.level) \(s.charges!)")
}

public func valueOptionals() {
    var out: [String] = []
    optionalGetter(&out)
    optionalStructCopy(&out)
    optionalArgument(&out)
    print(out.joined(separator: "\n"))
}
