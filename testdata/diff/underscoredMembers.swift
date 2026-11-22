import DiffBase

public func underscoredMembers() {
    var fields = UnderscoredFields(_type: "weapon", _class: 3)
    fields._class += 1
    print(fields._type, fields._class, fields._describe())
}
