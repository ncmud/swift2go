import DiffBase
import Foundation

private struct GosByte: OptionSet, Codable {
    let rawValue: Int8
    static let top = GosByte(rawValue: 1 << 7)
    static let low = GosByte(rawValue: 1 << 0)
}

private struct GosLocal: FlagBits {
    let rawValue: Int64
    static let one = GosLocal(rawValue: 1 << 0)
    static let two = GosLocal(rawValue: 1 << 1)
    static let allFlags: [GosLocal] = [.one, .two]
    var name: String { self == .one ? "ONE" : self == .two ? "TWO" : "" }
    var description: String { "local:" + setNames.joined(separator: ",") }
}

private func gosDescribe<F: FlagBits>(_ flags: F) -> String { flags.description }

private func gosSetOperations<F: OptionSet>(_ flags: [F], unknown: F) -> Bool where F.RawValue == Int64, F.Element == F {
    var value = unknown
    for flag in flags { value.insert(flag) }
    var all = true
    for flag in flags {
        all = all && value.contains(flag)
        value.remove(flag)
    }
    return all && value.rawValue == unknown.rawValue
}

private func gosRoundTrip<F: OptionSet & Codable>(_ type: F.Type, json: String) -> String where F.RawValue: FixedWidthInteger {
    let value = try! JSONDecoder().decode(type, from: Data(json.utf8))
    return "\(value.rawValue) " + String(decoding: try! JSONEncoder().encode(value), as: UTF8.self)
}

public func genericOptionSet() {
    print(GosByte.top.rawValue, GosByte.low.rawValue, GosByte([.top, .low]).rawValue)
    print(gosDescribe(BaseColors([.red, .blue])), gosDescribe(BaseColors([])))
    print(gosDescribe(GosLocal([.two])))
    print(gosSetOperations([BaseColors.red, .blue], unknown: BaseColors(rawValue: 1 << 40)))
    print(gosRoundTrip(GosByte.self, json: "-1"))
    print(gosRoundTrip(BaseColors.self, json: "33"))
}
