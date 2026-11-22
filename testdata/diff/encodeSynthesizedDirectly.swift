import Foundation
struct ENCInner: Codable { var a: Int; var b: String }
struct ENCOwn: Codable {
    var c: Int
    func encode(to encoder: Encoder) throws { var k = encoder.singleValueContainer(); try k.encode("own\(c)") }
    init(c: Int) { self.c = c }
    init(from decoder: Decoder) throws { c = Int(try decoder.singleValueContainer().decode(String.self).dropFirst(3))! }
}
enum ENCChoice: Codable {
    case inner(ENCInner)
    case own(ENCOwn)
    func encode(to encoder: Encoder) throws {
        switch self {
        case .inner(let v): try v.encode(to: encoder)
        case .own(let v): try v.encode(to: encoder)
        }
    }
    init(from decoder: Decoder) throws {
        if let v = try? ENCOwn(from: decoder) { self = .own(v); return }
        self = .inner(try ENCInner(from: decoder))
    }
}
public func encodeSynthesizedDirectly() {
    let enc = JSONEncoder()
    enc.outputFormatting = .sortedKeys
    for c in [ENCChoice.inner(ENCInner(a: 1, b: "x")), .own(ENCOwn(c: 7))] {
        let data = try! enc.encode(c)
        print(String(data: data, encoding: .utf8)!)
        let back = try! JSONDecoder().decode(ENCChoice.self, from: data)
        print(String(data: try! enc.encode(back), encoding: .utf8)!)
    }
}
