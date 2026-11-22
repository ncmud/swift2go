import Foundation

// A type's own init(from:) can call the one Swift synthesizes for another type.

private struct SynthesizedStoredColumn: Decodable {
    let percent: Int
    let floor: Int?
    let label: String
}

struct SynthesizedColumn: Decodable {
    var percent: Int
    var floor: Int
    var label: String

    init(from decoder: any Decoder) throws {
        let stored = try SynthesizedStoredColumn(from: decoder)
        percent = stored.percent
        floor = stored.floor ?? -1
        label = stored.label.uppercased()
    }
}

public func synthesizedDecodable() {
    let decoder = JSONDecoder()
    let full = try! decoder.decode(SynthesizedColumn.self, from: Data(#"{"percent":200,"floor":40,"label":"boss"}"#.utf8))
    let sparse = try! decoder.decode([SynthesizedColumn].self, from: Data(#"[{"percent":150,"label":"elite"}]"#.utf8))
    print(full.percent, full.floor, full.label)
    print(sparse[0].percent, sparse[0].floor, sparse[0].label)
    do {
        _ = try decoder.decode(SynthesizedColumn.self, from: Data(#"{"floor":1}"#.utf8))
        print("decoded")
    } catch {
        print("missing key")
    }
}
