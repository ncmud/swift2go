enum OSLColor { case red, green }

func oslWord(_ s: String?) -> String {
    switch s {
    case nil: return "none"
    case "active": return "A"
    case "timers", "t": return "T"
    default: return "other(\(s!))"
    }
}

func oslNumber(_ n: Int?) -> String {
    switch n {
    case 0: return "zero"
    case (1...9)?: return "digit"
    case .some(let v) where v < 0: return "negative"
    case .none: return "none"
    default: return "big"
    }
}

func oslColor(_ c: OSLColor?) -> String {
    switch c {
    case .red: return "red"
    case .green?: return "green"
    case nil: return "nil"
    }
}

public func optionalSwitchLiterals() {
    let words: [String?] = [nil, "active", "timers", "t", "x"]
    print(words.map(oslWord))
    let lowered = String("ACTIVE".lowercased())
    print(oslWord(Optional(lowered)))
    print([0, 5, -3, nil, 42].map(oslNumber))
    print([OSLColor.red, .green, nil].map(oslColor))
}
