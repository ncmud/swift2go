import Base

public final class CapturingWriter: Writer {
    public override init() { super.init() }
    public override func write(_ s: String) -> String { "captured " + s }
}
