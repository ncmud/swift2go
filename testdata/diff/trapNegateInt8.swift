@inline(never) func opaqueTrapNegateInt8<T>(_ v: T) -> T { v }
public func trapNegateInt8() { let x = opaqueTrapNegateInt8(Int8(-128)); print("\(-x)") }
