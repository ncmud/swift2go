import Foundation

private func tgoeFirst(_ wanted: Int) async -> String {
    enum Outcome: Sendable {
        case found(Int, String)
        case failed(String)
    }
    let outcome = await withTaskGroup(of: Outcome?.self) { group in
        group.addTask {
            if wanted > 0 { return .found(wanted, "n\(wanted)") }
            if wanted < 0 { return .failed("negative") }
            return nil
        }
        defer { group.cancelAll() }
        return await group.next() ?? nil
    }
    switch outcome {
    case .found(let n, let label):
        return "found \(n) \(label)"
    case .failed(let why):
        return "failed \(why)"
    case nil:
        return "nothing"
    }
}

public func taskGroupOptionalEnum() {
    let done = DispatchSemaphore(value: 0)
    Task {
        print(await tgoeFirst(3))
        print(await tgoeFirst(-1))
        print(await tgoeFirst(0))
        done.signal()
    }
    done.wait()
}
