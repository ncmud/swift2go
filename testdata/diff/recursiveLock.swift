import Foundation

final class RecursiveLockLedger {
    private let lock = NSRecursiveLock()
    private var rows: [String: [Int]] = ["a": [1, 2], "b": [3]]

    func lasts() -> [Int] {
        lock.withLock { rows.values.compactMap(\.last).sorted() }
    }

    func total() -> Int {
        lock.withLock { lasts().reduce(0, +) }
    }
}

public func recursiveLock() {
    let ledger = RecursiveLockLedger()
    print(ledger.lasts(), ledger.total())
}
