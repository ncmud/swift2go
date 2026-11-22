protocol GSMContext {}
struct GSMBasic: GSMContext {}

struct GSMHolder<Context: GSMContext> {
    static var tags: Set<String> { ["t", "u"] }
    static var prefix: String? { "<" }
    static func label(_ s: String) -> String { (prefix ?? "") + s + ">" }
    func render(_ s: String) -> String { Self.label(s) + String(Self.tags.count) }
    func tagged(_ s: String) -> Bool { Self.tags.contains(s) }
}

public func genericStaticMembers() {
    let holder = GSMHolder<GSMBasic>()
    print(holder.render("x"), holder.tagged("t"), holder.tagged("v"))
    print(GSMHolder<GSMBasic>.label("y"), GSMHolder<GSMBasic>.tags.sorted())
}
