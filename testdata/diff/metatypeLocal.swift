enum MetatypeLocalBits {
    static let read: UInt64 = 1 << 0
    static let write: UInt64 = 1 << 1
}

public func metatypeLocal() {
    let bits = MetatypeLocalBits.self
    print(bits.read | bits.write)
}
