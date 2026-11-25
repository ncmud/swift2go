protocol MEPart {}
struct MEMarker: MEPart { let label: String }
struct MESecond: MEPart {}
class MEBase: MEPart {}
final class MEDerived: MEBase {}
enum MEMove: MEPart { case step(Int), leap(Int, Int), rest }
enum MEBox<T>: MEPart { case full(T), empty }

struct MEMade {}
func meMake() -> MEMade { print("made"); return MEMade() }

final class MERegistry {
    var parts: [any MEPart] = []
    // Registers a part once per type, as a registry keyed on type(of:) does.
    func register(_ part: any MEPart) {
        guard !parts.contains(where: { type(of: $0) == type(of: part) }) else { return }
        parts.append(part)
    }
}

public func metatypeEquality() {
    let parts: [any MEPart] = [MEMarker(label: "a"), MESecond(), MEBase(), MEDerived(), MEMove.step(1), MEMove.rest,
                               MEBox<Int>.full(1), MEBox<Int>.empty, MEBox<String>.full("s")]
    for x in parts {
        print(parts.map { type(of: $0) == type(of: x) ? "1" : "0" }.joined())
        print(parts.map { type(of: $0) != type(of: x) ? "1" : "0" }.joined())
    }
    print(type(of: parts[0]) == MEMarker.self, type(of: parts[0]) == MESecond.self)
    print(type(of: parts[3]) == MEDerived.self, type(of: parts[3]) == MEBase.self, type(of: parts[2]) == MEBase.self)
    print(type(of: parts[4]) == MEMove.self, type(of: parts[5]) == MEMove.self, type(of: parts[6]) == MEMove.self)
    print(type(of: parts[7]) == MEBox<Int>.self, type(of: parts[8]) == MEBox<Int>.self)
    let none: Any.Type? = nil
    let some: Any.Type? = MESecond.self
    print(none == nil, some == nil, type(of: parts[1]) == none, type(of: parts[1]) == some, none != some)
    print(type(of: 1) == type(of: 2), type(of: 1) == type(of: "s"), type(of: 1) == Int.self)

    let registry = MERegistry()
    registry.register(MEMarker(label: "first"))
    registry.register(MESecond())
    registry.register(MEMarker(label: "again"))
    registry.register(MEMove.rest)
    registry.register(MEMove.step(2))
    print(registry.parts.count, registry.parts.compactMap { ($0 as? MEMarker)?.label })

    // A metatype held as a value, or taken from a value, is the type at run time, not the static one.
    let base: MEBase = MEDerived()
    print(type(of: base) == MEDerived.self, type(of: base) == MEBase.self)
    let held: MEBase.Type = MEDerived.self
    let erased: Any.Type = held
    print(erased == MEDerived.self, erased == MEBase.self, erased)
    let made: Any.Type = type(of: meMake())
    print(made == MEMade.self)
}
