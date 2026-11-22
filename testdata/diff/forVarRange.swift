public func forVarRange() {
    var out: [String] = []
    var n = 0
    for var i in 0..<5 {
        i += 2
        n += 1
        out.append("\(i)")
    }
    out.append("n=\(n)")
    for var i in stride(from: 0, to: 6, by: 2) {
        i *= 10
        out.append("s\(i)")
    }
    for var j in 1...3 where j != 2 {
        j -= 1
        out.append("c\(j)")
    }
    print(out.joined(separator: " "))
}
