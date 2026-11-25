import Base

class Mobile: Entity {}

final class Player: Entity {}

public func isItem(_ e: Entity) -> Bool { e is Item }
public func isPlayer(_ e: Entity) -> Bool { e is Player }
public func asPlayer(_ e: Entity) -> Bool { (e as? Player) != nil }
public func mustPlayer(_ e: Any) -> Bool { (e as! Player) === e as AnyObject }
public func isMobile(_ e: Entity) -> Bool { e is Mobile }
public func switchPlayer(_ e: Any) -> Bool {
    switch e {
    case let p as Player: return p === e as AnyObject
    default: return false
    }
}
