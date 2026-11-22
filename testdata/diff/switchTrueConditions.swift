func stcMatched(_ owner: String?, _ vnum: Int?, _ tag: String?) -> String {
    let matchedBy: String?
    switch true {
    case owner != nil: matchedBy = "owner"
    case vnum != nil && vnum! > 0: matchedBy = "vnum"
    case tag?.isEmpty == false: matchedBy = "tag"
    default: matchedBy = nil
    }
    return matchedBy ?? "none"
}

public func switchTrueConditions() {
    print(stcMatched("mob", nil, nil), stcMatched(nil, 3, nil), stcMatched(nil, 0, "x"), stcMatched(nil, nil, ""))
}
