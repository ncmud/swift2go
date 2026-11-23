private func mshGeneric<T: FixedWidthInteger>(_ x: T, _ n: T) -> (T, T) {
    (x &<< n, x &>> n)
}

public func maskingShifts() {
    let counts = [-65, -64, -1, 0, 1, 7, 8, 9, 31, 32, 33, 63, 64, 65, 127, 128]
    for n in counts {
        print("Int", n, 1 &<< n, Int.min &>> n, -12345 &>> n, 0x1234 &<< n)
        print("Int8", n, Int8(1) &<< Int8(truncatingIfNeeded: n), Int8.min &>> Int8(truncatingIfNeeded: n))
        print("UInt8", n, UInt8(0x81) &<< UInt8(truncatingIfNeeded: n), UInt8(0x81) &>> UInt8(truncatingIfNeeded: n))
        print("Int16", n, Int16(-3) &<< Int16(truncatingIfNeeded: n), Int16(-3) &>> Int16(truncatingIfNeeded: n))
        print("UInt32", n, UInt32(0xdeadbeef) &<< UInt32(truncatingIfNeeded: n), UInt32(0xdeadbeef) &>> UInt32(truncatingIfNeeded: n))
        print("Int64", n, Int64(-1) &<< Int64(n), Int64.max &>> Int64(n))
        print("UInt", n, UInt.max &<< UInt(truncatingIfNeeded: n), UInt.max &>> UInt(truncatingIfNeeded: n))
    }
    var h: UInt32 = 2166136261
    for byte in Array("swift2go".utf8) {
        h ^= UInt32(byte)
        h = (h &<< 13) | (h &>> 19)
        h &*= 16777619
    }
    print("hash", h)
    var a = 1
    a &<<= 70
    print("a", a)
    var b: Int8 = -128
    b &>>= 9
    print("b", b)
    var c: UInt16 = 0xffff
    c &<<= 20
    print("c", c)
    var d: UInt64 = 1
    let shift = 200
    d &<<= UInt64(shift)
    print("d", d)
    let mixed = UInt8(3) &<< 9
    print("mixed", mixed, Int32(1) &<< 33, Int32(-8) &>> 35)
    print(mshGeneric(Int8(5), Int8(10)), mshGeneric(UInt(5), UInt(66)), mshGeneric(-5, -1))
}
