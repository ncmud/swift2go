protocol CCNamed: AnyObject {
    var tag: String { get }
}

class CCRoot {
    var id: Int
    init(id: Int) { self.id = id }
    var asMid: CCMid? { self as? CCMid }
    var asLeaf: CCLeaf? { self as? CCLeaf }
    var asOther: CCOther? { self as? CCOther }
}

class CCMid: CCRoot, CCNamed {
    var tag: String { "mid\(id)" }
}

final class CCLeaf: CCMid {
    override var tag: String { "leaf\(id)" }
}

final class CCOther: CCRoot {}

func ccDescribe(_ r: CCRoot?) -> String {
    let mid = r?.asMid.map { "\($0.id)" } ?? "-"
    let leaf = r?.asLeaf.map { "\($0.id)" } ?? "-"
    let other = r?.asOther.map { "\($0.id)" } ?? "-"
    let named = (r as? CCNamed)?.tag ?? "-"
    return "mid=\(mid) leaf=\(leaf) other=\(other) named=\(named) isMid=\(r is CCMid) isLeaf=\(r is CCLeaf)"
}

public func classCasts() {
    let all: [CCRoot?] = [CCRoot(id: 1), CCMid(id: 2), CCLeaf(id: 3), CCOther(id: 4), nil]
    // Twice, so the second pass casts each class again after the first has seen it.
    for _ in 0..<2 {
        for r in all {
            print(ccDescribe(r))
        }
    }
    let anys: [Any] = [CCLeaf(id: 5), CCOther(id: 6), 7, "eight"]
    for a in anys {
        let root = (a as? CCRoot).map { "\($0.id)" } ?? "-"
        let mid = (a as? CCMid).map { "\($0.id)" } ?? "-"
        print("root=\(root) mid=\(mid) int=\(a as? Int ?? -1)")
    }
    let leafAsMid: CCMid = CCLeaf(id: 9)
    print((leafAsMid as! CCLeaf).tag, (leafAsMid as CCRoot).asLeaf === leafAsMid)
}
