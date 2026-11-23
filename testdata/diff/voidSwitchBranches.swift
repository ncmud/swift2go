private enum VsbPiece { case plain, noted, printed, skipped }

private final class VsbLog {
    var lines: [String] = []
    @discardableResult func note(_ s: String) -> String? { lines.append(s); return s }
    @discardableResult func count() -> Int { lines.count }
}

private func vsbAt(_ body: () -> Void) { body() }

public func voidSwitchBranches() {
    let log = VsbLog()
    for piece in [VsbPiece.plain, .noted, .printed, .skipped] {
        vsbAt {
            switch piece {
            case .plain: log.lines.append("plain")
            case .noted: log.note("noted")
            case .printed: print("printed")
            case .skipped: log.count()
            }
        }
        vsbAt {
            if piece == .noted { log.note("again") } else { log.count() }
        }
    }
    print(log.lines)
}
