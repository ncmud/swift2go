// A case with several patterns binds its variables from whichever pattern matched.

enum SharedBindingPlace {
    case nowhere
    case room(Int)
    case carried(by: Int64)
    case worn(by: Int64, slot: Int)
}

func sharedBindingHolder(_ place: SharedBindingPlace) -> String {
    switch place {
    case .nowhere:
        return "nowhere"
    case .room(let vnum):
        return "room \(vnum)"
    case .carried(let id), .worn(let id, _):
        return "held by \(id)"
    }
}

func sharedBindingPair(_ pair: (Int, String)) -> String {
    switch pair {
    case (0, let s) where s.isEmpty, (3, let s):
        return "guarded \(s)"
    case (1, let s), (2, let s):
        return "small \(s)"
    default:
        return "other"
    }
}

public func switchSharedBindings() {
    print(sharedBindingHolder(.carried(by: 63)), sharedBindingHolder(.worn(by: 64, slot: 3)))
    print(sharedBindingHolder(.room(206)), sharedBindingHolder(.nowhere))
    print(sharedBindingPair((1, "one")), sharedBindingPair((2, "two")), sharedBindingPair((0, "zero")), sharedBindingPair((5, "five")), sharedBindingPair((3, "three")), sharedBindingPair((0, "")))
}
