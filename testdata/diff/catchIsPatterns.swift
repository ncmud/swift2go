struct CIStructError: Error { let code: Int }
enum CIEnumError: Error { case missing, failed(String) }
protocol CIKindedError: Error { var kind: String { get } }
struct CIKinded: CIKindedError { let kind: String }
final class CIClassError: Error { let detail: String; init(_ d: String) { detail = d } }
final class CISubclassError: Error {}

func ciThrow(_ n: Int) throws {
    switch n {
    case 0: throw CIStructError(code: 4)
    case 1: throw CIEnumError.missing
    case 2: throw CIEnumError.failed("disk")
    case 3: throw CIKinded(kind: "proto")
    case 4: throw CIClassError("cls")
    case 5: throw CISubclassError()
    default: return
    }
}

func ciCatchIs(_ n: Int) -> String {
    do {
        try ciThrow(n)
        return "none"
    } catch is CIStructError {
        return "struct"
    } catch is CIKindedError {
        return "kinded"
    } catch is CIClassError {
        return "class"
    } catch {
        return "other"
    }
}

func ciCatchLetAs(_ n: Int) -> String {
    do {
        try ciThrow(n)
        return "none"
    } catch let e as CIStructError where e.code > 10 {
        return "big struct \(e.code)"
    } catch let e as CIStructError {
        return "struct \(e.code)"
    } catch let e as CIEnumError {
        return "enum \(e)"
    } catch let e as CIKindedError {
        return "kinded \(e.kind)"
    } catch let e as CIClassError {
        return "class \(e.detail)"
    } catch {
        return "other"
    }
}

func ciIfCase(_ n: Int) -> String {
    do {
        try ciThrow(n)
        return "none"
    } catch {
        if case CIEnumError.missing = error {
            return "if-case missing"
        } else if case CIEnumError.failed(let why) = error {
            return "if-case failed \(why)"
        } else if error is CIKindedError {
            return "is kinded"
        } else if let s = error as? CIStructError {
            return "as? struct \(s.code)"
        }
        return "if-case other"
    }
}

func ciRethrow(_ n: Int) throws -> String {
    do {
        try ciThrow(n)
    } catch is CIEnumError {
        return "handled enum"
    }
    return "none"
}

public func catchIsPatterns() {
    var out: [String] = []
    for n in 0...6 {
        out.append("\(n): \(ciCatchIs(n)) | \(ciCatchLetAs(n)) | \(ciIfCase(n))")
        do {
            out.append("   rethrow: \(try ciRethrow(n))")
        } catch {
            out.append("   rethrow escaped: struct=\(error is CIStructError) kinded=\(error is CIKindedError)")
        }
    }
    print(out.joined(separator: "\n"))
}
