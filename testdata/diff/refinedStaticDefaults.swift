protocol RSDKinded {
    static var kind: String { get }
    static func greet() -> String
    var label: String { get }
}
extension RSDKinded {
    static var kind: String { "base" }
    static func greet() -> String { "hello from base" }
    var label: String { "base label" }
}
protocol RSDRefined: RSDKinded {}
extension RSDRefined {
    static var kind: String { "refined" }
    static func greet() -> String { "hello from refined" }
    var label: String { "refined label" }
}
protocol RSDMoreRefined: RSDRefined {}
extension RSDMoreRefined {
    static var kind: String { "more refined" }
}
protocol RSDMarker {}
struct RSDPlain: RSDKinded {}
struct RSDDirect: RSDRefined {}
struct RSDIndirect: RSDMoreRefined {}
struct RSDOwn: RSDRefined { static let kind = "own" }
struct RSDBoth: RSDKinded, RSDMarker, RSDRefined {}
protocol RSDTagged { static var tag: String { get } }
extension RSDTagged { static var tag: String { "base tag" } }
protocol RSDRefinedTagged: RSDTagged {}
extension RSDRefinedTagged { static var tag: String { "refined tag" } }
struct RSDSplit {}
extension RSDSplit: RSDTagged {}
extension RSDSplit: RSDRefinedTagged {}

func rsdKindOf<T: RSDKinded>(_ t: T.Type) -> String { t.kind }

func rsdTagOf<T: RSDTagged>(_ t: T.Type) -> String { t.tag }

public func refinedStaticDefaults() {
    let all: [any RSDKinded.Type] = [RSDPlain.self, RSDDirect.self, RSDIndirect.self, RSDOwn.self, RSDBoth.self]
    for t in all {
        print(t.kind, "|", t.greet())
    }
    print(rsdKindOf(RSDPlain.self), rsdKindOf(RSDDirect.self), rsdKindOf(RSDIndirect.self), rsdKindOf(RSDOwn.self), rsdKindOf(RSDBoth.self))
    let values: [any RSDKinded] = [RSDPlain(), RSDDirect(), RSDIndirect(), RSDOwn(), RSDBoth()]
    print(values.map(\.label))
    let tagged: [any RSDTagged.Type] = [RSDSplit.self]
    print(tagged.map { $0.tag }, rsdTagOf(RSDSplit.self))
}
