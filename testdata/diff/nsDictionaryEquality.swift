import Foundation

private func ndeObject(_ json: String) -> NSDictionary {
    try! JSONSerialization.jsonObject(with: Data(json.utf8)) as! NSDictionary
}

private func ndeSame<T: Equatable>(_ a: T, _ b: T) -> Bool { a == b }

public func nsDictionaryEquality() {
    let d = ndeObject(#"{"active": false, "count": 3, "ratio": 1.0, "keys": ["a", "b"], "chars": [], "nested": {"on": true}}"#)
    print(ndeSame(d, ["active": false, "count": 3, "ratio": 1, "keys": ["a", "b"], "chars": [], "nested": ["on": true]]))
    print(ndeSame(d, ["active": 0, "count": 3.0, "ratio": 1, "keys": ["a", "b"], "chars": [], "nested": ["on": 1]]))
    print(ndeSame(d, ["active": true, "count": 3, "ratio": 1, "keys": ["a", "b"], "chars": [], "nested": ["on": true]]))
    print(ndeSame(d, ["active": false, "count": 3, "ratio": 1, "keys": ["b", "a"], "chars": [], "nested": ["on": true]]))
    let rows = try! JSONSerialization.jsonObject(with: Data(#"[{"x": 2}, {"x": -1}]"#.utf8)) as! [NSDictionary]
    print(ndeSame(rows, [["x": 2], ["x": -1]]), ndeSame(rows, [["x": 2], ["x": 1]]))
}
