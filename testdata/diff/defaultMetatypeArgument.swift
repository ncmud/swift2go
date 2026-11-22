import Foundation
struct DMAPoint: Codable { var x: Int }
struct DMALabel: Codable { var x: Int; var note: String? }

func dmaConvert<L: Encodable, S: Decodable>(_ value: L, as type: S.Type = S.self) -> S {
    let data = try! JSONEncoder().encode(value)
    return try! JSONDecoder().decode(S.self, from: data)
}

func dmaCount<T: Decodable>(_ json: String, _ type: T.Type = T.self) -> Int {
    (try? JSONDecoder().decode([T].self, from: Data(json.utf8)))?.count ?? -1
}

public func defaultMetatypeArgument() {
    let label: DMALabel = dmaConvert(DMAPoint(x: 4))
    print(label.x, label.note ?? "nil")
    print(dmaConvert(DMAPoint(x: 5), as: DMAPoint.self).x)
    let points: [DMAPoint] = [DMALabel(x: 1, note: "a"), DMALabel(x: 2, note: nil)].map { dmaConvert($0) }
    print(points.map(\.x))
    print(dmaCount("[1,2,3]", Int.self), dmaCount("[{\"x\":1}]", DMAPoint.self))
}
