@inline(never) func opaqueOperatorPrecedence<T>(_ v: T) -> T { v }

final class OPBox {
    var value: Int? = 6
    var bits: UInt8 = 0b0101_0110
    var mask: UInt8 {
        get { bits }
        set { bits = newValue }
    }
    var items: [Int] = []
}

struct OPFlags {
    var raw: UInt8
    var isLow: Bool { raw < 16 }
    var isEven: Bool { raw & 1 == 0 }
}

func opGenericShift<T: FixedWidthInteger>(_ a: T, _ b: T) -> T { a & b << 2 }
func opGenericMul<T: FixedWidthInteger>(_ a: T, _ b: T) -> T { a &* b << 1 }
func opGenericOr<T: BinaryInteger>(_ a: T, _ b: T) -> T { a | b & 3 }

public func operatorPrecedence() {
    let a = opaqueOperatorPrecedence(0b1100_1010)
    let b = opaqueOperatorPrecedence(0b0110_0101)
    let c = opaqueOperatorPrecedence(0b0011_1100)
    let n = opaqueOperatorPrecedence(3)
    let u = opaqueOperatorPrecedence(UInt8(0b1010_0101))
    let v = opaqueOperatorPrecedence(UInt8(0b0001_1110))

    // A shift binds tighter than a multiplicative operator in Swift, but not in Go.
    print("and-shl", a & b << 2, a & b << n, u & v << 1, u & v >> 1)
    print("wrapmul-shl", a &* b << 2, a &* b >> 1, u &* v << 1)
    print("mul-shl", a * b << 2, a / b >> 1, a % b << 1, a * b << n)
    print("shl-and", a << 2 & b, a >> 1 &* b, u << 1 & v)
    print("generic", opGenericShift(a, b), opGenericMul(u, v), opGenericOr(a, b))

    // Additive and multiplicative bitwise operators.
    print("or-and", a | b & c, a ^ b & c, a &+ b & c, a & b | c, a & b ^ c, a &- b & c)
    print("or-shl", a | b << 2, a ^ b >> 1, a &+ b << 2, a &- b << 1)
    print("or-xor", a | b ^ c, a ^ b | c, a &+ b ^ c, a ^ b &+ c, a | b &- c)
    print("add-and", a + b & c, a - b | c, a + b ^ c)

    // Bitwise operators against comparisons.
    print("cmp", a | b == c | 0xEF, a & b == 0b0100_0000, a ^ b != c, a << 1 == 404, u &+ v > 190)

    // Logical operators, including Bool lowerings.
    let flags = OPFlags(raw: opaqueOperatorPrecedence(UInt8(10)))
    let arr = opaqueOperatorPrecedence([Int]())
    let str = opaqueOperatorPrecedence("text")
    let t = opaqueOperatorPrecedence(true), f = opaqueOperatorPrecedence(false)
    print("logic", t || f && f, !t || t, !(t && f), f && f || t, a < b || b < c && c < a)
    print("lowered", !arr.isEmpty && t, f == arr.isEmpty, arr.isEmpty == f, !str.isEmpty, !str.hasPrefix("x") && f)
    print("computed", !flags.isLow || t, flags.isEven && !flags.isLow, !flags.isEven == f)

    // Unary operators.
    print("unary", ~a & b, ~(a | b), -a &* b, -(a &+ b), ~a << 2, ~u & v, u & ~0x0F, ~0 & a, -arr.count &+ 1, ~arr.count)

    // Floating point.
    let x = opaqueOperatorPrecedence(2.5), y = opaqueOperatorPrecedence(4.0), z = opaqueOperatorPrecedence(1.5)
    print("float", x * y + z, x - (y - z), -x * y, x / y * z, -(x - y), x - -y, x - y * z, (x - y) * z)

    // Nil-coalescing.
    let opt = opaqueOperatorPrecedence(Int?.none), some: Int? = opaqueOperatorPrecedence(5)
    print("coalesce", (opt ?? 1) + 2, opt ?? 1 + 2, a | (opt ?? 0), opt ?? a | b, opt ?? 0 == 0, some ?? 0 &* 3, (some ?? 0) << 1)

    // Ranges.
    print("range", (0..<n + 1).count, (0...a & 7).count, (0..<a | b).count)
    var sum = 0
    for i in 0..<n &+ 1 {
        sum = sum &+ i
    }
    for i in 1...n << 1 {
        sum &+= i
    }
    print("range-loop", sum)

    // Casts.
    let anyValue: Any = opaqueOperatorPrecedence(7)
    print("cast", (anyValue as! Int) &+ 1, anyValue is Int && f, (anyValue as? Int ?? 0) | 8, Int(u) << 2 & 0xFF, Int(u & 0xF) << 4, UInt8(truncatingIfNeeded: a << 2), Int(u) &* Int(v) << 1)

    // Ternaries.
    print("ternary", t ? a : b &+ 1, (f ? a : b) &* 2, a == 1 ? a | b : c, t ? a & b << 1 : 0, f ? 0 : a ^ b)

    // Optional chaining.
    let box = OPBox()
    let none = opaqueOperatorPrecedence(OPBox?.none)
    let boxed: OPBox? = opaqueOperatorPrecedence(box)
    print("chain", (boxed?.value ?? 0) &* 2, boxed?.value == 6, (none?.value ?? 1) | 2, boxed!.bits & 0x0F << 1, boxed?.items.isEmpty == true)

    // String interpolation.
    print("interp \(a & b << 2) \(a | b) \(-a &* b) \(a == b || t) \(u ^ v &+ 1)")

    // Compound assignments with mixed operators.
    var p = a
    p &= b << 2
    print("c-and-shl", p)
    p = a
    p |= b & c << 1
    print("c-or-and-shl", p)
    p = a
    p ^= b | c
    print("c-xor-or", p)
    p = a
    p &+= b &* c
    print("c-wrapadd-mul", p)
    p = a
    p &-= b | c
    print("c-wrapsub-or", p)
    p = a
    p &*= b &+ 1
    print("c-wrapmul-add", p)
    p = a
    p <<= n &+ 1
    print("c-shl-add", p)
    p = a
    p >>= 1
    p &= ~b
    print("c-and-not", p)
    var q = u
    q |= v ^ 0xFF
    q &= ~v | 0x0F
    print("c-u8", q)
    var d = x
    d *= y + z
    print("c-float-mul", d)
    d = x
    d /= y * z
    print("c-float-div", d)
    d = x
    d -= -y
    print("c-float-neg", d)
    var list = [a, b, c]
    list[1] |= a ^ c
    list[n - 1] &-= b | c
    print("c-subscript", list)
    var dict = ["k": a]
    dict["k", default: 0] &+= b | c
    dict["j", default: 1] <<= 2
    print("c-dict", dict["k"]!, dict["j"]!)
    box.mask ^= v | 0x80
    box.mask &= v << 1
    print("c-setter", box.mask)
    var s = "a"
    s += str + "b"
    print("c-string", s)
}
