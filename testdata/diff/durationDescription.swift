public func durationDescription() {
    let ds: [Duration] = [.milliseconds(50), .seconds(1), .seconds(90), .nanoseconds(1), .zero, .milliseconds(-1500), .microseconds(3)]
    for d in ds {
        print(d, "\(d)")
    }
    print([Duration.seconds(2)])
    let budget: Duration = .milliseconds(50)
    print("Limits: \(budget) per resume")
}
