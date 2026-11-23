import Foundation

private func dfpFormat(_ pattern: String, _ date: Date, zone: TimeZone, am: String? = nil, pm: String? = nil) -> String {
    let f = DateFormatter()
    f.locale = Locale(identifier: "en_US_POSIX")
    f.timeZone = zone
    if let am { f.amSymbol = am }
    if let pm { f.pmSymbol = pm }
    f.dateFormat = pattern
    return f.string(from: date)
}

public func dateFormatterPatterns() {
    let utc = TimeZone(secondsFromGMT: 0)!
    let ny = TimeZone(identifier: "America/New_York")!
    let evening = Date(timeIntervalSince1970: 1_795_967_045.25)
    let morning = Date(timeIntervalSince1970: 1_771_744_945.5)
    let patterns = [
        "LLLL", "LLLL yyyy", "LLL", "MMMM d, yyyy", "MMM dd,yyyy HH:mm", "EEE MMM dd HH:mm:ss yyyy",
        "EEEE, MMMM d", "yy/MM/dd HH:mm:ss", "yyyyMMdd-HHmmss", "h:mm a zzz", "'at' h 'o''clock'",
        "yyyy-MM-dd'T'HH:mm:ss.SSSZ", "ZZZZZ", "M/d/yy", "D", "K k", "MMMMM EEEEE", "GGG y",
    ]
    for p in patterns {
        print(p, "|", dfpFormat(p, evening, zone: utc), "|", dfpFormat(p, morning, zone: ny))
    }
    print(dfpFormat("MMM d, h:mm a zzz", evening, zone: ny, am: "am", pm: "pm"))
    print(dfpFormat("MMM d, h:mm a", morning, zone: ny, am: "am", pm: "pm"))
}
