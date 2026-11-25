public func stringContainsRegex() {
    let lines = ["hit [12] for 4", "a] [b", "plain text", "[0]", ""]
    for line in lines {
        print(line.contains(#/\] \[/#), line.contains(#/\[\d+\]/#), line.contains("["), line.contains(#/^$/#))
    }
    let digits = #/\d{2,}/#
    print(lines.filter { $0.contains(digits) })
    print(!lines.contains { $0.wholeMatch(of: #/\[\d+\]/#) != nil || $0.contains(#/\] \[/#) })
    print("Case".contains(#/case/#.ignoresCase()), "Case".contains(#/case/#))
}
