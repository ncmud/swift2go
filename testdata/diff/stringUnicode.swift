import Foundation

public func stringUnicode() {
    print("e\u{301}x".hasPrefix("e"), "e\u{301}x".hasPrefix("é"), "xe\u{301}".hasSuffix("é"))
    print("x\r\n".hasSuffix("\n"), "x\r\n".hasSuffix("\r\n"), "x\n".hasSuffix("\n"), "\r\nx".hasPrefix("\r"))
    print("2nd x_y".capitalized, "hello wORLD".capitalized, "o'neil mc-d".capitalized, "élan vital".capitalized)
    print("straße".uppercased(), "ﬁx".uppercased(), "İ".lowercased().count)
    print("ab\r\n".padding(toLength: 5, withPad: ".", startingAt: 0).utf8.count)
    print("a👍b".padding(toLength: 5, withPad: "-", startingAt: 0), "abcdef".padding(toLength: 3, withPad: "-", startingAt: 0))
    print("x".padding(toLength: 6, withPad: "ab", startingAt: 1))
    print(String(format: "%i|%5@|%-4@|%5d|%-3i|", 4, "ab", "cd", 42, 7))
    print(String(format: "%@ %@", "a", "b"), String(format: "%05.1f %x %X %o %%", 3.14159, 255, 255, 8))
    print(String(format: "%3$@ %1$@ %2$@", "a", "b", "c"))
    for s in ["a.b c:d", "x1y 1x", "don\u{2019}t stop", "a\u{b7}b", "\u{1c6}emal", "\u{c9}COLE \u{e9}l\u{e8}ve", "mcdonald's", "'abc", "\u{65e5}\u{672c}a", "\u{2b0}a", "a\u{200d}b", "\u{2170}\u{2171}", "a^b", "a\u{2018}b", "\u{df} sss", "\u{fb01}sh", "a\tb\nc"] {
        print(s.capitalized)
    }
    print("\u{1c6}".uppercased(), "\u{149}".uppercased(), "\u{3a3}".lowercased(), "\u{391}\u{3a3}".lowercased(), "\u{fb00}".uppercased())
    print(String(format: "%d|%-3d|%ld|%lu|%e|%g|%.2e|%+d|% d|%#x|%5.2f|%-8.3f|", 1, 2, 3, 4, 1234.5, 0.0001, 12345.678, 5, 6, 255, 3.14159, 2.5))
    print(String(format: "%10.4@|%.1@|%g|%g|%x|%lx|%*d|", "abcdef", "xyz", 1234567.0, 100000.0, -1, -1, 4, 7))
}
