struct MirrorChildrenItem {
    var name: String
    var weight: Int
    var tags: [String]
}

public func mirrorChildren() {
    let a = MirrorChildrenItem(name: "sword", weight: 3, tags: ["sharp"])
    let b = MirrorChildrenItem(name: "sword", weight: 4, tags: ["sharp"])
    for child in Mirror(reflecting: a).children {
        print(child.label ?? "?", child.value)
    }
    let same = zip(Mirror(reflecting: a).children, Mirror(reflecting: b).children).compactMap { mine, theirs in
        "\(mine.value)" == "\(theirs.value)" ? mine.label : nil
    }
    print(same)
}
