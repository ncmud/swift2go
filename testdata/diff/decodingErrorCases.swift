struct DECKey: CodingKey {
    var stringValue: String
    var intValue: Int? { nil }
    init(_ s: String) { stringValue = s }
    init?(stringValue: String) { self.stringValue = stringValue }
    init?(intValue: Int) { nil }
}

func decDescribe(_ error: any Error) -> String {
    guard let decodingError = error as? DecodingError else { return "other" }
    let context: DecodingError.Context
    switch decodingError {
    case .typeMismatch(_, let ctx), .valueNotFound(_, let ctx), .keyNotFound(_, let ctx), .dataCorrupted(let ctx):
        context = ctx
    @unknown default:
        return "unknown"
    }
    let path = context.codingPath.map { $0.intValue.map(String.init) ?? $0.stringValue }.joined(separator: "/")
    return path.isEmpty ? context.debugDescription : "\(path): \(context.debugDescription)"
}

struct DECOther: Error {}

public func decodingErrorCases() {
    let errors: [any Error] = [
        DecodingError.dataCorrupted(.init(codingPath: [], debugDescription: "bad bytes")),
        DecodingError.dataCorrupted(.init(codingPath: [DECKey("commands"), DECKey("maxInWorld")], debugDescription: "too many")),
        DECOther(),
    ]
    for e in errors {
        print(decDescribe(e))
    }
}
