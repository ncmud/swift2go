// A weak stored property doesn't keep its object alive and reads nil once the object is freed.
// Go frees it at a collection, so the case allocates until the reference clears or a bound is hit.

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

func wrChurn(_ isGone: () -> Bool) -> Bool {
    var rounds = 0
    while !isGone() && rounds < 2000 {
        let junk = [Int](repeating: rounds, count: 50_000)
        rounds += junk.count > 0 ? 1 : 0
    }
    return isGone()
}

public func weakReferences() {
    var out: [String] = []
    let root = WRNode(name: "root")
    var child: WRNode? = WRLeaf(name: "child")
    child!.parent = root
    root.children.append(child!)
    out.append("\(child!.parent?.name ?? "nil") \(child!.parent === root)")

    var holder = WRHolder()
    var temp: WRNode? = WRNode(name: "temp")
    holder.delegate = temp
    holder.node = child
    out.append("\(holder.delegate?.tag ?? "nil") \(holder.node?.name ?? "nil") \(holder.node is WRLeaf)")
    temp = nil
    out.append("delegate gone: \(wrChurn { holder.delegate == nil })")
    out.append("node kept: \(holder.node?.name ?? "nil")")

    let cache = WRCache()
    out.append("cache starts nil: \(cache.last == nil)")
    cache.last = child
    root.children.removeAll()
    child = nil
    out.append("child gone: \(wrChurn { cache.last == nil && holder.node == nil })")
    print(out.joined(separator: "\n"))
}
