protocol FieldBesideMethodStore {
    func kills(ofRun run: Int) -> [Int]
}

extension FieldBesideMethodStore {
    func kills(ofRun run: Int) -> [Int] { [] }
}

final class FieldBesideMethodRecorder: FieldBesideMethodStore {
    var kills: [(run: Int, vnum: Int)] = []
    func kills(ofRun run: Int) -> [Int] { kills.filter { $0.run == run }.map(\.vnum) }
}

public func fieldBesideMethod() {
    let recorder = FieldBesideMethodRecorder()
    recorder.kills.append((run: 1, vnum: 3001))
    recorder.kills.append((run: 2, vnum: 3002))
    let store: FieldBesideMethodStore = recorder
    print(store.kills(ofRun: 1), recorder.kills.count, recorder.kills.map(\.run))
}
