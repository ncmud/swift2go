// `guard case .a(let x?, _) = optional` tests the case and the payload, not only the Optional.

final class GCPlayer {
    var room: Int
    init(room: Int) { self.room = room }
}

enum GCKind {
    case blow(from: GCPlayer?)
    case burning(from: GCPlayer?, element: Int)
    case toll
}

struct GCRun {
    var injury: GCKind?
    var victimRoom = 1
    var isDistant: Bool {
        guard case .burning(let applier?, _) = injury else { return false }
        return applier.room != victimRoom
    }
    var element: Int {
        guard case .burning(_, let element)? = injury else { return -1 }
        return element
    }
}

public func guardCaseOptionalPayload() {
    let runs = [
        GCRun(injury: nil), GCRun(injury: .toll), GCRun(injury: .blow(from: GCPlayer(room: 2))),
        GCRun(injury: .burning(from: nil, element: 3)), GCRun(injury: .burning(from: GCPlayer(room: 2), element: 4)),
        GCRun(injury: .burning(from: GCPlayer(room: 1), element: 5)),
    ]
    print(runs.map { $0.isDistant }, runs.map { $0.element })
}
