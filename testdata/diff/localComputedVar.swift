public func localComputedVar() {
    var base = [1, 2, 3, 4]
    var evens: [Int] { base.filter { $0 % 2 == 0 } }
    print(evens)
    base.append(6)
    print(evens.count, evens)
    var label: String {
        let n = evens.count
        return n == 1 ? "one even" : "\(n) evens"
    }
    print(label)
    if base.count > 2 {
        var evens: Int { base.count }
        print("inner", evens)
    }
    print("outer", evens)
    let doubled = { (k: Int) in evens.map { $0 * k } }
    print(doubled(3))
    var loads = 0
    lazy var cached: Int = { loads += 1; return base.count }()
    print(cached, cached, loads)
}
