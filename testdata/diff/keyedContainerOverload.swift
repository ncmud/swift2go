import Foundation

protocol KoZero: Codable {
    static var koZero: Self { get }
    var isKoZero: Bool { get }
}

extension KoZero where Self: Equatable {
    var isKoZero: Bool { self == Self.koZero }
}

extension Int: KoZero { static var koZero: Int { 0 } }
extension String: KoZero { static var koZero: String { "" } }
extension Array: KoZero where Element: Codable {
    static var koZero: [Element] { [] }
    var isKoZero: Bool { isEmpty }
}

@propertyWrapper
struct KoOmitted<Value: KoZero>: Codable {
    var wrappedValue: Value
    init(wrappedValue: Value) { self.wrappedValue = wrappedValue }
    init(from decoder: any Decoder) throws { wrappedValue = try Value(from: decoder) }
    func encode(to encoder: any Encoder) throws { try wrappedValue.encode(to: encoder) }
}

extension KeyedEncodingContainer {
    mutating func encode<Value>(_ value: KoOmitted<Value>, forKey key: Key) throws {
        guard !value.wrappedValue.isKoZero else { return }
        try encode(value.wrappedValue, forKey: key)
    }
}

extension KeyedDecodingContainer {
    func decode<Value>(_: KoOmitted<Value>.Type, forKey key: Key) throws -> KoOmitted<Value> {
        KoOmitted(wrappedValue: try decodeIfPresent(Value.self, forKey: key) ?? .koZero)
    }
}

private struct KoRecord: Codable {
    init(id: Int) { self.id = id }
    var id: Int
    @KoOmitted var hits: Int = 0
    @KoOmitted var name: String = ""
    @KoOmitted var tags: [String] = []
}

private func koZeroOf<T: KoZero>(_ v: T) -> Bool { v.isKoZero }

public func keyedContainerOverload() {
    let enc = JSONEncoder()
    enc.outputFormatting = .sortedKeys
    for r in [KoRecord(id: 1), { var r = KoRecord(id: 2); r.hits = 3; r.name = "x"; r.tags = ["a"]; return r }()] {
        let data = try! enc.encode(r)
        print(String(decoding: data, as: UTF8.self))
        let back = try! JSONDecoder().decode(KoRecord.self, from: data)
        print(back.id, back.hits, back.name, back.tags)
    }
    print(koZeroOf(0), koZeroOf(5), koZeroOf(""), koZeroOf([1]))
    let zeros: [any KoZero] = [0, 7, "", "a", [String]()]
    print(zeros.map { $0.isKoZero })
}
