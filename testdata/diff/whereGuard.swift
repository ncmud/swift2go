public func whereGuard() {
    for scalar in "ab-é.c".unicodeScalars {
        switch scalar {
        case "a"..."z": print("letter", String(scalar))
        case "'", ".": continue
        case _ where !scalar.isASCII: print("nonascii")
        default: print("other", String(scalar))
        }
    }
}
