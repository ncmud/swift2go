public protocol FlagBits: OptionSet where Element == Self {
    static var allFlags: [Self] { get }
    var name: String { get }
    var description: String { get }
}

public extension FlagBits {
    var setNames: [String] {
        Self.allFlags.filter { contains($0) && !$0.name.isEmpty }.map(\.name)
    }

    var description: String { setNames.isEmpty ? "NONE" : setNames.joined(separator: " ") }
}

public struct BaseColors: FlagBits, Codable, Sendable {
    public let rawValue: Int64
    public init(rawValue: Int64) { self.rawValue = rawValue }
    public static let red = BaseColors(rawValue: 1 << 0)
    public static let blue = BaseColors(rawValue: 1 << 5)
    public static let allFlags: [BaseColors] = [.red, .blue]
    public var name: String {
        switch self {
        case .red: return "RED"
        case .blue: return "BLUE"
        default: return ""
        }
    }
}
