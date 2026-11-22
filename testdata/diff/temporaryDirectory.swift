import Foundation

public func temporaryDirectory() {
    let dir = FileManager.default.temporaryDirectory.appendingPathComponent("swift2go-tmp-\(UUID().uuidString)")
    try! FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    print(FileManager.default.fileExists(atPath: dir.path), dir.path.hasPrefix("/"))
    try! FileManager.default.removeItem(at: dir)
    print(FileManager.default.fileExists(atPath: dir.path))
}
