import Dispatch

private actor AcaLedger {
    let label: String
    private(set) var entries = 0

    init(label: String) { self.label = label }

    func record() { entries += 1 }

    func forward(to other: AcaLedger) async {
        record()
        await other.echo(back: self)
    }

    func echo(back other: AcaLedger) async {
        record()
        await other.record()
    }
}

public func actorCallsActor() {
    let a = AcaLedger(label: "a")
    let b = AcaLedger(label: "b")
    let done = DispatchSemaphore(value: 0)
    let rounds = 300
    Task.detached {
        for _ in 0..<rounds { await a.forward(to: b) }
        done.signal()
    }
    Task.detached {
        for _ in 0..<rounds { await b.forward(to: a) }
        done.signal()
    }
    done.wait()
    done.wait()
    Task.detached {
        print(a.label, await a.entries)
        print(b.label, await b.entries)
        done.signal()
    }
    done.wait()
}
