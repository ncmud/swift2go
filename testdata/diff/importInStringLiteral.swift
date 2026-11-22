import DiffBase

let ilsNotes = ["the whole world.", "see diffbase.UnderscoredFields in prose", "UnderscoredFields.x"]

public func importInStringLiteral() {
    print(ilsNotes.joined(separator: "|"))
}
