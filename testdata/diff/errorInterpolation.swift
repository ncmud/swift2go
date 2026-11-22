// An `any Error` interpolates as the value it holds, and a `#line` default takes the caller's line.

struct InterpolatedFailure: Error, CustomStringConvertible {
    let problem: String
    var description: String { problem }
}

struct PlainFailure: Error {
    let code: Int
}

func interpolationLog(_ message: String, loud: Bool = false, line: Int = #line) {
    print("\(line): \(message)\(loud ? "!" : "")")
}

func interpolationFail(_ which: Int) throws {
    if which == 0 { throw InterpolatedFailure(problem: "the tuning is out of range") }
    throw PlainFailure(code: which)
}

public func errorInterpolation() {
    for which in 0..<2 {
        do {
            try interpolationFail(which)
        } catch {
            interpolationLog("failed: \(error)", loud: true)
            print(String(describing: error))
        }
    }
    let stored: any Error = InterpolatedFailure(problem: "stored")
    interpolationLog("\(stored) and \(stored)")
}
