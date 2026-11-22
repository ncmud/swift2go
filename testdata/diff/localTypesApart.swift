import Foundation

func localTypesApartFirst() -> Int {
    final class Counter { var n = 1 }
    let c = Counter()
    c.n += 1
    return c.n
}

func localTypesApartSecond() -> String {
    struct Counter { var label = "second" }
    return Counter().label
}

func localTypesApartDecodeA() -> Int {
    struct Envelope: Codable { let a: Int }
    return try! JSONDecoder().decode(Envelope.self, from: Data("{\"a\":1}".utf8)).a
}

func localTypesApartDecodeB() -> String {
    struct Envelope: Codable { let b: String }
    return try! JSONDecoder().decode(Envelope.self, from: Data("{\"b\":\"x\"}".utf8)).b
}

public func localTypesApart() {
    print(localTypesApartFirst(), localTypesApartSecond(), localTypesApartDecodeA(), localTypesApartDecodeB())
}
