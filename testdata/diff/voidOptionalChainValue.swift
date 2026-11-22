final class VOCSpeaker {
    var said: [String] = []
    func speak() { said.append("spoke") }
}

final class VOCHolder {
    var speaker: VOCSpeaker?
    init(_ speaker: VOCSpeaker?) { self.speaker = speaker }
}

func vocRelease(_ h: VOCHolder) -> ()? {
    h.speaker?.speak()
}

func vocReleaseExplicit(_ h: VOCHolder) -> Void? {
    return h.speaker?.speak()
}

public func voidOptionalChainValue() {
    let s = VOCSpeaker()
    print(vocRelease(VOCHolder(s)) == nil, s.said)
    print(vocRelease(VOCHolder(nil)) == nil)
    print(vocReleaseExplicit(VOCHolder(s)) == nil, s.said)
    print(vocReleaseExplicit(VOCHolder(nil)) == nil)
    let fn: (VOCHolder) -> ()? = { $0.speaker?.speak() }
    print(fn(VOCHolder(nil)) == nil, fn(VOCHolder(s)) == nil, s.said.count)
    let stored = VOCHolder(nil).speaker?.speak()
    print(stored == nil)
    VOCHolder(s).speaker?.speak()
    print(s.said.count)
}
