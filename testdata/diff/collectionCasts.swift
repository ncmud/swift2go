protocol CCShape { var sides: Int { get } }
struct CCSquare: CCShape, Hashable { var sides: Int { 4 } }
final class CCNode { let id: Int; init(_ id: Int) { self.id = id } }

func ccDescribe(_ v: Any) -> String {
    var parts: [String] = []
    if let d = v as? [String: Any] { parts.append("dictAny=\(d.count)") }
    if let d = v as? [String: Int] { parts.append("dictInt=\(d.keys.sorted())") }
    if let d = v as? [String: String] { parts.append("dictString=\(d.count)") }
    if let d = v as? [AnyHashable: Any] { parts.append("anyHashable=\(d.count)") }
    if let a = v as? [Any] { parts.append("arrAny=\(a.count)") }
    if let a = v as? [Int] { parts.append("arrInt=\(a)") }
    if let a = v as? [Int?] { parts.append("arrOptInt=\(a.count)") }
    if let a = v as? [String] { parts.append("arrString=\(a)") }
    if let a = v as? [CCShape] { parts.append("arrShape=\(a.map(\.sides))") }
    if let a = v as? [CCNode] { parts.append("arrNode=\(a.map(\.id))") }
    if let a = v as? [[Any]] { parts.append("nested=\(a.count)") }
    if let a = v as? [String: [Int]] { parts.append("dictOfArr=\(a.count)") }
    if let s = v as? Set<AnyHashable> { parts.append("setAnyHashable=\(s.count)") }
    if let s = v as? Set<Int> { parts.append("setInt=\(s.sorted())") }
    if let s = v as? Set<CCSquare> { parts.append("setSquare=\(s.count)") }
    if v is [Any] { parts.append("is[Any]") }
    if v is [String: Any] { parts.append("is[String:Any]") }
    return parts.isEmpty ? "none" : parts.joined(separator: " ")
}

// Collections whose keys are arrays. AnyHashable is a Go interface, which cannot key a Go map
// with an array, so these are not cast to AnyHashable-keyed collections here.
func ccHashed(_ v: Any) -> String {
    var parts: [String] = []
    if let d = v as? [[Int]: Any] { parts.append("arrKeyedAny=\(d.count)") }
    if let d = v as? [[Int]: Int] { parts.append("arrKeyedInt=\(d.keys.sorted { $0.count < $1.count })") }
    if let d = v as? [[Int]: String] { parts.append("arrKeyedString=\(d.count)") }
    if let s = v as? Set<[Int]> { parts.append("setOfArr=\(s.count)") }
    if let s = v as? Set<[Int8]> { parts.append("setOfInt8Arr=\(s.count)") }
    return parts.isEmpty ? "none" : parts.joined(separator: " ")
}

public func collectionCasts() {
    let samples: [(String, Any)] = [
        ("dictInt", ["b": 2, "a": 1] as [String: Int]),
        ("dictMixed", ["a": 1, "b": "x"] as [String: Any]),
        ("dictEmpty", [:] as [String: Int]),
        ("arrInt", [3, 1, 2]),
        ("arrMixed", [1, "two"] as [Any]),
        ("arrAnyInts", [5, 6] as [Any]),
        ("arrOpt", [1, nil, 3] as [Int?]),
        ("arrOptFull", [1, 2] as [Int?]),
        ("arrEmpty", [] as [Int]),
        ("arrString", ["x", "y"]),
        ("arrSquare", [CCSquare(), CCSquare()]),
        ("arrNode", [CCNode(1), CCNode(2)]),
        ("arrNodeAsAny", [CCNode(7)] as [Any]),
        ("nested", [[1, 2], [3]] as [[Int]]),
        ("dictOfArr", ["a": [1]] as [String: [Int]]),
        ("dictOfArrAny", ["a": [1] as [Any]] as [String: Any]),
        ("setInt", Set([3, 1])),
        ("setSquare", Set([CCSquare()])),
        ("intDict", [1: "one"] as [Int: String]),
        ("string", "hello"),
        ("int", 4),
    ]
    for (name, v) in samples {
        print("\(name): \(ccDescribe(v))")
    }
    let hashed: [(String, Any)] = [
        ("arrKeyed", [[1]: 1, [2, 3]: 2] as [[Int]: Int]),
        ("arrKeyedAny", [[1]: "a"] as [[Int]: Any]),
        ("setOfArr", Set([[1], [2]])),
        ("arrInt", [1]),
    ]
    for (name, v) in hashed {
        print("\(name): \(ccHashed(v))")
    }
    let forced = (["k": 9] as [String: Int]) as Any
    let d = forced as! [String: Any]
    print("forced \(d["k"] as? Int ?? -1)")
    let arr = ([1, 2, 3] as [Int]) as Any
    let anys = arr as! [Any]
    print("forcedArr \(anys.compactMap { $0 as? Int }.reduce(0, +))")
}
