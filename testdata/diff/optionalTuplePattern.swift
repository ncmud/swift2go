func optionalTuplePatternRoute(_ method: String, _ path: String) -> String {
    let parts = path.split(separator: "/").map(String.init)
    switch (method, parts.count, parts.first) {
    case ("GET", 2, "users"):
        return "user"
    case ("GET", 3, "guilds") where parts[2] == "roles":
        return "roles"
    case (_, _, nil):
        return "root"
    default:
        return "unexpected \(method) \(path)"
    }
}

public func optionalTuplePattern() {
    print(optionalTuplePatternRoute("GET", "guilds/100/roles"), optionalTuplePatternRoute("GET", "users/1"), optionalTuplePatternRoute("PUT", ""), optionalTuplePatternRoute("GET", "x"))
}
