import Dispatch

func tgfaSquares(_ n: Int) async -> [Int] {
    await withTaskGroup(of: (Int, Int).self) { group in
        for i in 0..<n {
            group.addTask { (i, i * i) }
        }
        var out = Array(repeating: 0, count: n)
        for await (i, sq) in group { out[i] = sq }
        return out
    }
}

nonisolated(unsafe) var tgfaResult: [Int] = []

public func taskGroupForAwait() {
    let done = DispatchSemaphore(value: 0)
    Task {
        tgfaResult = await tgfaSquares(5)
        done.signal()
    }
    done.wait()
    print(tgfaResult)
}
