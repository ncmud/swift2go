import Testing
import Checks
@testable import Lib

@Test func equalArraysPass() {
    checkEqual(["a", "b"], ["a", "b"])
    checkEqual(Row(["x"]), Row(["x"]))
    checkEqual([1: [2]], [1: [2]])
}

@Test func comparableUsesTheTypesOwnLess() {
    checkLess(Score(9), Score(2))
    checkLess(1, 2)
    checkLess("a", "b")
}

@Test func unequalArraysRecordAnIssue() {
    checkEqual(["a"], ["b"], "arrays differ")
}

struct NamedRows {
    static let rows: [(name: String, size: Int)] = [("one", 3), ("three", 5)]

    @Test(arguments: rows.map(\.name))
    func keyPathArguments(name: String) {
        checkEqual(Self.rows.first { $0.name == name }!.size, name.count)
    }
}

@Test func optionalReferencesThroughGenerics() throws {
    let room: Room = try checkSome(Room.shared)
    checkEqual(room.vnum, 3001)
    let missing: Room? = nil
    checkNil(missing)
    let error: (any Error)? = nil
    checkNil(error)
    let failure: (any Error)? = LibError()
    checkEqual(try checkSome(failure) is LibError, true)
}
