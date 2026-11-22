import Glue
import Toy

public func report(name: String) -> String {
    var counter = Counter()
    counter.bump(by: glueAdd(1, 1))
    counter.bump(by: Limits.glue)
    let tally = Tally(total: counter.count)
    var sum = 0
    for n in evens(upTo: 7) {
        sum += n
    }
    let box = Box(sum)
    let mood: String
    switch shade(unbox(box)) {
    case .dark:
        mood = "dark"
    case .light:
        mood = "light"
    }
    let tag = firstOf([Tag("tagged")])
    return "\(greeting(for: name)): \(tally.doubled()) \(unbox(box)) \(mood) \(tag?.name ?? "none")"
}
