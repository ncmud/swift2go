import Foundation

private struct OauRecord { let key: UUID; let name: String }

private func oauSame<T: Equatable>(_ a: T, _ b: T) -> Bool { a == b }

public func optionalArrayUpcast() {
    let rooms = ["north", "south"]
    let walked: [String?] = ["north", nil]
    print(oauSame(walked, rooms), oauSame(["north", "south"] as [String?], rooms))
    let id = UUID(uuidString: "6E2D4E2B-0E9B-4C0A-9D59-1B7B6F0F9A11")!
    let records = [OauRecord(key: id, name: "one")]
    let made: OauRecord? = records.first
    print(oauSame(records.map(\.key), [made?.key]), oauSame(records.map(\.name), [made?.name]))
    var copies: [Int?] = [1, 2].map { $0 }
    copies[0] = nil
    print(copies)
}
