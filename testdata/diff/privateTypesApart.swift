private struct ApartShared { let v = 3 }

struct ApartOuter: Codable {
    private struct ApartShared { let label = "nested" }
    var n = 1
    var label: String { ApartShared().label }
}

public func privateTypesApart() {
    print(ApartShared().v, privateTypesApartOtherValue(), ApartOuter().label)
}
