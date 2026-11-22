import DiffBase

final class OSSilent: XPDescriptor {
    override func write(_ msg: String) {}
    override func kind() -> String { "silent" }
}

class OSLoud: XPDescriptor {
    var shouts = 0
    init(volume: Int) {
        shouts = volume
        super.init(name: "loud\(volume)")
        XPLog.trace.append("OSLoud.init")
    }
    override func write(_ msg: String) {
        shouts += 1
        super.write(msg.uppercased())
    }
    override var label: String { "LOUD+" + super.label }
}

class OSLouder: OSLoud {
    override func kind() -> String { "louder" }
}

public func openSubclassAcrossPackages() {
    var out: [String] = []
    let all: [XPDescriptor] = [XPDescriptor(name: "plain"), OSSilent(name: "s"), OSSilent(), OSLoud(volume: 2), OSLouder(volume: 5)]
    for d in all {
        out.append(xpDeliver(d, "hi"))
        out.append("\(d.name) \(d is OSSilent) \(d is OSLoud) \(d as? OSLouder != nil)")
    }
    let loud = OSLoud(volume: 1)
    loud.send("a")
    loud.connected = false
    loud.send("b")
    out.append("\(loud.shouts) \(loud.output)")
    out.append(XPLog.trace.joined(separator: ","))
    print(out.joined(separator: "\n"))
}
