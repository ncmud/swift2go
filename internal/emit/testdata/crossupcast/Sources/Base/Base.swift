open class Writer {
    public init() {}
    open func write(_ s: String) -> String { "base " + s }
}

public func describe(_ w: Writer) -> String { w.write("x") }
