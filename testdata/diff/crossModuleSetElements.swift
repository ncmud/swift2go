import DiffBase

struct CrossModuleExpedition {
    var flagged: Set<BaseFlaggedAct> = []
}

public func crossModuleSetElements() {
    var expedition = CrossModuleExpedition()
    expedition.flagged.insert(BaseFlaggedAct(name: "loot", vnum: 7))
    expedition.flagged.insert(BaseFlaggedAct(name: "loot", vnum: 7))
    expedition.flagged.insert(BaseFlaggedAct(name: "kill", vnum: nil))
    let empty = Set<BaseFlaggedAct>()
    print(expedition.flagged.count, expedition.flagged.contains { $0.vnum == nil }, expedition.flagged.isEmpty, empty.isEmpty)
    var notes = [BaseFlaggedAct: Int]()
    notes[BaseFlaggedAct(name: "kill", vnum: nil), default: 0] += 1
    notes[BaseFlaggedAct(name: "kill", vnum: nil), default: 0] += 1
    print(notes.count, notes[BaseFlaggedAct(name: "kill", vnum: nil)] ?? 0)
}
