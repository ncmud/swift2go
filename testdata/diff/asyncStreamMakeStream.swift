import Dispatch

nonisolated(unsafe) var asmsSeen: [String] = []

public func asyncStreamMakeStream() {
    let (numbers, feed) = AsyncStream<Int>.makeStream()
    let (gate, open) = AsyncStream<Void>.makeStream()
    feed.yield(1)
    feed.yield(2)
    feed.finish()
    let done = DispatchSemaphore(value: 0)
    Task {
        for await n in numbers { asmsSeen.append("n\(n)") }
        for await _ in gate { asmsSeen.append("opened"); break }
        done.signal()
    }
    open.yield()
    done.wait()
    print(asmsSeen)
}
