@inline(never) func opaqueTrapInvertedRangeValue<T>(_ v: T) -> T { v }
public func trapInvertedRangeValue() { let lo = opaqueTrapInvertedRangeValue(5), hi = opaqueTrapInvertedRangeValue(1); let r = lo..<hi; print("\(r.count)") }
