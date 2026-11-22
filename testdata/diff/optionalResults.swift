// Functions, methods and protocol requirements that return a value type's Optional, consumed every way
// a caller can: bound, coalesced, chained, compared, switched, mapped, stored, passed on and used as
// a function value.

enum ORSKey: Equatable, CustomStringConvertible {
    case nowhere
    case room(Int)
    case instance(zone: Int, room: Int)

    var description: String {
        switch self {
        case .nowhere: return "nowhere"
        case .room(let v): return "room \(v)"
        case .instance(let z, let r): return "instance \(z)/\(r)"
        }
    }
}

struct ORSExit: Equatable {
    var keywords: [String]
    var destination: ORSKey
    var key: Int?
}

final class ORSRoom {
    var exits: [Int: ORSExit] = [:]
    var visits = 0

    func exit(_ dir: Int) -> ORSExit? { exits[dir] }

    func destination(_ dir: Int) -> ORSKey? { exit(dir)?.destination }

    func lock(_ dir: Int) -> Int? { exit(dir)?.key ?? nil }

    func firstKeyword(_ dir: Int) -> String? {
        guard let e = exit(dir), !e.keywords.isEmpty else { return nil }
        return e.keywords[0]
    }

    func countedVisit(_ dir: Int) -> Int? {
        visits += 1
        return exits[dir] == nil ? nil : visits
    }

    static func parse(_ s: String) -> Int? { Int(s) }
}

protocol ORSLookup {
    func find(_ name: String) -> Int?
}

extension ORSLookup {
    func findOrZero(_ name: String) -> Int { find(name) ?? 0 }
    func twice(_ name: String) -> Int? { find(name).map { $0 * 2 } }
}

struct ORSTable: ORSLookup {
    var names: [String]
    func find(_ name: String) -> Int? { names.firstIndex(of: name) }
}

class ORSBase {
    func score() -> Double? { nil }
}

final class ORSDerived: ORSBase {
    override func score() -> Double? { 2.5 }
}

struct ORSCounter {
    var n = 0
    mutating func next(limit: Int) -> Int? {
        guard n < limit else { return nil }
        n += 1
        return n
    }
}

func orsHalf(_ x: Int) -> Int? { x % 2 == 0 ? x / 2 : nil }

func orsThrowing(_ x: Int) throws -> Int? {
    struct Bad: Error {}
    if x < 0 { throw Bad() }
    return x == 0 ? nil : x
}

func orsDeferred(_ x: Int) -> String? {
    var log: [String] = []
    defer { log.append("done") }
    do {
        if x > 2 { return "big" }
        let v = try orsThrowing(x)
        return v.map { "v\($0)" }
    } catch {
        return nil
    }
}

func orsSwitch(_ k: ORSKey?) -> String {
    switch k {
    case .room(let v)?: return "r\(v)"
    case .some(.instance(let z, _)): return "z\(z)"
    case .nowhere?: return "nowhere"
    case nil: return "nil"
    }
}

func orsRecursive(_ n: Int) -> Int? {
    if n <= 0 { return nil }
    if n == 1 { return 1 }
    return orsRecursive(n - 1).map { $0 + n }
}

func orsVoid(_ go: Bool) -> Void? {
    if go { return () }
    return nil
}

func orsNested(_ x: Int) -> Int?? {
    if x < 0 { return nil }
    if x == 0 { return .some(nil) }
    return x
}

func orsGeneric<T>(_ xs: [T], _ i: Int) -> T? { i < xs.count ? xs[i] : nil }

func orsTuple(_ x: Int) -> (Int, String)? { x > 0 ? (x, "p\(x)") : nil }

struct ORSStack<Element> {
    var items: [Element] = []
    func depth(atLeast n: Int) -> Int? { items.count >= n ? items.count : nil }
    func top() -> Element? { items.last }
    mutating func pop() -> Element? { items.popLast() }
}

infix operator +?: AdditionPrecedence
func +? (lhs: Int, rhs: Int) -> Int? { lhs > rhs ? lhs - rhs : nil }

