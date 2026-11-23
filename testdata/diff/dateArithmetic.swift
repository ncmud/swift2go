import Foundation

public func dateArithmetic() {
    let start = Date(timeIntervalSince1970: 1_000_000)
    var day = start
    day += 24 * 3600
    day -= 600
    let later = start + 90
    let sooner = start - 30
    print(day.timeIntervalSince1970, later.timeIntervalSince1970, sooner.timeIntervalSince1970, later.timeIntervalSince(start), day > start)
}
