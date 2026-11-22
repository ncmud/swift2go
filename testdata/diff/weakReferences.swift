// A weak stored property doesn't keep its object alive and reads nil once the object is freed.
// Go frees it at a collection, so the case allocates enough to run many collections before it reads
// the reference once. It doesn't poll: reading a weak pointer while the collector is marking keeps
// the object alive for that cycle. Each object is made and dropped in a function of its own, so no
// temporary in the caller's frame keeps it reachable.

protocol WRDelegate: AnyObject {
    var tag: String { get }
}

class WRNode: WRDelegate {
    let name: String
    weak var parent: WRNode?
    var children: [WRNode] = []
    init(name: String) { self.name = name }
    var tag: String { "node " + name }
}

final class WRLeaf: WRNode {}

struct WRHolder {
    weak var delegate: (any WRDelegate)?
    weak var node: WRNode?
}

final class WRCache {
    weak var last: WRNode? = nil
}

final class WRWorld {
    let root = WRNode(name: "root")
    let holder = WRHolderBox()
    let cache = WRCache()
}

final class WRHolderBox {
    var value = WRHolder()
}

@inline(never)
func wrAttach(_ w: WRWorld) -> String {
    let child = WRLeaf(name: "child")
    child.parent = w.root
    w.root.children.append(child)
    let temp = WRNode(name: "temp")
    w.holder.value.delegate = temp
    w.holder.value.node = child
    w.cache.last = child
    let h = w.holder.value
    return "\(child.parent?.name ?? "nil") \(child.parent === w.root) \(h.delegate?.tag ?? "nil") \(h.node?.name ?? "nil") \(h.node is WRLeaf)"
}

@inline(never)
func wrDelegateGone(_ w: WRWorld) -> Bool { w.holder.value.delegate == nil }

@inline(never)
func wrChildGone(_ w: WRWorld) -> Bool { w.cache.last == nil && w.holder.value.node == nil }

@inline(never)
func wrNodeName(_ w: WRWorld) -> String { w.holder.value.node?.name ?? "nil" }

@inline(never)
func wrChurn() -> Int {
    var total = 0
    for round in 0..<1000 {
        let junk = [Int](repeating: round, count: 50_000)
        total += junk.count
    }
    return total
}

public func weakReferences() {
    var out: [String] = []
    let w = WRWorld()
    out.append("cache starts nil: \(w.cache.last == nil)")
    out.append(wrAttach(w))
    _ = wrChurn()
    out.append("delegate gone: \(wrDelegateGone(w))")
    out.append("node kept: \(wrNodeName(w))")
    w.root.children.removeAll()
    _ = wrChurn()
    out.append("child gone: \(wrChildGone(w))")
    print(out.joined(separator: "\n"))
}
