import Foundation

public func dataKeyedDictionary() {
    var byKey: [String: [Data: String]] = [:]
    byKey["ann", default: [:]][Data("k1".utf8)] = "one"
    byKey["ann", default: [:]][Data("k2".utf8)] = "two"
    byKey["ann", default: [:]][Data("k1".utf8)] = "uno"
    print(byKey["ann"]?[Data("k1".utf8)] ?? "nil", byKey["ann"]?.count ?? 0, byKey["bob"]?[Data("k1".utf8)] ?? "nil")
    let seen: Set<Data> = [Data([1, 2]), Data([1, 2]), Data([3])]
    print(seen.count, seen.contains(Data([3])))
}
