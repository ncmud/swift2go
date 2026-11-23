struct ECItem {
    var name: String
    var tags: [String]
}

final class ECBag {
    var items: [ECItem] = [ECItem(name: "a", tags: ["x"]), ECItem(name: "b", tags: [])]
}

public func enumeratedChains() {
    let words = ["zero", "one", "two", "three"]
    print(words.enumerated().map { "\($0.offset):\($0.element)" })
    print(words.enumerated().compactMap { $0.offset % 2 == 0 ? $0.element.uppercased() : nil })
    print(words.enumerated().flatMap { pair in Array(repeating: pair.element.first!, count: pair.offset) })
    print(words.enumerated().allSatisfy { $0.element.count >= 3 || $0.offset == 1 })
    print(words.enumerated().allSatisfy { $0.offset < 2 })
    let empty: [Int] = []
    print(empty.enumerated().map { $0.offset }, empty.enumerated().allSatisfy { _ in false })

    // A closure that changes the array still sees the elements the array had when enumerated() ran.
    var grow = [1, 2, 3]
    let seen = grow.enumerated().map { pair -> Int in
        grow.append(pair.element * 10)
        if pair.offset == 0 { grow[1] = 99 }
        return pair.element
    }
    print(seen, grow)

    let bag = ECBag()
    let named = bag.items.enumerated().map { pair -> String in
        bag.items[pair.offset].tags.append("seen")
        bag.items.append(ECItem(name: "late", tags: []))
        return "\(pair.offset)=\(pair.element.name)\(pair.element.tags)"
    }
    print(named, bag.items.count, bag.items[0].tags)

    var items = [ECItem(name: "p", tags: ["1"]), ECItem(name: "q", tags: ["2"])]
    let tagged = items.enumerated().map { pair -> ECItem in
        var item = pair.element
        item.tags.append("#\(pair.offset)")
        return item
    }
    items[0].tags.append("orig")
    print(tagged.map(\.tags), items.map(\.tags))
}
