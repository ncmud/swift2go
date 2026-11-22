public func printArguments() {
    let xs = [1, 2, 3]
    print(xs)
    print("takes", xs)
    print("a", "b", separator: "-")
    print("no newline", terminator: "|")
    print()
    print(1, 2.5, true, separator: ", ", terminator: ".\n")
    Swift.print("qualified")
    Swift.print("q", 2, separator: "+")
}
