import Foundation

public func localizedSort() {
    var names = ["(TIC) Clubhouse", "-=FM=- Clubhouse", "zebra", "Apple", "apple", "10 Downing", "9 Lives", "_under", "éclair", "eclair"]
    names.sort { $0.localizedCaseInsensitiveCompare($1) == .orderedAscending }
    print(names)
    print(names.sorted { $0.localizedStandardCompare($1) == .orderedAscending })
    print(names.sorted { $0.localizedCompare($1) == .orderedAscending })
}
