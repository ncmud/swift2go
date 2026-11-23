import Foundation

struct RCProfile: Codable {
    var bounds: ClosedRange<Int64>
    var window: Range<Double>
    var levels: [ClosedRange<Int>]
}

func rcDecode<T: Decodable>(_ type: T.Type, _ json: String) -> String {
    do {
        return "\(try JSONDecoder().decode(T.self, from: Data(json.utf8)))"
    } catch {
        return "\(error)"
    }
}

public func rangeCodable() {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.sortedKeys]
    let p = RCProfile(bounds: -1_000_000_000_000...1_000_000_000_000, window: 0.5..<2.0, levels: [1...5, 7...7])
    let data = try! encoder.encode(p)
    print(String(decoding: data, as: UTF8.self))
    let back = try! JSONDecoder().decode(RCProfile.self, from: data)
    print(back.bounds, back.window, back.levels)
    print(rcDecode(ClosedRange<Int>.self, "[3,9]"))
    print(rcDecode(Range<Int>.self, "[3,3]"))
    print(rcDecode(ClosedRange<Int>.self, "[9,3]"))
    print(rcDecode(Range<Int>.self, "[9,3]"))
    print(rcDecode(ClosedRange<Int>.self, "[1]"))
    print(rcDecode(ClosedRange<Int>.self, "{}"))
    print(rcDecode(ClosedRange<Int>.self, "[1,2,3]"))
}
