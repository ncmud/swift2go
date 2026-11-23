import Foundation

public func httpURLResponseInit() {
    let r = HTTPURLResponse(url: URL(string: "https://example.com/a")!, statusCode: 503, httpVersion: nil,
                            headerFields: ["Retry-After": "5"])!
    print(r.statusCode, r.url?.path ?? "-", r.value(forHTTPHeaderField: "retry-after") ?? "-")
    let plain: URLResponse = r
    print(plain.url?.host ?? "-", (plain as? HTTPURLResponse)?.statusCode ?? 0)
}
