import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public func urlSessionErrorDescription() {
    let done = DispatchSemaphore(value: 0)
    URLSession.shared.dataTask(with: URLRequest(url: URL(string: "http://127.0.0.1:1/refused")!)) { data, response, error in
        print(data == nil, response == nil, error?.localizedDescription ?? "no error")
        done.signal()
    }.resume()
    done.wait()
}
