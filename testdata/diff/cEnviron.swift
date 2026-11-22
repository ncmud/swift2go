#if canImport(Darwin)
import Darwin
#else
import Glibc
#endif

func ceFind(_ prefix: String) -> [String] {
    var found: [String] = []
    var i = 0
    while let entry = environ[i] {
        let s = String(cString: entry)
        if s.hasPrefix(prefix) { found.append(s) }
        i += 1
    }
    return found
}

public func cEnviron() {
    setenv("SWIFT2GO_ENVIRON_A", "one", 1)
    setenv("SWIFT2GO_ENVIRON_B", "two=2", 1)
    print(ceFind("SWIFT2GO_ENVIRON_").sorted())
    unsetenv("SWIFT2GO_ENVIRON_A")
    print(ceFind("SWIFT2GO_ENVIRON_").sorted())
    var count = 0
    while environ[count] != nil { count += 1 }
    print(count > 0)
}
