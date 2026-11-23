import Foundation

private struct GssMark: Codable, Equatable { let route: Int }

private struct GssState {
    private var docs: [String: String] = [:]
    subscript<T: Codable>(key: String) -> T? {
        get { docs[key].flatMap { try? JSONDecoder().decode(T.self, from: Data($0.utf8)) } }
        set {
            if let v = newValue, let d = try? JSONEncoder().encode(v) { docs[key] = String(decoding: d, as: UTF8.self) } else { docs[key] = nil }
        }
    }
    var count: Int { docs.count }
}

private final class GssHolder { var state = GssState() }

public func genericSubscriptSet() {
    var s = GssState()
    s["flag"] = true
    s["mark"] = GssMark(route: 4)
    let flag: Bool? = s["flag"]
    let mark: GssMark? = s["mark"]
    print(flag ?? false, mark?.route ?? -1, s.count)
    s["flag"] = nil as Bool?
    print(s.count)
    let h = GssHolder()
    h.state["n"] = 3
    let n: Int? = h.state["n"]
    print(n ?? 0)
}
