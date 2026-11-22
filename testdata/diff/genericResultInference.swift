import Foundation

struct GRIRejected: Error { let why: String }

enum GRIOutcome<Rejection: Error> {
    case accepted(Int)
    case rejected(Rejection)
}

final class GRIStore {
    var log: [String] = []

    func write<T>(_ body: (GRIStore) throws -> T) rethrows -> T { try body(self) }

    func refresh<Rejection>(_ authorize: (Int) -> Result<Int, Rejection>) -> GRIOutcome<Rejection> {
        switch authorize(7) {
        case .success(let n): return .accepted(n)
        case .failure(let r): return .rejected(r)
        }
    }

    private static func decode<T: Decodable>(_ json: String) throws -> T {
        try JSONDecoder().decode(T.self, from: Data(json.utf8))
    }

    func decoded() throws -> [Int] {
        let numbers: [Int] = try Self.decode("[1,2]")
        return numbers
    }
}

nonisolated(unsafe) var griCancelled = false

func griOffload<T: Sendable>(_ operation: @escaping @Sendable () -> T) async -> T {
    let task = Task.detached(priority: .userInitiated) { operation() }
    return await withTaskCancellationHandler {
        await task.value
    } onCancel: {
        task.cancel()
    }
}

func griVoidHandler() async {
    await withTaskCancellationHandler {
        griCancelled = false
    } onCancel: {
        griCancelled = true
    }
}

public func genericResultInference() {
    let store = GRIStore()
    store.write { s in s.log.append("wrote") }
    let n = store.write { s in s.log.count }
    print(store.log, n)
    switch store.refresh({ n -> Result<Int, GRIRejected> in n > 5 ? .failure(GRIRejected(why: "big")) : .success(n) }) {
    case .accepted(let v): print("accepted", v)
    case .rejected(let r): print("rejected", r.why)
    }
    print((try? store.decoded()) ?? [])
    let done = DispatchSemaphore(value: 0)
    nonisolated(unsafe) var result = 0
    Task {
        result = await griOffload { 6 * 7 }
        await griVoidHandler()
        done.signal()
    }
    done.wait()
    print(result, griCancelled)
}
