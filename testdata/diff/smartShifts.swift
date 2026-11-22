@inline(never) func opaqueSmartShifts<T>(_ v: T) -> T { v }
public func smartShifts() {
    let one = opaqueSmartShifts(1), n64 = opaqueSmartShifts(64), n70 = opaqueSmartShifts(70), neg = opaqueSmartShifts(-1), m8 = opaqueSmartShifts(-8)
    var out = "\(one << n64) \(one << n70) \(one << neg) \(m8 >> one) \(m8 >> n70) \(opaqueSmartShifts(8) >> n64) \(opaqueSmartShifts(8) << opaqueSmartShifts(-2)) \(m8 >> opaqueSmartShifts(-2)) \(m8 << opaqueSmartShifts(-70))\n"
    let u = opaqueSmartShifts(UInt8(0x81))
    out += "\(u << 1) \(u >> 1) \(u << opaqueSmartShifts(8)) \(u >> opaqueSmartShifts(-1)) \(Int8(bitPattern: u) >> 1) \(Int8(bitPattern: u) << opaqueSmartShifts(-9))\n"
    let i32 = opaqueSmartShifts(Int32(1))
    out += "\(i32 << opaqueSmartShifts(31)) \(i32 << opaqueSmartShifts(32)) \(i32 << opaqueSmartShifts(33)) \(UInt32(1) << opaqueSmartShifts(31)) \(i32 << opaqueSmartShifts(Int8(-1)))\n"
    var s = opaqueSmartShifts(256)
    s >>= opaqueSmartShifts(-2)
    s <<= opaqueSmartShifts(-3)
    out += "\(s)\n"
    print(out, terminator: "")
}
