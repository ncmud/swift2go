enum Channel { case fire, cold }
enum Slot: String { case head, body }
enum Level: Int { case low = 1, high = 5 }
enum RoomRef: CustomStringConvertible {
    case world(Int), nowhere
    var description: String {
        switch self {
        case .world(let v): return "#\(v)"
        case .nowhere: return "nowhere"
        }
    }
}
enum Mood: CustomStringConvertible {
    case calm, angry
    var description: String { self == .calm ? "CALM" : "ANGRY" }
}
enum Loc { case room(Int), carried(by: String, slot: Int), nowhere }
struct Money: CustomStringConvertible { var gold: Int; var description: String { "\(gold)gp" } }
struct Plain { var a = 1; var b = "x" }
struct Nested { var p = Plain(); var tags = ["t"]; var o: Int? = nil; var c = Channel.fire }
final class Box { var v = 1 }

public func describeValues() {
    print("\(Channel.cold)")
    print("\(Slot.body)")
    print("\(Level.high)")
    print("\(RoomRef.world(3001))")
    print("\(RoomRef.nowhere)")
    print("\(Mood.angry)")
    print("\(Loc.carried(by: "orc", slot: 2))")
    print("\(Loc.room(7))")
    print("\(Loc.nowhere)")
    print("\(Money(gold: 5))")
    print("\(Plain())")
    print("\(Nested())")
    print("\([Channel.fire])")
    print("\([Money(gold: 1)])")
    print("\([RoomRef.world(1)])")
    print(String(describing: Slot.head))
    print(String(describing: Channel.fire))
    print("\(["k": 1])")
    print("\(["k": "v"])")
    print("\([Int: String]())")
    print("\((1, "a"))")
    print("\((1, (2, "x")))")
    print("\(Optional(3) as Any)")
    print("\([1, nil, 3] as [Int?])")
    print("\(["s", "t"])")
    print("\([Character("c")])")
    print("\(Set([5]))")
    print("\(Box().v)")
    print(Box())
    let noSlot: Slot? = nil, someSlot: Slot? = .head, someLoc: Loc? = .room(2), noPlain: Plain? = nil
    print("\(noSlot as Any) \(someSlot as Any) \(someLoc as Any) \(noPlain as Any)")
    print([noSlot, someSlot])
    print(["a\u{1}'\"\\\t\n\r\0é\u{7f}x"])
    print([Substring("sub")])
    print(String(reflecting: "x"), String(reflecting: Slot.head))
    print([Int8(3)], [UInt(4)])
}
