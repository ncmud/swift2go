import Foundation

let input = readLine(strippingNewline: true) ?? ""
do {
    struct Header: Decodable { var version: Int }
    let header = try JSONDecoder().decode(Header.self, from: Data(input.utf8))
    print("version \(header.version)")
} catch {
    print("not a header")
}
