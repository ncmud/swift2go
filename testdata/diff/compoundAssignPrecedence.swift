final class CAPBox {
    var stored: UInt8 = 0b1010_0101
    var computed: Int {
        get { stored == 0 ? 0 : Int(stored) }
        set { stored = UInt8(truncatingIfNeeded: newValue) }
    }
    var scale = 10.0
    var ratio: Double {
        get { scale }
        set { scale = newValue }
    }
}

public func compoundAssignPrecedence() {
    let a: UInt8 = 0b1100_1100
    let b: UInt8 = 0b1100_1100
    var diff: UInt8 = 0b0000_0001
    diff |= a ^ b
    print("or-xor", diff)
    var x: UInt8 = 0b1111_0000
    x ^= a | b
    print("xor-or", x)
    var y: UInt8 = 0b0011_1100
    y &= a ^ 0b0000_1111
    print("and-xor", y)
    var z: UInt8 = 0b1000_0001
    z |= a & b ^ 0b0101_0101
    print("or-and-xor", z)
    var w = 100
    w &-= 30 &- 10
    print("wrapping-sub", w)
    var v = 7
    v &*= 2 &+ 3
    print("wrapping-mul", v)
    var d = 10.0
    d -= 4.0 - 1.0
    print("double-sub", d)
    let box = CAPBox()
    box.computed -= 50 - 20
    print("setter-sub", box.computed)
    box.ratio -= 4.0 - 1.0
    print("setter-double-sub", box.ratio)
    var s: Int = 1
    s <<= 1 + 1
    print("shift", s)
}
