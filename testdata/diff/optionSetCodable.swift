import Foundation

struct OSDoor: Codable, Equatable {
    struct Flags: OptionSet, Codable, Hashable {
        let rawValue: Int64
        static let closed = Flags(rawValue: 1 << 3)
        static let locked = Flags(rawValue: 1 << 4)
    }
    var flags: Flags
    var key: Int
}

public func optionSetCodable() {
    let door = OSDoor(flags: [.closed, .locked], key: 7)
    let enc = JSONEncoder()
    enc.outputFormatting = [.sortedKeys]
    print(String(data: try! enc.encode(door), encoding: .utf8)!)
    let back = try! JSONDecoder().decode(OSDoor.self, from: Data("{\"flags\":8,\"key\":3}".utf8))
    print(back.flags.contains(.closed), back.flags.contains(.locked), back.key)
    let keyed = try? JSONDecoder().decode(OSDoor.self, from: Data("{\"flags\":{\"rawValue\":8},\"key\":3}".utf8))
    print(keyed == nil)
}
