import Foundation

public func nsDictionarySubscript() {
    let data = Data("{\"hp\": 12, \"name\": \"ally\", \"debt\": null}".utf8)
    let dict = try! JSONSerialization.jsonObject(with: data) as! NSDictionary
    print(dict["hp"] as? Int ?? -1, dict["name"] as? String ?? "?", dict["missing"] == nil, dict["debt"] is NSNull)
}
