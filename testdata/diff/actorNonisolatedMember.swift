import Dispatch

private actor AnmWorker {
    nonisolated let name: String
    private var jobs = 0

    init(name: String) { self.name = name }

    nonisolated var title: String { "worker " + name }

    nonisolated func describe() -> String { title + " is nonisolated" }

    func block(entered: DispatchSemaphore, release: DispatchSemaphore) {
        jobs += 1
        entered.signal()
        release.wait()
    }

    func jobCount() -> Int { jobs }
}

public func actorNonisolatedMember() {
    let worker = AnmWorker(name: "w1")
    let entered = DispatchSemaphore(value: 0)
    let release = DispatchSemaphore(value: 0)
    let done = DispatchSemaphore(value: 0)
    Task.detached {
        await worker.block(entered: entered, release: release)
        done.signal()
    }
    entered.wait()
    print(worker.name)
    print(worker.describe())
    release.signal()
    done.wait()
    Task.detached {
        print("jobs", await worker.jobCount())
        done.signal()
    }
    done.wait()
}
