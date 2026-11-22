import Testing
import App
@testable import Lib

extension Counter {
    func doubled() -> Int { count * 2 }
}

@Test func libInternalsAndAppTogether() {
    var c = makeCounter(start: 1)
    c.bump()
    #expect(c.count == 2)
    #expect(c.doubled() == 4)
    #expect(bumpedTwice(from: c.count) == 4)
}

@Test func memberwiseDefaultsFromTheTestedTarget() {
    let t = Tally(vnum: 7, losses: 5)
    #expect(t.wins == 1 && t.losses == 5 && t.reason == "none" && t.level == nil)
    #expect(Counter().count == 0)
}

@Test func underscoredInternalsFromAnotherPackage() {
    let m = Mover()
    m._move()
    #expect(m._moves == 1)
}
