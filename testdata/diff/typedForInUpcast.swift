private class TfiBase {
    var n: Int { 1 }
}

private final class TfiSub: TfiBase {
    override var n: Int { 2 }
}

private func tfiUse(_ b: TfiBase) -> Int { b.n * 10 }

public func typedForInUpcast() {
    let a = TfiSub(), b = TfiSub()
    var total = 0
    for ch: TfiBase in [a, b] { total += tfiUse(ch) }
    for _: TfiBase in [a] { total += 1 }
    print(total)
}
