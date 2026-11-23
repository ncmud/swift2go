private final class NstcBox { var value = 0 }

private struct NstcView {
    let box: NstcBox
    var value: Int {
        get { box.value }
        nonmutating set { box.value = newValue }
    }
}

private struct NstcMissing: Error {}
private func nstcNeed<T>(_ v: T?) throws -> T { guard let v else { throw NstcMissing() }; return v }

public func nonmutatingSetterThroughCall() {
    let box = NstcBox()
    let view: NstcView? = NstcView(box: box)
    try! nstcNeed(view).value = 26
    try! nstcNeed(view).value += 4
    print(box.value)
}
