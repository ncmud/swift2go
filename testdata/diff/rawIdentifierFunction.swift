struct RawIdentifierHolder {
    func `get, take and gather`() -> String { "method" }
}

func `raw identifier, with punctuation`() -> String { "free" }

public func rawIdentifierFunction() {
    print(`raw identifier, with punctuation`(), RawIdentifierHolder().`get, take and gather`())
}
