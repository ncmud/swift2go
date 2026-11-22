enum IILog { nonisolated(unsafe) static var trace: [String] = [] }

class IIBase {
    var tag: String
    var count = 1
    init(tag: String) { self.tag = tag; IILog.trace.append("IIBase.init(tag:) \(kind())") }
    convenience init() { self.init(tag: "default"); IILog.trace.append("IIBase.init()") }
    convenience init?(code: Int) {
        if code < 0 { return nil }
        self.init(tag: "code\(code)")
    }
    func kind() -> String { "base" }
    func show() -> String { "\(tag):\(kind()):\(count)" }
}

class IIDerived: IIBase {
    var extra = "x"
    override func kind() -> String { "derived+\(extra)" }
}

class IIOverriding: IIBase {
    var note: String
    override init(tag: String) {
        note = "n-" + tag
        super.init(tag: tag.uppercased())
        IILog.trace.append("IIOverriding.init(tag:)")
    }
    override func kind() -> String { "overriding(\(note))" }
}

class IILeaf: IIDerived {
    var leafy = 3
    override func kind() -> String { "leaf\(leafy)+" + super.kind() }
}

class IINoInit { var a = 5; var b = "b"; func f() -> String { "\(a)\(b)" } }
class IINoInitSub: IINoInit { var c = 7; override func f() -> String { super.f() + "\(c)" } }

public func classInheritedInit() {
    var out: [String] = []
    out.append(IIDerived(tag: "t").show())
    out.append(IIDerived().show())
    out.append(String(describing: IIDerived(code: -1)?.show()))
    out.append(IIDerived(code: 4)!.show())
    out.append(IIOverriding(tag: "o").show())
    out.append(IIOverriding().show())
    out.append(IIOverriding(code: 9)!.show())
    out.append(IILeaf(tag: "l").show())
    out.append(IILeaf().show())
    let d: IIBase = IILeaf(code: 2)!
    out.append(d.show())
    out.append(IINoInitSub().f())
    out.append(IINoInit().f())
    out.append(contentsOf: IILog.trace)
    print(out.joined(separator: "\n"))
}
