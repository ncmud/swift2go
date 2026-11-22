@inline(never) func opaqueTrapNegate<T>(_ v: T) -> T { v }
public func trapNegate() { let x = opaqueTrapNegate(Int.min); print("\(-x)") }
