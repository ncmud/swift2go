#if canImport(Darwin)
import Darwin
#else
import Glibc
#endif

public func stdioLines() {
    print("before flush")
    print(fflush(nil))
    print(readLine() == nil)
    print(readLine(strippingNewline: false) == nil)
}
