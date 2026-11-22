private struct ApartShared { let w = "other" }

func privateTypesApartOtherValue() -> String { ApartShared().w }

public func privateTypesApartOther() {
    print(privateTypesApartOtherValue())
}
