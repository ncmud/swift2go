import Foundation

struct DIJLogin: Decodable { let username: String }

public func decodingInvalidJSON() {
    for json in ["not json", "", #"{"username":"a","#, #"{"username":"a"} x"#, "{not json"] {
        do {
            _ = try JSONDecoder().decode(DIJLogin.self, from: Data(json.utf8))
            print("ok")
        } catch let DecodingError.dataCorrupted(context) {
            print("dataCorrupted", context.codingPath.count, context.debugDescription, context.underlyingError != nil)
        } catch {
            print("other")
        }
    }
}
