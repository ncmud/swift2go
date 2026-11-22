protocol PDNamed { func title() -> String }
extension PDNamed {
    func title() -> String { "untitled" }
    func heading() -> String { "# " + title() }
}
class PDDoc: PDNamed {}
class PDReport: PDDoc { func title() -> String { "report" } }
class PDAnnual: PDReport { override func title() -> String { "annual " + super.title() } }

class PDMemo: PDNamed { func title() -> String { "memo" } }
class PDNote: PDMemo { override func title() -> String { "note" } }

protocol PDSized { var size: Int { get } }
extension PDSized { var size: Int { 0 } }
class PDBox: PDSized {}
class PDBigBox: PDBox { var size: Int { 99 } }
class PDHugeBox: PDBigBox { override var size: Int { super.size * 2 } }

func PDtitled<T: PDNamed>(_ x: T) -> String { x.title() }

public func protocolDefaultSubclass() {
    var out: [String] = []
    let n: PDNamed = PDReport()
    out.append(n.title())
    out.append(n.heading())
    out.append(PDReport().title())
    out.append(PDtitled(PDReport()))
    let d: PDDoc = PDReport()
    out.append(d.title())
    let m: PDNamed = PDNote()
    out.append(m.title() + " " + m.heading() + " " + PDtitled(PDNote()))
    let s: PDSized = PDBigBox()
    out.append("\(s.size) \(PDBigBox().size)")
    let a: PDNamed = PDAnnual()
    let r: PDReport = PDAnnual()
    out.append(a.title() + " | " + r.title() + " | " + PDtitled(PDAnnual()))
    let h: PDSized = PDHugeBox()
    let hb: PDBigBox = PDHugeBox()
    out.append("\(h.size) \(hb.size)")
    print(out.joined(separator: "\n"))
}
