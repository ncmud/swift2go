import Foundation

enum DEDKind: String, Codable { case a }
struct DEDItem: Codable { let kind: DEDKind; let n: Int }
struct DEDDoc: Codable { let items: [DEDItem]; let name: String }
struct DEDUnderlying: Error, CustomStringConvertible { var description: String { "custom underlying" } }

func dedDecode<T: Decodable>(_ type: T.Type, _ json: String) -> String {
    do {
        _ = try JSONDecoder().decode(T.self, from: Data(json.utf8))
        return "ok"
    } catch {
        return "\(error)"
    }
}

public func decodingErrorDescriptions() {
    print(dedDecode(DEDDoc.self, #"{"name":"x","items":[{"kind":"a","n":1},{"kind":"zz","n":1}]}"#))
    print(dedDecode(DEDDoc.self, #"{"name":"x","items":[{"kind":"a","n":"s"}]}"#))
    print(dedDecode(DEDDoc.self, #"{"name":"x","items":[{"kind":"a","n":null}]}"#))
    print(dedDecode(DEDDoc.self, #"{"name":"x","items":[{"kind":"a"}]}"#))
    print(dedDecode(DEDDoc.self, #"{"items":[]}"#))
    print(dedDecode(DEDDoc.self, #"[1]"#))
    print(dedDecode([[Int]].self, #"[[1],[2,"x"]]"#))
    print(dedDecode([String: [Int]].self, #"{"a b":[1,"x"]}"#))
    print(DecodingError.dataCorrupted(.init(codingPath: [], debugDescription: "d", underlyingError: DEDUnderlying())))
    print(DecodingError.dataCorrupted(.init(codingPath: [], debugDescription: "")))
    print(EncodingError.invalidValue(1.5, .init(codingPath: [], debugDescription: "bad")))
    print(EncodingError.invalidValue("s", .init(codingPath: [], debugDescription: "")))
}
