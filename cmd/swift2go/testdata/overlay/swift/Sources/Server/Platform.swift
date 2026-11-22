import Foundation

/// Names the host the server runs on, by way of a platform call.
public func hostLabel() -> String {
    "swift pid \(getpid())"
}
