private struct GRIBox {
    var any: Int
    var string: String
    var len: Int
    private var map: [String: Int] = [:]
    init(any: Int, string: String, len: Int) {
        self.any = any
        self.string = string
        self.len = len
        map[string] = len
    }
    func lookup(_ key: String) -> Int? { map[key] }
}

private enum GRIKind {
    case int, string, `func`, chan, range, select

    var label: String {
        switch self {
        case .int: return "int"
        case .string: return "string"
        case .func: return "func"
        case .chan: return "chan"
        case .range: return "range"
        case .select: return "select"
        }
    }
}

private func griJoin(any: [Any], string: String, len: Int, cap: Int, make: Bool) -> String {
    let parts = any.map { "\($0)" }
    return make ? "\(string)\(parts.joined(separator: ",")) len=\(len) cap=\(cap) count=\(parts.count)" : string
}

private func griSum(_ int: Int, _ uint: UInt8, _ float64: Double, real: Double, imag: Double) -> Double {
    Double(int) + Double(uint) + float64 + real + imag
}

public func goReservedIdentifiers() {
    let any: [Any] = [1, "two", 3.5]
    let int = any.count
    let string = "s:\(int)"
    var len = string.count
    let cap = [Int](repeating: 0, count: 4).count
    let make = len > 0
    let new = GRIBox(any: int, string: string, len: len)
    let error: Error? = nil
    let type = "kind"
    let `func` = { (x: Int) -> Int in x * 2 }
    let chan = [String: Int]()
    var select = 0
    let go = true
    let `defer` = "deferred"
    let range = 0..<3
    let map = range.map { $0 * 10 }
    let iota = map.reduce(0, +)
    let comparable = [3, 1, 2].sorted()
    let complex64 = Int32(7)
    let uintptr = UInt(9)
    let rune: Character = "r"
    let byte: UInt8 = 0x41
    let interface = [any.count, int]
    let `var` = 5
    let goto = "label"
    let package = "pkg"
    let `fallthrough` = 1
    for (i, any) in any.enumerated() {
        select += i
        print("element", i, any)
    }
    len += `func`(select)
    print(griJoin(any: any, string: string, len: len, cap: cap, make: make))
    print(new.any, new.string, new.len, new.lookup(string) ?? -1, new.lookup("none") ?? -1)
    print(error == nil, type, chan.isEmpty, go, `defer`, map, iota, comparable, complex64, uintptr, rune, byte)
    print(interface, `var`, goto, package, `fallthrough`)
    print(griSum(int, byte, 1.5, real: 0.25, imag: 0.125))
    for kind in [GRIKind.int, .string, .func, .chan, .range, .select] {
        print(kind.label)
    }
    let nested: (Int) -> String = { any in
        let string = "nested \(any)"
        return string
    }
    print(nested(int))
    if let any = any.first as? Int {
        print("first int", any)
    }
}
