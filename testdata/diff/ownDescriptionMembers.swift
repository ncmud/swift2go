enum ODRace: Int, CaseIterable {
    case human = 1, giant
    var description: String { "the \(self == .human ? "humans" : "giants") of old" }
}

struct ODPoint {
    var x: Int
    var description: String { "point \(x)" }
}

struct ODLabel: CustomStringConvertible {
    var text: String
    var description: String { "label \(text)" }
}

enum ODMood: CustomStringConvertible {
    case calm
    var description: String { "very calm" }
}

public func ownDescriptionMembers() {
    print(ODRace.allCases.map { String(describing: $0) }, ODRace.human.description)
    print(String(describing: ODPoint(x: 2)), ODPoint(x: 3).description)
    print(ODLabel(text: "a"), "\(ODMood.calm)")
    print("\(ODRace.giant)")
}
