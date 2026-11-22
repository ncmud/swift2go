@inline(never) func opaqueTrapInvertedRange<T>(_ v: T) -> T { v }
public func trapInvertedRange() { let lo = opaqueTrapInvertedRange(5), hi = opaqueTrapInvertedRange(1); var n = 0; for _ in lo..<hi { n += 1 }; print("\(n)") }
