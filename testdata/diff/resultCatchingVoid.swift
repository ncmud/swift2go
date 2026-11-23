private struct RcvFailure: Error {}

private func rcvMaybeThrow(_ fail: Bool) throws { if fail { throw RcvFailure() } }

public func resultCatchingVoid() {
    for fail in [false, true] {
        switch Result(catching: { try rcvMaybeThrow(fail) }) {
        case .success: print("ok")
        case .failure(let e): print("failed", e is RcvFailure)
        }
    }
    let n = Result { () throws -> Int in 4 }
    print((try? n.get()) ?? 0)
}
