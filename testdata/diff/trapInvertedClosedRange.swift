@inline(never) func opaqueTrapInvertedClosedRange<T>(_ v: T) -> T { v }
public func trapInvertedClosedRange() { let lo = opaqueTrapInvertedClosedRange(5), hi = opaqueTrapInvertedClosedRange(1); var n = 0; for _ in lo...hi { n += 1 }; print("\(n)") }
