public func rawPointerBitPattern() {
    let a = UnsafeMutableRawPointer(bitPattern: 0x1000)!
    let b = UnsafeMutableRawPointer(bitPattern: 0x1000)!
    let c = UnsafeMutableRawPointer(bitPattern: 0x2000)!
    print(a == b, a == c, UnsafeMutableRawPointer(bitPattern: 0) == nil)
    var owners: [UnsafeMutableRawPointer: String] = [:]
    owners[a] = "first"
    print(owners[b] ?? "none", owners[c] ?? "none")
}
