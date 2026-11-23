struct NestedTypeNameOuter {
    struct Still {}
    final class Quiet {}
    enum Mode { case on }
}

public func nestedTypeName() {
    let values: [Any] = [NestedTypeNameOuter.Still(), NestedTypeNameOuter.Quiet(), NestedTypeNameOuter.Mode.on, NestedTypeNameOuter()]
    print(values.map { String(describing: type(of: $0)) }, String(describing: NestedTypeNameOuter.Still.self))
}
