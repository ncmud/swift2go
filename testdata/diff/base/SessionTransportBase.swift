import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public protocol SessionTransport: Sendable {
    func data(for request: URLRequest) async throws -> (Data, URLResponse)
}

extension URLSession: SessionTransport {}

public struct SessionClient: Sendable {
    let transport: any SessionTransport

    public init(transport: any SessionTransport = URLSession.shared) {
        self.transport = transport
    }

    public var usesSession: Bool { transport is URLSession }
}
