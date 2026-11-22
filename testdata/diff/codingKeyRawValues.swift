import Foundation

struct CKRow: Codable, Equatable {
    let pker: Int
    let pkee: Int
    let kills: Int

    enum CodingKeys: String, CodingKey {
        case pker = "pkerClubId"
        case pkee = "pkeeClubId"
        case kills
    }
}

public func codingKeyRawValues() {
    let row = CKRow(pker: 1, pkee: 2, kills: 7)
    let enc = JSONEncoder()
    enc.outputFormatting = [.sortedKeys]
    print(String(data: try! enc.encode(row), encoding: .utf8)!)
    let json = Data("{\"pkerClubId\":3,\"pkeeClubId\":4,\"kills\":9}".utf8)
    let back = try! JSONDecoder().decode(CKRow.self, from: json)
    print(back.pker, back.pkee, back.kills)
    print(back)
    let wrong = try? JSONDecoder().decode(CKRow.self, from: Data("{\"pker\":3,\"pkee\":4,\"kills\":9}".utf8))
    print(wrong == nil)
}
