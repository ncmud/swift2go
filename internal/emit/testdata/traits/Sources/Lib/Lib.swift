public final class Ledger: @unchecked Sendable {
    public static let shared = Ledger()
    @TaskLocal public static var current: Ledger?
    public static var active: Ledger { current ?? shared }

    public var entries: [String] = []
    public init() {}
}

public protocol Dispatching: AnyObject, Sendable {
    func dispatch(_ line: String) -> String
}

public enum Dispatch {
    @TaskLocal public static var current: (any Dispatching)?
}
