// A deinit that only frees pointer memory: Go's collector frees it, so the program behaves the same.

@freestanding(expression)
public macro diUnused(_ value: Int) -> Int = #externalMacro(module: "DiffMacros", type: "UnusedMacro")

class DIBuffer {
    let input: UnsafeMutablePointer<CChar>
    init(size: Int) {
        input = .allocate(capacity: size)
        input.initialize(repeating: 65, count: size)
    }
    deinit {
        input.deallocate()
    }
    func first() -> CChar { input[0] }
}

final class DILineBuffer: DIBuffer {
    let line: UnsafeMutablePointer<CChar>
    init(size: Int, line: Int) {
        self.line = .allocate(capacity: line)
        self.line.initialize(repeating: 66, count: line)
        super.init(size: size)
    }
    deinit {
        line.deallocate()
    }
}

public func deinitFreesMemory() {
    var out: [String] = []
    for i in 1...3 {
        var b: DIBuffer? = i == 2 ? DILineBuffer(size: i, line: 4) : DIBuffer(size: i)
        out.append("\(b!.first()) \(b is DILineBuffer)")
        b = nil
        out.append("released \(b == nil)")
    }
    print(out.joined(separator: "\n"))
}
