import Foundation

public protocol WcoZero: Codable {
    static var zero: Self { get }
    var isZero: Bool { get }
}
extension Int: WcoZero {
    public static var zero: Int { 0 }
    public var isZero: Bool { self == 0 }
}
extension Array: WcoZero where Element: Codable {
    public static var zero: [Element] { [] }
    public var isZero: Bool { isEmpty }
}

@propertyWrapper
public struct WcoOmitted<Value: WcoZero>: Codable {
    public var wrappedValue: Value
    public init(wrappedValue: Value) { self.wrappedValue = wrappedValue }
    public init(from decoder: any Decoder) throws { wrappedValue = try Value(from: decoder) }
    public func encode(to encoder: any Encoder) throws { try wrappedValue.encode(to: encoder) }
}

extension KeyedDecodingContainer {
    func decode<Value>(_: WcoOmitted<Value>.Type, forKey key: Key) throws -> WcoOmitted<Value> {
        WcoOmitted(wrappedValue: try decodeIfPresent(Value.self, forKey: key) ?? .zero)
    }
}

extension KeyedEncodingContainer {
    mutating func encode<Value>(_ value: WcoOmitted<Value>, forKey key: Key) throws {
        guard !value.wrappedValue.isZero else { return }
        try encode(value.wrappedValue, forKey: key)
    }
}

public struct WcoSnap: Codable {
    public var id: Int
    @WcoOmitted public var maim: Int = 0
    @WcoOmitted public var followers: [Int] = []
    public init(id: Int) { self.id = id }
}

public func wrappedCodableOverloads() {
    let enc = JSONEncoder()
    enc.outputFormatting = .sortedKeys
    var s = WcoSnap(id: 1)
    s.maim = 3
    s.followers = [7]
    let data = try! enc.encode(s)
    print(String(decoding: data, as: UTF8.self))
    let back = try! JSONDecoder().decode(WcoSnap.self, from: data)
    print(back.maim, back.followers)
    let sparse = try! JSONDecoder().decode(WcoSnap.self, from: Data(#"{"id":2}"#.utf8))
    print(sparse.id, sparse.maim, sparse.followers)
}
