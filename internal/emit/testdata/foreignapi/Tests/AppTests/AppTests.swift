import Ledger
import Testing
@testable import App

@Test func operatorsBuildExpressions() {
    #expect(unreadQuery("bo") == "SELECT * FROM Mail WHERE (to = 'bo' AND read = 0) LIMIT 10 OFFSET 5")
    #expect(anyOfQuery([1, 2]) == "SELECT * FROM Mail WHERE (id IN (1, 2) OR NOT owner IS NULL)")
    #expect(ordering() == "id DESC")
}

@Test func classesAndGenerics() {
    #expect(insertAll(["a", "b"]) == 23)
    let (id, name) = readRow()
    #expect(id == 7)
    #expect(name == "ada")
}

@Test func literalsAndProperties() {
    #expect(arguments() == "1,2,3;min=3")
    #expect(options() == 3)
    #expect(configured() == 5)
    #expect(safeRetries() == 3)
    #expect(isFast(.fast))
    #expect(!isFast(.safe(retries: 0)))
    #expect(isBusy(.busy))
}

@Test func conformancesRegisterWitnesses() {
    let (entries, ids, sql) = registered()
    #expect(entries == "log,scores,Mail")
    #expect(ids == [1, 2, 300])
    #expect(sql == "SELECT * FROM log WHERE id = 1")
}
