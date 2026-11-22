public enum XPLog { nonisolated(unsafe) public static var trace: [String] = [] }

open class XPDescriptor {
    public var name: String
    var buffer: [String] = []
    public var connected = true

    public init(name: String) {
        self.name = name
        XPLog.trace.append("XPDescriptor.init \(kind())")
    }

    public convenience init() { self.init(name: "anon") }

    open func kind() -> String { "descriptor" }

    open func write(_ msg: String) {
        guard connected else { return }
        buffer.append(msg)
    }

    open var label: String { "label(\(name))" }

    public func send(_ msg: String) { write("[\(kind())] " + msg) }

    public var output: String { buffer.joined(separator: "|") }
}

public func xpDeliver(_ d: XPDescriptor, _ msg: String) -> String {
    d.send(msg)
    return d.kind() + ":" + d.label + ":" + d.output
}
