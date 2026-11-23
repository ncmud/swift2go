import Foundation

public func crashReporterGaps() {
    let dir = FileManager.default.temporaryDirectory.appendingPathComponent("swift2go-move-\(UUID().uuidString)").path
    try! FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)
    let from = dir + "/crash.log"
    let to = dir + "/crash.log.reported.1"
    try! "fault".write(toFile: from, atomically: false, encoding: .utf8)
    do {
        try FileManager.default.moveItem(atPath: from, toPath: to)
        print("moved", FileManager.default.fileExists(atPath: from), (try? String(contentsOfFile: to, encoding: .utf8)) ?? "")
    } catch {
        print("move failed")
    }
    do {
        try FileManager.default.moveItem(atPath: from, toPath: to)
        print("moved a missing file")
    } catch {
        print("missing source throws")
    }
    try! "again".write(toFile: from, atomically: false, encoding: .utf8)
    do {
        try FileManager.default.moveItem(atPath: from, toPath: to)
        print("moved over an existing file")
    } catch {
        print("existing destination throws", FileManager.default.fileExists(atPath: from))
    }
    try! FileManager.default.removeItem(atPath: dir)

    let start = DispatchTime.now()
    let later = start + 1.5
    let sooner = later - 0.5
    print(later.uptimeNanoseconds - start.uptimeNanoseconds, sooner.uptimeNanoseconds - start.uptimeNanoseconds)
    print(DispatchTime.distantFuture + 2.0 == DispatchTime.distantFuture)
    let semaphore = DispatchSemaphore(value: 0)
    print(semaphore.wait(timeout: .now() + 0.01 + 0.01) == .timedOut)

    fflush(nil)
    FileHandle.standardOutput.write(Data("written to standard output\n".utf8))
}
