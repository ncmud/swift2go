protocol CFTagged: AnyObject {
    var tag: String { get }
}

class CFCharacter {
    let name: String
    init(name: String) { self.name = name }
    var isPC: Bool { self is CFPlayer }
    var player: CFPlayer? { self as? CFPlayer }
    var asHero: CFHero? { self as? CFHero }
}

class CFPlayer: CFCharacter, CFTagged {
    var tag: String { "player \(name)" }
}

final class CFHero: CFPlayer {
    override var tag: String { "hero \(name)" }
}

final class CFShopkeeper: CFCharacter {}

class CFPet {
    var owner: CFCharacter?
    init(owner: CFCharacter?) { self.owner = owner }
}

final class CFFamiliar: CFPet {}

func cfDescribe(_ c: CFCharacter) -> String {
    let player = c.player.map { $0.name } ?? "-"
    let hero = c.asHero.map { $0.name } ?? "-"
    let tagged = (c as? CFTagged)?.tag ?? "-"
    let shop = (c as? CFShopkeeper).map { $0.name } ?? "-"
    return "\(c.name): isPC=\(c.isPC) player=\(player) hero=\(hero) tagged=\(tagged) shop=\(shop)"
}

public func classCastFailures() {
    let everyone: [CFCharacter] = [
        CFCharacter(name: "goblin"), CFPlayer(name: "bonzo"), CFHero(name: "ash"),
        CFShopkeeper(name: "grocer"), CFCharacter(name: "rat"),
    ]
    // The failing casts run many times, as an NPC's isPC does on every pulse.
    var npcs = 0
    for _ in 0..<1000 {
        npcs += everyone.filter { !$0.isPC }.count
    }
    print("npc casts", npcs)
    for c in everyone {
        print(cfDescribe(c))
    }
    let anys: [Any] = everyone + [CFPet(owner: nil), CFFamiliar(owner: everyone[1]), 3, "four"]
    for a in anys {
        let character = (a as? CFCharacter)?.name ?? "-"
        let player = (a as? CFPlayer)?.name ?? "-"
        let pet = a is CFPet
        let familiar = (a as? CFFamiliar)?.owner?.name ?? "-"
        print("character=\(character) player=\(player) pet=\(pet) familiar=\(familiar)")
    }
    let pets: [CFPet] = [CFPet(owner: everyone[0]), CFFamiliar(owner: everyone[2])]
    for p in pets {
        print(p is CFFamiliar, (p as? CFFamiliar)?.owner?.isPC ?? false, p.owner?.player?.tag ?? "-")
    }
}
