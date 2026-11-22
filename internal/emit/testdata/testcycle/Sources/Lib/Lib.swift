struct Counter {
    var count = 0
    mutating func bump() { count += 1 }
}

func makeCounter(start: Int) -> Counter { Counter(count: start) }

struct Tally {
    var vnum: Int
    var wins = 1, losses = 2
    var reason = "none"
    var level: Int?
}

final class Mover {
    var _moves = 0
    func _move() { _moves += 1 }
}
