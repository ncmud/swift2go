import Foundation

private actor AlHook {
    var stops = 0
    func stop() { stops += 1 }
    func count() -> Int { stops }
}

private func alRun() async -> Int {
    let hook = AlHook()
    async let first: Void = hook.stop()
    async let second: Void = hook.stop()
    async let n: Int = hook.count()
    _ = await (first, second)
    _ = await n
    return await hook.count()
}

public func asyncLetValues() {
    let done = DispatchSemaphore(value: 0)
    Task {
        print(await alRun())
        done.signal()
    }
    done.wait()
}
