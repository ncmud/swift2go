public protocol Inserting {
    mutating func didInsert(_ id: Int64)
}

extension Inserting {
    public mutating func didInsert(_ id: Int64) {}
}

protocol AutoIncremented: Inserting {
    var id: Int64? { get set }
}

extension AutoIncremented {
    public mutating func didInsert(_ id: Int64) { self.id = id }
}

struct Log: AutoIncremented {
    var id: Int64?
    var text: String
}

final class Entry: AutoIncremented {
    var id: Int64?
    init() {}
}

func insertLog(_ id: Int64) -> Int64? {
    var log = Log(id: nil, text: "x")
    log.didInsert(id)
    return log.id
}

func describe(_ record: any AutoIncremented) -> String {
    record.id.map { "#\($0)" } ?? "new"
}

func renumber(_ id: Int64) -> String {
    var log = Log(id: 1, text: "x")
    log.id = id
    return describe(log)
}

func insertEntry(_ id: Int64) -> Int64? {
    var entry = Entry()
    entry.didInsert(id)
    return entry.id
}
