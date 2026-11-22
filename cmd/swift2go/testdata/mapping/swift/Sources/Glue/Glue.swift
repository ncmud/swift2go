public enum Limits {
    public static let glue = 40
}

public func glueAdd(_ a: Int, _ b: Int) -> Int { a + b }

public struct Tally {
    public var total: Int

    public init(total: Int) { self.total = total }

    public func doubled() -> Int { total * 2 }
}

public enum Shade {
    case light
    case dark
}

public func shade(_ n: Int) -> Shade { n > 10 ? .dark : .light }

public struct Box<T> {
    public let value: T

    public init(_ value: T) { self.value = value }
}

public func unbox<T>(_ box: Box<T>) -> T { box.value }

public func evens(upTo n: Int) -> StrideTo<Int> { stride(from: 0, to: n, by: 2) }

public final class Tag {
    public let name: String

    public init(_ name: String) { self.name = name }
}

public func firstOf<T>(_ items: [T]) -> T? { items.first }
