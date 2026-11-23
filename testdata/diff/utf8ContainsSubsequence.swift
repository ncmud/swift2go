public func utf8ContainsSubsequence() {
    let report = "Band  Mobs\n\rElite 1\n\r"
    print(report.utf8.contains("Elite 1\n\r".utf8), report.utf8.contains("Elite 2".utf8), [1, 2, 3, 4].contains([2, 3]), [1, 2, 3].contains([3, 1]))
}
