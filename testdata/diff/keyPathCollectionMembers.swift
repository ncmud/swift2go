private struct KpcmClub { let name: String; let members: [Int] }

public func keyPathCollectionMembers() {
    let words = ["alpha", "beta", ""]
    print(words.map(\.first))
    print(words.map(\.count), words.map(\.isEmpty))
    let clubs = [KpcmClub(name: "Owls", members: [3, 1]), KpcmClub(name: "", members: [])]
    print(clubs.map(\.name.first), clubs.map(\.members.first), clubs.map(\.members.count))
}
