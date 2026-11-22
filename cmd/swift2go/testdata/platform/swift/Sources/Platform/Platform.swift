import Foundation
#if canImport(Darwin)
import Darwin
#elseif canImport(Glibc)
import Glibc
#elseif canImport(Musl)
import Musl
#endif

/// Names the platform and C library the package was compiled for.
public func platformName() -> String {
    #if os(Linux)
    let os = "linux"
    #elseif os(macOS)
    let os = "macos"
    #else
    let os = "other"
    #endif
    #if canImport(Musl)
    return os + "/musl"
    #elseif canImport(Glibc)
    return os + "/glibc"
    #else
    return os + "/darwin"
    #endif
}

/// The message of the day shipped in the target's resource bundle.
public func motd() -> String {
    guard let url = Bundle.module.url(forResource: "motd", withExtension: "txt"),
          let text = try? String(contentsOf: url, encoding: .utf8) else { return "none" }
    return text.trimmingCharacters(in: .whitespacesAndNewlines)
}
