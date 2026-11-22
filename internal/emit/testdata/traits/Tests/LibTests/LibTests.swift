import Lib
import Scopes
import Testing

@Suite("ledger scoping", .freshLedger, .serialized)
struct LedgerTests {
    @Test("the suite's scope wraps each test")
    func suiteScope() {
        #expect(Dispatch.current?.dispatch("hi") == "echo hi")
        Ledger.active.entries.append("a")
        #expect(Ledger.active.entries == ["fresh", "a"])
    }

    @Test(.ledger("inner"))
    func testScopeInsideSuiteScope() {
        #expect(Ledger.active.entries == ["inner"])
    }

    @Test(.enabled(if: 1 + 1 == 3))
    func disabledByCondition() {
        Issue.record("ran a test whose condition is false")
    }

    @Test(.disabled("never runs"))
    func disabled() {
        Issue.record("ran a disabled test")
    }

    @Test(.enabled(if: true), .timeLimit(.minutes(1)))
    func enabledByCondition() {
        #expect(Ledger.current != nil)
    }
}

@Test func unscoped() {
    #expect(Ledger.current == nil)
    #expect(Dispatch.current == nil)
}
