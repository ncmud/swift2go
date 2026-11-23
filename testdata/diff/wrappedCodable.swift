import Foundation

@propertyWrapper
public struct WcBoxed<Value: Codable>: Codable {
    public var wrappedValue: Value
    public init(wrappedValue: Value) { self.wrappedValue = wrappedValue }
    public init(from decoder: any Decoder) throws { wrappedValue = try Value(from: decoder) }
    public func encode(to encoder: any Encoder) throws { try wrappedValue.encode(to: encoder) }
}

public struct WcSnap: Codable {
    public var id: Int
    @WcBoxed public var hp: Int = 0
    @WcBoxed public var tags: [String] = []

    public init(id: Int) { self.id = id }
}

public struct WcKeyed: Codable {
    public var name: String
    @WcBoxed public var level: Int = 1
    enum CodingKeys: String, CodingKey { case name, level = "lvl" }
    public init(name: String) { self.name = name }
}

public func wrappedCodable() {
    let enc = JSONEncoder()
    enc.outputFormatting = .sortedKeys
    var s = WcSnap(id: 7)
    s.hp = 250
    s.tags = ["a"]
    let data = try! enc.encode(s)
    print(String(decoding: data, as: UTF8.self))
    let back = try! JSONDecoder().decode(WcSnap.self, from: data)
    print(back.id, back.hp, back.tags)
    let fromSwift = try! JSONDecoder().decode(WcSnap.self, from: Data(#"{"id":3,"hp":42,"tags":[]}"#.utf8))
    print(fromSwift.hp, fromSwift.tags)
    var k = WcKeyed(name: "x")
    k.level = 12
    print(String(decoding: try! enc.encode(k), as: UTF8.self))
    print(try! JSONDecoder().decode(WcKeyed.self, from: Data(#"{"name":"y","lvl":3}"#.utf8)).level)
}
