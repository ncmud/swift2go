import DiffBase

class FCBeing {
    let name: String
    init(name: String) { self.name = name }
    var isPC: Bool { self is FCPlayer }
    var player: FCPlayer? { self as? FCPlayer }
}

class FCCharacter: FCBeing {}

class FCMobile: FCCharacter {}

final class FCPlayer: FCCharacter {}

final class FCGuard: FCMobile {}

final class FCLoner {
    let id: Int
    init(id: Int) { self.id = id }
}

func fcDescribe(_ b: FCBeing?) -> String {
    let player = (b as? FCPlayer)?.name ?? "-"
    let guardName = (b as? FCGuard)?.name ?? "-"
    return "isPlayer=\(b is FCPlayer) isGuard=\(b is FCGuard) player=\(player) guard=\(guardName)"
}

func fcDescribeAny(_ a: Any) -> String {
    let player = (a as? FCPlayer)?.name ?? "-"
    let loner = (a as? FCLoner).map { "\($0.id)" } ?? "-"
    return "isPlayer=\(a is FCPlayer) isLoner=\(a is FCLoner) player=\(player) loner=\(loner)"
}

func fcSwitch(_ a: Any) -> String {
    switch a {
    case let p as FCPlayer: return "player \(p.name)"
    case is FCGuard: return "guard"
    case let l as FCLoner: return "loner \(l.id)"
    default: return "other"
    }
}

public func finalClassCasts() {
    let player = FCPlayer(name: "ash")
    let beings: [FCBeing?] = [
        player, FCGuard(name: "sentry"), FCMobile(name: "rat"), FCCharacter(name: "ghost"), FCBeing(name: "wisp"), nil,
    ]
    for b in beings {
        print(fcDescribe(b))
    }
    var npcs = 0
    for _ in 0..<1000 {
        npcs += beings.compactMap { $0 }.filter { !$0.isPC }.count
    }
    print("npc casts", npcs)

    let asCharacter: FCCharacter = player
    print(asCharacter is FCPlayer, (asCharacter as? FCPlayer)?.name ?? "-", (asCharacter as! FCPlayer).name)
    let asBeing: FCBeing = FCGuard(name: "warden")
    print(asBeing is FCPlayer, asBeing.player?.name ?? "-", (asBeing as! FCGuard).name)

    let noPlayer: FCPlayer? = nil
    print(noPlayer is FCPlayer, (noPlayer as Any) is FCPlayer)
    let noBeing: FCBeing? = nil
    print(noBeing is FCPlayer, (noBeing as? FCPlayer) == nil)

    let anys: [Any] = [player, FCGuard(name: "sentry"), FCMobile(name: "rat"), FCLoner(id: 7), 3, "four", noPlayer as Any]
    for a in anys {
        print(fcDescribeAny(a), fcSwitch(a))
    }
    print((anys[0] as! FCPlayer).name, (anys[3] as! FCLoner).id)

    final class FCLocalEntity: FCBaseEntity {}
    let entities: [FCBaseEntity] = [FCBaseItem(label: "sword"), FCLocalEntity(label: "rock"), FCBaseEntity(label: "dust")]
    for x in entities {
        print(x.label, x is FCBaseItem, (x as? FCBaseItem)?.label ?? "-", (x as Any) is FCBaseItem)
    }
    print((entities[0] as! FCBaseItem).label)
}
