import Dispatch

private actor ArGate {
    var log: [String] = []
    private var waiting: CheckedContinuation<Void, Never>?

    func slow(_ suspended: DispatchSemaphore) async {
        log.append("slow start")
        await withCheckedContinuation { continuation in
            waiting = continuation
            suspended.signal()
        }
        log.append("slow end")
    }

    func fast() {
        log.append("fast")
        waiting?.resume()
        waiting = nil
    }
}

public func actorReentrancy() {
    let gate = ArGate()
    let suspended = DispatchSemaphore(value: 0)
    let done = DispatchSemaphore(value: 0)
    Task.detached {
        await gate.slow(suspended)
        done.signal()
    }
    suspended.wait()
    Task.detached {
        await gate.fast()
        done.signal()
    }
    done.wait()
    done.wait()
    Task.detached {
        for line in await gate.log { print(line) }
        done.signal()
    }
    done.wait()
}
