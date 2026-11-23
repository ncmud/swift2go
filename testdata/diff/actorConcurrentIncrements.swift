import Dispatch

private actor AciCounter {
    private(set) var count = 0
    private var history: [Int] = []

    func increment() {
        let seen = count
        history.append(seen)
        count = seen + 1
    }

    func incrementLater() -> Task<Void, Never> {
        Task { self.increment() }
    }

    var distinctSeen: Int { Set(history).count }
}

public func actorConcurrentIncrements() {
    let counter = AciCounter()
    let done = DispatchSemaphore(value: 0)
    let workers = 8
    for _ in 0..<workers {
        Task.detached {
            for _ in 0..<500 {
                await counter.increment()
                await counter.incrementLater().value
            }
            done.signal()
        }
    }
    for _ in 0..<workers { done.wait() }
    Task.detached {
        print("count", await counter.count)
        print("distinct values seen", await counter.distinctSeen)
        done.signal()
    }
    done.wait()
}
