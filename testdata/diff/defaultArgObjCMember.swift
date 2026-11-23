import DiffBase
import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

private enum DaomError: Error { case unreachable }

private struct DaomFake: SessionTransport {
    func data(for request: URLRequest) async throws -> (Data, URLResponse) {
        throw DaomError.unreachable
    }
}

public func defaultArgObjCMember() {
    print(SessionClient().usesSession)
    print(SessionClient(transport: DaomFake()).usesSession)
}
