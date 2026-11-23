import Foundation

public func stringRegexLookaround() {
    let lines = ["level 61 warrior", "level 610", "room 1612", "61"]
    for line in lines {
        print(line.range(of: #"(?<![0-9])61(?![0-9])"#, options: .regularExpression) != nil)
    }
    let s = "Price: $40, cost: $7, total 47"
    if let r = s.range(of: #"(?<=\$)\d+"#, options: .regularExpression) { print(s[r]) }
    print(s.replacingOccurrences(of: #"(?<=\$)(\d+)"#, with: "<$1>", options: .regularExpression))
    print("Hello hello HELLO".replacingOccurrences(of: #"(?<!^)hello"#, with: "x", options: [.regularExpression, .caseInsensitive]))
}
