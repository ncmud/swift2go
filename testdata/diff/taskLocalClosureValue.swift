private enum TlcvLog {
    @TaskLocal static var sink: @Sendable (String) -> Void = { _ in }
}

private final class TlcvLines: @unchecked Sendable { var lines: [String] = [] }

private func tlcvNote(_ s: String) { TlcvLog.sink(s) }

public func taskLocalClosureValue() {
    let lines = TlcvLines()
    TlcvLog.$sink.withValue({ line in lines.lines.append(line) }) {
        tlcvNote("one")
        tlcvNote("two")
    }
    tlcvNote("dropped")
    let n = TlcvLog.$sink.withValue({ lines.lines.append("x" + $0) }) { () -> Int in
        tlcvNote("three")
        return lines.lines.count
    }
    print(lines.lines, n)
}
