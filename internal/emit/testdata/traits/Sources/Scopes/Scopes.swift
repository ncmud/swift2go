import Lib
import Testing

/// Gives each test a fresh Ledger, tagged with the scope's label.
public struct FreshLedger: TestTrait, SuiteTrait, TestScoping {
    let label: String
    public var isRecursive: Bool { true }

    public func provideScope(
        for test: Test, testCase: Test.Case?, performing function: @Sendable () async throws -> Void
    ) async throws {
        let ledger = Ledger()
        ledger.entries.append(label)
        try await Ledger.$current.withValue(ledger) {
            try await Dispatch.$current.withValue(Echo()) {
                try await function()
            }
        }
    }
}

extension Trait where Self == FreshLedger {
    public static var freshLedger: Self { FreshLedger(label: "fresh") }
    public static func ledger(_ label: String) -> Self { FreshLedger(label: label) }
}

final class Echo: Dispatching, @unchecked Sendable {
    func dispatch(_ line: String) -> String { "echo " + line }
}
