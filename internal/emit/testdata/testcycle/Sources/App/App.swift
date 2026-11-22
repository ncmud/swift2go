@testable import Lib

public func bumpedTwice(from start: Int) -> Int {
    var c = makeCounter(start: start)
    c.bump()
    c.bump()
    return c.count
}
