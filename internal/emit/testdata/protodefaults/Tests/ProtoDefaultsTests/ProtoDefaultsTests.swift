import Testing
@testable import ProtoDefaults

@Test func refinedDefaultWins() {
    #expect(insertLog(42) == 42)
    #expect(insertEntry(7) == 7)
}

@Test func settableRequirementOnStruct() {
    #expect(renumber(9) == "#9")
}
