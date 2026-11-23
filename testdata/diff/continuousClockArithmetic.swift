public func continuousClockArithmetic() {
    let clock = ContinuousClock()
    let start = clock.now
    let deadline = start + .seconds(5)
    let earlier = deadline - .seconds(10)
    print(start < deadline, deadline < start, earlier < start, deadline - start == .seconds(5))
    var spins = 0
    while ContinuousClock.now < start + .milliseconds(1) { spins += 1 }
    let elapsed = clock.measure { spins += 1 }
    print(spins > 0, elapsed >= .zero)
}
