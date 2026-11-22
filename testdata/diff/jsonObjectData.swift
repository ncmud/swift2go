import Foundation

public func jsonObjectData() {
    let data = try! JSONSerialization.data(withJSONObject: ["b": 2, "a": [1, 2]])
    let back = try! JSONSerialization.jsonObject(with: data) as! [String: Any]
    print(back.keys.sorted(), back["b"] as! Int)
}
