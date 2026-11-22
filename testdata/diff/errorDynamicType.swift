struct DynBoom: Error {}

final class DynFault: Error {
    let code: Int
    init(code: Int) { self.code = code }
}

enum DynFailure: Error {
    case broken
    case worn(Int)
}

func dynThrow(_ n: Int) throws {
    switch n {
    case 0: throw DynBoom()
    case 1: throw DynFault(code: 7)
    case 2: throw DynFailure.broken
    default: throw DynFailure.worn(n)
    }
}

public func errorDynamicType() {
    let e: any Error = DynBoom()
    print("\(type(of: e))")
    print(type(of: e))
    for n in 0..<4 {
        do {
            try dynThrow(n)
        } catch {
            print("caught \(type(of: error))")
            print(String(describing: type(of: error)))
        }
    }
    let errors: [any Error] = [DynBoom(), DynFault(code: 1), DynFailure.worn(2)]
    print(errors.map { "\(type(of: $0))" }.joined(separator: ","))
}
