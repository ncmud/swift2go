import Foundation

struct DEMInner: Codable {
    var count: Int
    var label: String
}

enum DEMColor: String, Codable { case red, green }

struct DEMMore: Codable {
    var s: String?
    var i: Int?
    var d: Double?
    var small: Int8?
    var u: UInt?
    var arr: [Int]?
    var inner: DEMInner?
}

struct DEMOuter: Codable {
    var name: String
    var level: Int
    var flag: Bool
    var color: DEMColor
    var items: [DEMInner]
    var note: String?
}

func demDescribe(_ error: any Error) -> String {
    guard let e = error as? DecodingError else { return "not a DecodingError" }
    let kind: String
    let context: DecodingError.Context
    switch e {
    case .typeMismatch(_, let c): kind = "typeMismatch"; context = c
    case .valueNotFound(_, let c): kind = "valueNotFound"; context = c
    case .keyNotFound(let key, let c): kind = "keyNotFound(\(key.stringValue))"; context = c
    case .dataCorrupted(let c): kind = "dataCorrupted"; context = c
    @unknown default: return "unknown"
    }
    let path = context.codingPath.map { $0.intValue.map { "[\($0)]" } ?? $0.stringValue }.joined(separator: "/")
    return "\(kind) at \(path): \(context.debugDescription)"
}

public func decodingErrorMessages() {
    let valid = #"{"name":"a","level":1,"flag":true,"color":"red","items":[{"count":1,"label":"x"}]}"#
    let cases = [
        valid,
        #"{"level":1,"flag":true,"color":"red","items":[]}"#,
        #"{"name":5,"level":1,"flag":true,"color":"red","items":[]}"#,
        #"{"name":"a","level":"one","flag":true,"color":"red","items":[]}"#,
        #"{"name":"a","level":1.5,"flag":true,"color":"red","items":[]}"#,
        #"{"name":"a","level":null,"flag":true,"color":"red","items":[]}"#,
        #"{"name":"a","level":1,"flag":1,"color":"red","items":[]}"#,
        #"{"name":"a","level":1,"flag":true,"color":"blue","items":[]}"#,
        #"{"name":"a","level":1,"flag":true,"color":"red","items":{}}"#,
        #"{"name":"a","level":1,"flag":true,"color":"red","items":[{"count":1}]}"#,
        #"{"name":"a","level":1,"flag":true,"color":"red","items":[{"count":1,"label":"x"},{"count":"2","label":"y"}]}"#,
        #"{"name":"a","level":1,"flag":true,"color":"red","items":[],"note":7}"#,
        #"[1,2]"#,
    ]
    let more = [
        #"{"s":true}"#, #"{"i":true}"#, #"{"i":{}}"#, #"{"s":[]}"#, #"{"d":"x"}"#, #"{"small":300}"#,
        #"{"u":-1}"#, #"{"arr":[1,null]}"#, #"{"arr":"x"}"#, #"{"inner":5}"#, #"{"inner":{"count":null,"label":"x"}}"#,
        #"{"i":1e2}"#, #"{"i":"5"}"#,
    ]
    for json in more {
        do {
            _ = try JSONDecoder().decode(DEMMore.self, from: Data(json.utf8))
            print("ok")
        } catch {
            print(demDescribe(error))
        }
    }
    for json in cases {
        do {
            _ = try JSONDecoder().decode(DEMOuter.self, from: Data(json.utf8))
            print("ok")
        } catch {
            print(demDescribe(error))
        }
    }
}
