import Foundation

private final class TcTally: @unchecked Sendable {
    let lock = NSLock()
    var sum = 0
    var threads: Set<ObjectIdentifier> = []
}

public func threadCurrent() {
    let here = ObjectIdentifier(Thread.current)
    print(here == ObjectIdentifier(Thread.current), Thread.isMainThread)
    let tally = TcTally()
    DispatchQueue.concurrentPerform(iterations: 8) { i in
        tally.lock.lock()
        tally.sum += i
        tally.threads.insert(ObjectIdentifier(Thread.current))
        tally.lock.unlock()
    }
    print(tally.sum, !tally.threads.isEmpty)
    let done = DispatchSemaphore(value: 0)
    Task {
        await unsafeCurrentTaskCancel()
        done.signal()
    }
    done.wait()
}

private func unsafeCurrentTaskCancel() async {
    withUnsafeCurrentTask { print($0 == nil) }
    let task = Task {
        withUnsafeCurrentTask { $0?.cancel() }
        print(Task.isCancelled)
        withUnsafeCurrentTask { print($0?.isCancelled ?? false) }
        return 1
    }
    print(await task.value)
}
