import Testing

public func checkEqual<T: Equatable>(
    _ lhs: T, _ rhs: T, _ comment: Comment? = nil,
    fileID: String = #fileID, filePath: String = #filePath, line: Int = #line, column: Int = #column
) {
    if lhs != rhs {
        var text = "checkEqual failed: \(lhs) != \(rhs)"
        if let comment { text += " - \(comment.rawValue)" }
        Issue.record(
            Comment(rawValue: text),
            sourceLocation: SourceLocation(fileID: fileID, filePath: filePath, line: line, column: column)
        )
    }
}

public func checkLess<T: Comparable>(_ lhs: T, _ rhs: T, sourceLocation: SourceLocation = #_sourceLocation) {
    if !(lhs < rhs) || lhs >= rhs || rhs <= lhs || !(rhs > lhs) {
        Issue.record("checkLess failed: \(lhs) < \(rhs)", sourceLocation: sourceLocation)
    }
}

public struct CheckFailed: Error {}

@discardableResult
public func checkSome<T>(_ value: T?) throws -> T {
    guard let value else {
        Issue.record("checkSome failed: nil")
        throw CheckFailed()
    }
    return value
}

public func checkNil<T>(_ value: T?) {
    if value != nil { Issue.record("checkNil failed") }
}
