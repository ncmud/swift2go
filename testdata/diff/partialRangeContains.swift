private func prcGrade(_ score: Int) -> String {
    switch score {
    case ..<0: return "invalid"
    case ..<50: return "fail"
    case 90...: return "top"
    default: return "pass"
    }
}

private func prcBand(_ x: Double) -> String {
    switch x {
    case ...0.5: return "low"
    case 0.5...: return "high"
    default: return "nan"
    }
}

public func partialRangeContains() {
    let from = 5...
    let upTo = ..<5
    let through = ...5
    for x in [Int.min, -1, 0, 4, 5, 6, Int.max] {
        print(x, from.contains(x), upTo.contains(x), through.contains(x), from ~= x, upTo ~= x, through ~= x)
    }
    let letters = "m"...
    print(letters.contains("a"), letters.contains("m"), letters.contains("z"))
    let below: PartialRangeUpTo<Double> = ..<1.5
    print(below.contains(1.4), below.contains(1.5), (...2.5).contains(2.5), (2.5...).contains(2.4))
    let small: PartialRangeThrough<UInt8> = ...200
    print(small.contains(200), small.contains(201))
    for s in [-5, 10, 50, 89, 90, 1000] {
        print(s, prcGrade(s))
    }
    print(prcBand(0.5), prcBand(0.75))
    if case 3... = 4 {
        print("4 is at least 3")
    }
    if ..<0 ~= -1 {
        print("-1 is negative")
    }
    let limits = [10..., 20..., 30...]
    print(limits.filter { $0.contains(25) }.count)
}
