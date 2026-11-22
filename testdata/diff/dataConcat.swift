import Foundation

public func dataConcat() {
    let a = Data("ab".utf8)
    var b = a + Data("\n".utf8)
    b += Data([0x63])
    b += [0x64, 0x65]
    let c = b + [0x66]
    print(a.count, b.count, c.count, String(decoding: c, as: UTF8.self) == "ab\ncdef")
}
