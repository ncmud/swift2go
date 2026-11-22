import Dispatch

final class CVRQueue: @unchecked Sendable {
    var log: [String] = []
    func enqueue<T>(_ work: @escaping () throws -> T) async throws -> T {
        try await withCheckedThrowingContinuation { continuation in
            do {
                continuation.resume(returning: try work())
            } catch {
                continuation.resume(throwing: error)
            }
        }
    }
}

nonisolated(unsafe) var cvrLines: [String] = []

public func continuationVoidResult() {
    let done = DispatchSemaphore(value: 0)
    let queue = CVRQueue()
    Task {
        do {
            try await queue.enqueue { queue.log.append("void work") }
            let n: Int = try await queue.enqueue { 42 }
            let s: String? = try await queue.enqueue { nil }
            cvrLines.append("\(queue.log) \(n) \(String(describing: s))")
        } catch {
            cvrLines.append("error")
        }
        done.signal()
    }
    done.wait()
    for line in cvrLines { print(line) }
}
