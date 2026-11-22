// `===` on class-bound existentials compares the objects, even when the protocol has setters.

protocol IdentitySession: AnyObject {
    var label: String { get set }
}

final class IdentityDescriptor: IdentitySession {
    var label: String
    init(_ label: String) { self.label = label }
}

final class IdentityCharacter {
    var sessions: [any IdentitySession] = []

    func attach(_ session: any IdentitySession) -> Bool {
        if sessions.contains(where: { $0 === session }) { return false }
        sessions.append(session)
        return true
    }

    func detach(_ session: any IdentitySession) {
        sessions.removeAll { $0 === session }
    }
}

public func existentialIdentity() {
    let ch = IdentityCharacter()
    let first = IdentityDescriptor("first"), second = IdentityDescriptor("second")
    print(ch.attach(first), ch.attach(first), ch.attach(second), ch.sessions.count)
    ch.detach(first)
    print(ch.sessions.count, ch.sessions.first === second, ch.sessions.first !== first)
    ch.detach(second)
    print(ch.sessions.isEmpty)
}
