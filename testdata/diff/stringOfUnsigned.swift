public func stringOfUnsigned() {
    let big: UInt64 = .max
    print(String(big))
    print(String(big, radix: 16))
    print(String(big, radix: 16, uppercase: true))
    print(String(UInt64(1) << 63))
    print(String(Int64.min))
    print(String(UInt8(200)), String(Int8(-5), radix: 2))
}
