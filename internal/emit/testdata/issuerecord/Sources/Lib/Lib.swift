public struct Row: Equatable {
    public var cells: [String]
    public init(_ cells: [String]) { self.cells = cells }
}

public struct Score: Comparable {
    public var points: Int
    public init(_ points: Int) { self.points = points }
    public static func < (lhs: Score, rhs: Score) -> Bool { lhs.points > rhs.points }
}

public final class Room: Sendable {
    public let vnum: Int
    public init(vnum: Int) { self.vnum = vnum }
    public static let shared: Room? = Room(vnum: 3001)
}

public struct LibError: Error {}
