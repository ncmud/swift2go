import Foundation

struct LeftOutRow: Codable {
    var name: String
    var retries = 3
    var label: String = "none"
    var note: String? = "unset"
    var tags: [String] = ["a", "b"]

    enum CodingKeys: String, CodingKey {
        case name = "rowName"
    }
}

final class LeftOutBox: Codable {
    var size: Int
    var color = "red"

    enum CodingKeys: CodingKey {
        case size
    }

    init(size: Int) { self.size = size }
}

public func codingKeysLeftOut() {
    let row = try! JSONDecoder().decode(LeftOutRow.self, from: Data("{\"rowName\":\"x\",\"retries\":9}".utf8))
    print(row.name, row.retries, row.label, row.note ?? "nil", row.tags)
    let enc = JSONEncoder()
    enc.outputFormatting = [.sortedKeys]
    print(String(data: try! enc.encode(row), encoding: .utf8)!)
    let rows = try! JSONDecoder().decode([LeftOutRow].self, from: Data("[{\"rowName\":\"y\"}]".utf8))
    print(rows[0].retries, rows[0].label)
    let box = try! JSONDecoder().decode(LeftOutBox.self, from: Data("{\"size\":4,\"color\":\"blue\"}".utf8))
    print(box.size, box.color)
}
