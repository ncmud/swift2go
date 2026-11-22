import Dispatch

enum TaskLocalScope {
    @TaskLocal static var name = "default"
}

struct TaskLocalFailure: Error {}

nonisolated(unsafe) var taskLocalSeen: [String: String] = [:]
let taskLocalSeenLock = DispatchSemaphore(value: 1)

func taskLocalWait(_ semaphore: DispatchSemaphore) { semaphore.wait() }

func taskLocalRecord(_ key: String) {
    let value = TaskLocalScope.name
    taskLocalSeenLock.wait()
    taskLocalSeen[key] = value
    taskLocalSeenLock.signal()
}

public func taskLocalPerTask() {
    print(TaskLocalScope.name)
    TaskLocalScope.$name.withValue("outer") {
        print(TaskLocalScope.name)
        TaskLocalScope.$name.withValue("inner") { print(TaskLocalScope.name) }
        print(TaskLocalScope.name)
    }
    print(TaskLocalScope.name)
    do {
        try TaskLocalScope.$name.withValue("throwing") { throw TaskLocalFailure() }
    } catch {
        print("caught with", TaskLocalScope.name)
    }

    let done = DispatchSemaphore(value: 0)
    let aBound = DispatchSemaphore(value: 0)
    let bBound = DispatchSemaphore(value: 0)
    Task {
        TaskLocalScope.$name.withValue("a") {
            aBound.signal()
            bBound.wait()
            taskLocalRecord("a")
        }
        done.signal()
    }
    Task {
        taskLocalWait(aBound)
        TaskLocalScope.$name.withValue("b") {
            bBound.signal()
            taskLocalRecord("b")
        }
        done.signal()
    }
    TaskLocalScope.$name.withValue("parent") {
        Task {
            taskLocalRecord("child")
            Task {
                taskLocalRecord("grandchild")
                done.signal()
            }
            done.signal()
        }
        Task.detached {
            taskLocalRecord("detached")
            done.signal()
        }
    }
    for _ in 0..<5 { done.wait() }
    for key in ["a", "b", "child", "grandchild", "detached"] {
        print(key, taskLocalSeen[key] ?? "missing")
    }
    print(TaskLocalScope.name)
}
