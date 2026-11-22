@inline(never) func opaque(_ x: Double) -> Double { x }

public func floatFormatting() {
    print("\(Float(2.5)) \(Float(3)) \(Float(0.1)) \(Float(12345678.9)) \(Float(1.7e38) * 10)")
    print("\(Float(1e-7)) \(Float(16777216)) \(Float(-0.0)) \(Float.nan) \(-Float.infinity)")
    print("\(0.1 + 0.2) \(4.35 * 100) \(1.0 / 3.0) \(2.0 / 3.0 * 3.0)")
    print("\(-0.0) \(0.0) \(1.0) \(100.0) \(1e15) \(1e16) \(1e20) \(1.5e300) \(1e-4) \(1.5e-5)")
    print("\(123456789012345680.0) \(0.000123) \(Double.pi) \(Double.greatestFiniteMagnitude)")
    print("\(Double.nan) \(Double.infinity) \(-Double.infinity) \(Double.leastNonzeroMagnitude)")
    print("\(opaque(0.1) * 3) \(opaque(1e16) + 1)")
    print("\(Int8.bitWidth) \(UInt16.bitWidth) \(Int32.bitWidth) \(Int.bitWidth) \(UInt8.max) \(Int8.min)")
    print([1.0, 0.5, 1e21])
    print([Float(0.1)])
    let f: Float = 0.1
    print("\(Double(f))")
    let d = 0.1
    print("\(d * 3) \(d + 0.2)")
    print([1.0, .nan, 0.5].max()!, [1.0, .nan, 0.5].min()!, [Double.nan, 2.0].max()!)
    print(max(1.0, .nan), min(.nan, 1.0))
}