func orsTake(_ xs: inout [Int]) -> Int? { xs.isEmpty ? nil : xs.removeFirst() }

enum ORSDir: Int {
    case north, south
    static func named(_ s: String) -> ORSDir? { s == "n" ? .north : s == "s" ? .south : nil }
}

protocol ORSStore {
    associatedtype Item
    func count() -> Int
}

extension ORSStore where Item == Int {
    func firstInt() -> Item? { count() > 0 ? 5 : nil }
}

struct ORSIntStore: ORSStore {
    typealias Item = Int
    var n: Int
    func count() -> Int { n }
}

protocol ORSMaker {
    associatedtype Made
    func make() -> Made
}

struct ORSOptionalMaker: ORSMaker {
    func make() -> Int? { 1 }
}

struct ORSHandle {
    var raw = 4
    var ptr: Int? { raw > 0 ? raw : nil }
}

func orsHandlePtr(_ h: ORSHandle) -> Int? { return h.ptr }

func orsGenericCount<T>(_ x: T) -> Int? { "\(x)".count }

public func optionalResults() {
    let room = ORSRoom()
    room.exits[0] = ORSExit(keywords: ["door", "gate"], destination: .room(3001), key: 7)
    room.exits[2] = ORSExit(keywords: [], destination: .instance(zone: 4, room: 9), key: nil)

    // Bound, coalesced and compared.
    if let d = room.destination(0) { print("north", d) }
    guard let s = room.destination(2) else { return }
    print("south", s)
    print(room.destination(1) ?? .nowhere, room.lock(0) ?? -1, room.lock(2) ?? -1)
    print(room.destination(0) == .room(3001), room.destination(1) == nil, room.destination(2) != nil)
    print(room.firstKeyword(0) ?? "-", room.firstKeyword(2) ?? "-", room.firstKeyword(5) ?? "-")

    // Chained, switched and mapped.
    print(room.exit(0)?.keywords.count ?? 0, room.exit(0)?.destination.description ?? "")
    print([0, 1, 2].map { orsSwitch(room.destination($0)) })
    print(room.exit(0).map { $0.keywords.joined(separator: "+") } ?? "", room.exit(2).flatMap { $0.key } as Any)

    // Value semantics: the returned exit is a copy.
    var copy = room.exit(0)
    copy?.keywords.append("arch")
    copy?.key = 99
    print(copy!.keywords, room.exit(0)!.keywords, copy?.key as Any, room.exit(0)?.key as Any)
    var exitCopy = room.exit(0)!
    exitCopy.destination = .nowhere
    print(exitCopy.destination, room.destination(0)!)

    // Stored, passed on, collected.
    var seen: [ORSKey?] = []
    for dir in 0..<3 { seen.append(room.destination(dir)) }
    print(seen.map { $0?.description ?? "nil" })
    let dict: [Int: Int?] = [1: orsHalf(4), 2: orsHalf(3)]
    print(dict[1]! as Any, dict[2]! as Any, dict[3] as Any)
    func show(_ v: Int?) -> String { v.map(String.init) ?? "none" }
    print(show(orsHalf(10)), show(orsHalf(7)), show(ORSRoom.parse("42")), show(ORSRoom.parse("x")))

    // Side effects run once per call.
    print(room.countedVisit(0) as Any, room.countedVisit(9) as Any, room.countedVisit(2) as Any, room.visits)

    // Function values.
    let half = orsHalf
    let byRoom = room.destination
    print([1, 2, 3, 4].map(orsHalf), [1, 2, 3, 4].compactMap(orsHalf), half(8) as Any, byRoom(2) as Any)
    let parse: (String) -> Int? = ORSRoom.parse
    print(["1", "b", "3"].compactMap(parse))

    // Protocol requirements, extension defaults and overrides.
    let table = ORSTable(names: ["a", "b", "c"])
    let lookup: ORSLookup = table
    print(table.find("b") as Any, lookup.find("z") as Any, lookup.findOrZero("c"), lookup.twice("b") as Any, table.twice("q") as Any)
    let objs: [ORSBase] = [ORSBase(), ORSDerived()]
    print(objs.map { $0.score() ?? -1 })

    // Mutating methods, loops over a returned optional.
    var counter = ORSCounter()
    var got: [Int] = []
    while let v = counter.next(limit: 3) { got.append(v) }
    print(got, counter.next(limit: 3) as Any)

    // Throwing, do/catch and defer, recursion, Void?, nested and generic Optionals, tuples.
    print((try? orsThrowing(5)) as Any, (try? orsThrowing(0)) as Any, (try? orsThrowing(-1)) as Any)
    print(orsDeferred(1) as Any, orsDeferred(0) as Any, orsDeferred(3) as Any, orsDeferred(-2) as Any)
    print(orsRecursive(4) as Any, orsRecursive(0) as Any)
    print(orsVoid(true) != nil, orsVoid(false) == nil)
    print(orsNested(3) as Any, orsNested(0) as Any, orsNested(-1) as Any)
    print(orsGeneric([1, 2], 1) as Any, orsGeneric(["x"], 4) as Any)
    if let (n, label) = orsTuple(3) { print(n, label) }
    print(orsTuple(-1) == nil)

    // Generic types, operators, inout parameters, enums, and escaping closures that call later.
    var stack = ORSStack<String>(items: ["a", "b"])
    print(stack.depth(atLeast: 2) as Any, stack.depth(atLeast: 3) as Any, stack.top() as Any, stack.pop() as Any, stack.items)
    print((5 +? 3) as Any, (3 +? 5) as Any)
    var queue = [4, 5]
    let take = orsTake
    print(orsTake(&queue) as Any, take(&queue) as Any, take(&queue) as Any, queue)
    print(ORSDir.named("s")?.rawValue as Any, ORSDir.named("x") == nil, ORSDir.named("n") == .north)
    var later: [() -> ORSKey?] = []
    for dir in 0..<3 { later.append { room.destination(dir) } }
    room.exits[1] = ORSExit(keywords: [], destination: .nowhere, key: nil)
    print(later.map { $0()?.description ?? "-" })

    // == and != between Optionals of scalars and payload-free enums.
    let flags: [Bool?] = [true, false, nil]
    print(flags.map { $0 == true }, flags.map { $0 != false }, flags.map { $0 == nil })
    print(orsHalf(4) == orsHalf(5), orsHalf(4) == 2, orsHalf(3) == orsHalf(5), orsHalf(4) != orsHalf(8))
    print(room.firstKeyword(0) == "door", room.firstKeyword(2) == room.firstKeyword(9), ORSDir.named("s") != ORSDir.named("n"))
    let nan: Double? = .nan
    print(nan == nan, ORSDerived().score() == 2.5, ORSBase().score() == nil)

    // Constrained extensions, associated-type witnesses, a member named ptr, generic function values.
    print(ORSIntStore(n: 1).firstInt() as Any, ORSIntStore(n: 0).firstInt() as Any)
    let fromStore: Int? = ORSIntStore(n: 2).firstInt()
    print(fromStore as Any, ORSOptionalMaker().make() as Any)
    print(orsHandlePtr(ORSHandle()) as Any, orsHandlePtr(ORSHandle(raw: 0)) as Any)
    print([1, 22, 333].map(orsGenericCount), ["ab"].compactMap(orsGenericCount))

    // inout of a returned optional, and writing through an optional chain on it.
    var maybe = orsHalf(6)
    func bump(_ v: inout Int?) { v = (v ?? 0) + 100 }
    bump(&maybe)
    print(maybe as Any)
    var exitMaybe = room.exit(2)
    exitMaybe?.key = 5
    exitMaybe?.keywords += ["hatch"]
    print(exitMaybe?.key as Any, exitMaybe?.keywords as Any, room.exit(2)?.key as Any)
}
