import Base
import Kit

public func run(_ body: (CapturingWriter) -> String) -> String { body(CapturingWriter()) }

public func app() -> [String] {
    var w: Writer = Writer()
    let c = CapturingWriter()
    w = c
    return [describe(c), w.write("y"), run { describe($0) }]
}
