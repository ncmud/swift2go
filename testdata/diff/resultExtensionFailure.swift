enum REFOpenFailure: Error, Equatable {
    case taken
    case closed(Int)
}

private extension Result {
    var failure: Failure? {
        guard case .failure(let failure) = self else { return nil }
        return failure
    }

    func failureOr(_ fallback: Failure) -> Failure {
        failure ?? fallback
    }

    var isFailure: Bool { failure != nil }

    func label() -> String { isFailure ? "failed" : "succeeded" }
}

private func refOpen(_ n: Int) -> Result<String, REFOpenFailure> {
    switch n {
    case 0: return .success("opened")
    case 1: return .failure(.taken)
    default: return .failure(.closed(n))
    }
}

public func resultExtensionFailure() {
    for n in 0..<3 {
        let opened = refOpen(n)
        print(opened.failure.map { "\($0)" } ?? "none", opened.failure == .taken, opened.failureOr(.closed(-1)), opened.isFailure, opened.label())
    }
}
