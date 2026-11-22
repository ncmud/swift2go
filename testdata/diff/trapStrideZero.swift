@inline(never) func opaqueTrapStrideZero<T>(_ v: T) -> T { v }
public func trapStrideZero() { var n = 0; for _ in stride(from: 0, to: 3, by: opaqueTrapStrideZero(0)) { n += 1; break }; print("\(n)") }
