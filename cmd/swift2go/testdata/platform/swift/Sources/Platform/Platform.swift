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

/// Writes a file to the temporary directory, backdates it, and reads back its modification time and size,
/// which Foundation on Linux names as Swift statics and Darwin's imports from Objective-C.
public func fileAttributes() -> String {
    let path = NSTemporaryDirectory() + "platform-\(getpid()).txt"
    defer { try? FileManager.default.removeItem(atPath: path) }
    guard FileManager.default.createFile(atPath: path, contents: Data("hello".utf8)),
          (try? FileManager.default.setAttributes([.modificationDate: Date(timeIntervalSince1970: 1_000_000)], ofItemAtPath: path)) != nil,
          let attributes = try? FileManager.default.attributesOfItem(atPath: path),
          let modified = attributes[.modificationDate] as? Date,
          let size = attributes[.size] as? NSNumber else { return "none" }
    return "\(Int(modified.timeIntervalSince1970)) \(size.intValue)"
}
