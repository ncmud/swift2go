import Foundation

// A Swift Bool or number held in `Any` bridges to NSNumber, as JSON-shaped values are checked.

enum BridgingLevel: Int { case low = 1 }

func bridgingKind(_ value: Any) -> String {
    switch value {
    case let object as [String: Any]:
        return "object(\(object.count))"
    case let array as [Any]:
        return "array(\(array.count))"
    case let number as NSNumber:
        return "number \(number.doubleValue) \(number.int64Value)"
    case let text as String:
        return "string \(text)"
    default:
        return "other"
    }
}

public func nsNumberBridging() {
    let values: [Any] = [7, Int64(-3), 2.5, true, UInt8(200), "x", ["a": 1] as [String: Any], [1, 2] as [Any], BridgingLevel.low]
    for value in values {
        print(bridgingKind(value))
    }
    print((42 as Any) is NSNumber, ("42" as Any) is NSNumber, (BridgingLevel.low as Any) is NSNumber)
}
