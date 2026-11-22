import Ledger
import Model

public struct Mail: Persistable {
    public var id: Int64?
    public var subject: String

    public init(subject: String) { self.subject = subject }

    public mutating func didInsert(_ id: Int64) { self.id = id * 100 }
}

public func unreadQuery(_ recipient: String) -> String {
    Mail.filter(Column("to") == recipient && Column("read") == false).limit(10, offset: 5).sql
}

public func anyOfQuery(_ ids: [Int]) -> String {
    Mail.filter(ids.contains(Column("id")) || !(Column("owner") == nil)).sql
}

public func ordering() -> String { Column("id").desc.sql }

public func insertAll(_ subjects: [String]) -> Int {
    let book = Book()
    let n = book.write { b -> Int in
        for s in subjects {
            var mail = Mail(subject: s)
            mail.insert(b)
        }
        return b.count
    }
    book.write { b in
        b.entries.append("done")
    }
    return n * 10 + book.count
}

public func readRow() -> (Int64, String) {
    let row = Row(["id": Int64(7), "name": "ada"])
    return (row["id"], row["name"])
}

public func arguments() -> String {
    let positional: Arguments = [1, 2, 3]
    let named: Arguments = ["min": 3]
    return positional.text + ";" + named.text
}

public func options() -> Int {
    let o: Options = [.strict, .temporary]
    return o.rawValue
}

public func configured() -> Int {
    var config = Config()
    config.timeout = 5
    return config.timeout
}

public func isFast(_ mode: Mode) -> Bool {
    if case .fast = mode { return true }
    return false
}

public func safeRetries() -> Int { Mode.safe(retries: 3).retries + Mode.fast.retries }

public func isBusy(_ code: Code) -> Bool { code == .busy }

public struct Log: AutoIncremented {
    public static let tableName = "log"
    public var id: Int64?
    public var text: String

    public init(text: String) { self.text = text }
}

protocol AutoIncremented: Persistable {
    var id: Int64? { get set }
}

extension AutoIncremented {
    public mutating func didInsert(_ id: Int64) { self.id = id }
}

extension Score: AutoIncremented {
    public static var tableName: String { "scores" }
}

public func registered() -> (String, [Int64?], String) {
    let book = Book()
    var log = Log(text: "x")
    log.insert(book)
    var score = Score(points: 3)
    score.insert(book)
    var mail = Mail(subject: "s")
    mail.insert(book)
    return (book.entries.joined(separator: ","), [log.id, score.id, mail.id], Log.filter(Column("id") == 1).sql)
}

public func boundParameterTypes() -> [String] {
    [
        Handler.bind { (_, _: Book) in }.parameterType,
        Handler.bind { (_, _: Book?) in }.parameterType,
        Handler.bind { (_, _: Int) in }.parameterType,
        Handler.bindIfAny({ (_, _: Book?) in }, Book?.self).parameterType,
        Handler.bindIfAny(nil, Book.self).parameterType,
    ]
}

public func stampedFields() -> (String, String) {
    var fields = Fields()
    fields[.kind] = "note"
    fields[.tag] = "a"
    fields[.tag] = (fields[.tag] ?? "") + "b"
    var message = Message()
    message.fields[.kind] = "reply"
    message.fields[.tag] = fields[.tag]
    message.fields[.kind] = nil
    return (fields.rendered, message.fields.rendered)
}

public func fieldNameTexts() -> [String] {
    [FieldName.kind, .tag].map(\.text)
}

public func messageBodyText() -> String {
    var message = Message()
    message.fields[.kind] = "note"
    return message.body.text
}
