import Foundation
public func compareOptionsLiteral() {
    let names = ["Alpha", "beta", "GAMMA"]
    print(names.filter { $0.range(of: "a", options: [.caseInsensitive]) != nil })
    print(names.filter { $0.range(of: "A", options: [.caseInsensitive, .backwards]) != nil })
    print("abc".compare("ABC", options: [.caseInsensitive]) == .orderedSame)
}
