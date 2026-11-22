enum FIBError: Error { case bad }

func fibCheck(_ n: Int) throws -> Int {
    if n == 13 { throw FIBError.bad }
    return n
}

struct FIBThing {
    let n: Int
    init?(_ n: Int) {
        if n < 0 {
            defer { print("rejecting \(n)") }
            return nil
        }
        do {
            self.n = try fibCheck(n)
        } catch {
            print("caught for \(n)")
            return nil
        }
    }
}

final class FIBBox {
    let n: Int
    init?(_ n: Int) {
        for i in 0..<3 {
            defer { print("box loop \(i)") }
            if i == n { return nil }
        }
        self.n = n
    }
}

public func failableInitBlock() {
    for n in [-1, 2, 13] {
        print(n, FIBThing(n)?.n ?? -100)
    }
    for n in [1, 5] {
        print("box", n, FIBBox(n)?.n ?? -100)
    }
}
