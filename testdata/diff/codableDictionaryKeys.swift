import Foundation

enum CodedSlot: String, Codable { case head, body }
enum CodedKind: Int, Codable { case a = 1, b = 2 }
enum CodedBare: Codable { case x, y }
struct CodedInner: Codable, Hashable { var n: Int }
struct CodedSave: Codable {
    var bySlot: [CodedSlot: Int]
    var byKind: [CodedKind: String]
    var byBare: [CodedBare: Int]
    var byInt: [Int: String]
    var byString: [String: Int]
    var byInner: [CodedInner: Int]
    var f: Float
    var z: Double
    var s: String
}

public func codableDictionaryKeys() {
    let s = CodedSave(bySlot: [.head: 1], byKind: [.b: "two"], byBare: [.y: 3], byInt: [5: "five"], byString: ["k": 1], byInner: [CodedInner(n: 9): 8], f: 0.1, z: -0.0, s: "a/b")
    let enc = JSONEncoder()
    enc.outputFormatting = [.sortedKeys]
    let data = try! enc.encode(s)
    print(String(data: data, encoding: .utf8)!)
    let back = try! JSONDecoder().decode(CodedSave.self, from: data)
    print(back.bySlot[.head]!, back.byKind[.b]!, back.byBare[.y]!, back.byInner[CodedInner(n: 9)]!, back.f)
    let keyed = try? JSONDecoder().decode([CodedSlot: Int].self, from: Data("{\"head\":1}".utf8))
    let odd = try? JSONDecoder().decode([CodedSlot: Int].self, from: Data("[\"head\"]".utf8))
    print(keyed == nil, odd == nil)
}
