import Foundation

public func timeZoneShortNames() {
    let winter = Date(timeIntervalSince1970: 1764547200)
    let summer = Date(timeIntervalSince1970: 1751328000)
    let zones = ["Asia/Tokyo", "America/New_York", "America/Los_Angeles", "America/Mexico_City", "America/Havana",
                 "Asia/Shanghai", "Europe/London", "Europe/Paris", "UTC", "Asia/Kolkata", "Australia/Adelaide",
                 "Pacific/Honolulu", "America/Halifax", "America/Sao_Paulo"]
    for id in zones {
        let zone = TimeZone(identifier: id)!
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.amSymbol = "am"
        f.pmSymbol = "pm"
        f.timeZone = zone
        f.dateFormat = "MMM d, h:mm a zzz"
        print(id, f.string(from: winter), "|", f.string(from: summer), "|",
              zone.abbreviation(for: winter) ?? "-", zone.abbreviation(for: summer) ?? "-")
    }
}
