final class IDRegistry { nonisolated(unsafe) static var all: [IDNode] = [] }
enum IDLog { nonisolated(unsafe) static var events: [String] = [] }

class IDNode {
    var id: Int
    var log = ""
    let created: String
    init(id: Int) {
        self.id = id
        created = "node"
        IDRegistry.all.append(self)
        announce()
        IDLog.events.append("IDNode.init sees \(describe())")
    }
    func announce() { log += "node" }
    func describe() -> String { "IDNode(\(id))" }
}

class IDLeaf: IDNode {
    var depth: Int
    var tagged = "leaf-default"
    init() {
        depth = 3
        super.init(id: 7)
        announce()
        IDLog.events.append("IDLeaf.init after super sees \(describe())")
    }
    override func announce() { log += "leaf\(depth);" }
    override func describe() -> String { "IDLeaf(\(id), \(depth), \(tagged))" }
}

class IDTwig: IDLeaf {
    var twig = "t"
    override init() {
        super.init()
        twig = "T"
    }
    override func announce() { log += "twig\(twig);" }
}

class IDHolder {
    var items: [IDHolder] = []
    weak var parent: IDHolder?
    let name: String
    init(name: String, parent: IDHolder?) {
        self.name = name
        self.parent = parent
        parent?.items.append(self)
    }
}
class IDSubHolder: IDHolder {
    init(child name: String, of parent: IDHolder) {
        super.init(name: name + "!", parent: parent)
    }
}

class IDFallible {
    var v: Int
    init?(v: Int) {
        if v < 0 { return nil }
        self.v = v
    }
}
class IDFallibleSub: IDFallible {
    var w: String
    init?(w: String) {
        self.w = w
        super.init(v: w.count - 2)
    }
}

public func classInitIdentity() {
    var out: [String] = []
    let l = IDLeaf()
    l.id = 8
    out.append("\(IDRegistry.all[0] === l) \(IDRegistry.all[0].id) \(l.log)")
    let t = IDTwig()
    out.append("\(IDRegistry.all[1] === t) \(t.log) \(t.twig)")
    out.append(IDRegistry.all[1].describe())
    let root = IDHolder(name: "root", parent: nil)
    let kid = IDSubHolder(child: "kid", of: root)
    out.append("\(root.items.count) \(root.items[0] === kid) \(root.items[0].name) \(kid.parent === root)")
    kid.items.append(root)
    out.append("\(root.items[0].items.count)")
    out.append(String(describing: IDFallibleSub(w: "a")?.w))
    let fs = IDFallibleSub(w: "abcd")
    out.append("\(fs?.v ?? -1) \(fs?.w ?? "")")
    out.append(contentsOf: IDLog.events)
    print(out.joined(separator: "\n"))
}
