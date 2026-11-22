import Foundation

struct NUShape: Decodable {
    let kind: String
    let size: Int
}

struct NURoom: Decodable {
    let vnum: Int
    let shapes: [NUShape]
    let skipped: Int

    enum CodingKeys: String, CodingKey { case vnum, shapes }
    enum ShapeKeys: String, CodingKey { case kind }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        vnum = try values.decode(Int.self, forKey: .vnum)
        var entries = try values.nestedUnkeyedContainer(forKey: .shapes)
        var known: [NUShape] = []
        var skipped = 0
        while !entries.isAtEnd {
            let entry = try entries.superDecoder()
            let header = try entry.container(keyedBy: ShapeKeys.self)
            let kind = try header.decode(String.self, forKey: .kind)
            if kind == "circle" || kind == "square" {
                known.append(try NUShape(from: entry))
            } else {
                skipped += 1
            }
        }
        shapes = known
        self.skipped = skipped
    }
}

public func nestedUnkeyedDecoding() {
    let json = #"{"vnum": 3001, "shapes": [{"kind": "circle", "size": 2}, {"kind": "blob"}, {"kind": "square", "size": 5}]}"#
    let room = try! JSONDecoder().decode(NURoom.self, from: Data(json.utf8))
    print(room.vnum, room.shapes.map { "\($0.kind):\($0.size)" }, room.skipped)
    let tiers = [1, 2, 3, 4]
    print(tiers.reversed().compactMap { $0 % 2 == 0 ? "t\($0)" : nil })
}
