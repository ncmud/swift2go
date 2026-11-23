public func localFuncCapturesLaterVar() {
    var scale = 2
    func label(_ n: Int) -> String { "#\(n * scale)" }
    func summary() -> String {
        "\(totals.count): " + totals.map(label).joined(separator: " ")
    }
    var totals = [1, 2]
    print(summary())
    totals[0] = 10
    totals.append(3)
    scale = 3
    print(summary())
    let base = 100
    func shifted() -> Int { base + offset }
    let offset = 7
    print(shifted())
}
