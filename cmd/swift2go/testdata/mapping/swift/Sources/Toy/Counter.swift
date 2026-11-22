import Glue

public struct Counter {
    public private(set) var count = 0

    public init() {}

    public mutating func bump(by step: Int) {
        count = glueAdd(count, step)
    }
}

public func greeting(for name: String) -> String {
    "hello, \(name)"
}
