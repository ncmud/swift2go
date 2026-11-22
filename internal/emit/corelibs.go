package emit

import (
	"regexp"
	"strings"
)

// Foundation and Dispatch on Linux declare in Swift, Foundation's under FoundationEssentials,
// FoundationInternationalization and FoundationNetworking as well as Foundation, the types that
// Darwin imports from Objective-C. darwinFoundationName respells such names in a demangled symbol
// as Darwin spells them, so a package compiled for Linux calls the same runtime names as one
// compiled for macOS.
func darwinFoundationName(dem string) string {
	if strings.Contains(dem, "Dispatch.Dispatch") {
		dem = dispatchClass.ReplaceAllStringFunc(dem, func(s string) string {
			return "__C.OS_dispatch_" + strings.ToLower(strings.TrimPrefix(s, "Dispatch.Dispatch"))
		})
	}
	if !strings.Contains(dem, "Foundation") {
		return dem
	}
	dem = stringCompareOptions.ReplaceAllString(dem, "__C.NSStringCompareOptions")
	dem = foundationSubmodule.ReplaceAllString(dem, "Foundation.")
	return corelibsClass.ReplaceAllStringFunc(dem, func(s string) string {
		name := strings.TrimPrefix(s, "Foundation.")
		if objc, ok := corelibsObjCNames[name]; ok {
			return "__C." + objc
		}
		return "__C." + name
	})
}

var foundationSubmodule = regexp.MustCompile(`\bFoundation(?:Essentials|Internationalization|Networking)\.`)

// stringCompareOptions matches String.CompareOptions, which Linux declares in an extension of String.
var stringCompareOptions = regexp.MustCompile(`(?:\(extension in Foundation\w*\):)?Swift\.String\.CompareOptions\b`)

// dispatchClass matches the Dispatch classes that Darwin imports as OS_dispatch_<name> objects.
var dispatchClass = regexp.MustCompile(`\bDispatch\.Dispatch(?:Queue|Semaphore|Group|Source)\b`)

// corelibsObjCNames are the Objective-C names on Darwin of Foundation types that Linux declares
// in Swift under their Swift names. The Foundation types whose Swift names keep the NS prefix
// (NSLock, NSRegularExpression, NSNumber, ...) are Objective-C classes on Darwin as well.
var corelibsObjCNames = map[string]string{
	"Bundle":                              "NSBundle",
	"ByteCountFormatter":                  "NSByteCountFormatter",
	"ComparisonResult":                    "NSComparisonResult",
	"DateFormatter":                       "NSDateFormatter",
	"FileAttributeKey":                    "NSFileAttributeKey",
	"FileHandle":                          "NSFileHandle",
	"FileManager":                         "NSFileManager",
	"HTTPURLResponse":                     "NSHTTPURLResponse",
	"ISO8601DateFormatter":                "NSISO8601DateFormatter",
	"ISO8601DateFormatter.Options":        "NSISO8601DateFormatOptions",
	"JSONSerialization":                   "NSJSONSerialization",
	"JSONSerialization.ReadingOptions":    "NSJSONReadingOptions",
	"JSONSerialization.WritingOptions":    "NSJSONWritingOptions",
	"NSRegularExpression.MatchingOptions": "NSMatchingOptions",
	"NSRegularExpression.Options":         "NSRegularExpressionOptions",
	"String.CompareOptions":               "NSStringCompareOptions",
	"URLSessionDataTask":                  "NSURLSessionDataTask",
	"URLSessionTask":                      "NSURLSessionTask",
	"_NSRange":                            "_NSRange",
	"NSRange":                             "_NSRange",
	"NSString.CompareOptions":             "NSStringCompareOptions",
	"NotificationCenter":                  "NSNotificationCenter",
	"NumberFormatter":                     "NSNumberFormatter",
	"OperationQueue":                      "NSOperationQueue",
	"Pipe":                                "NSPipe",
	"Process":                             "NSTask",
	"ProcessInfo":                         "NSProcessInfo",
	"RunLoop":                             "NSRunLoop",
	"Scanner":                             "NSScanner",
	"Thread":                              "NSThread",
	"Timer":                               "NSTimer",
	"URLResponse":                         "NSURLResponse",
	"URLSession":                          "NSURLSession",
	"URLSessionConfiguration":             "NSURLSessionConfiguration",
	"UserDefaults":                        "NSUserDefaults",
}

var corelibsClass = func() *regexp.Regexp {
	names := []string{`NS[A-Z]\w*`}
	for name := range corelibsObjCNames {
		names = append(names, regexp.QuoteMeta(name))
	}
	// Longest first, so a nested type wins over its owner.
	sortByLengthDesc(names)
	return regexp.MustCompile(`\bFoundation\.(?:` + strings.Join(names, "|") + `)\b`)
}()

func sortByLengthDesc(names []string) {
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && len(names[j]) > len(names[j-1]); j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
}

// darwinOptionalProperties are Foundation properties that Linux declares non-Optional and Darwin
// imports as implicitly unwrapped Optionals, which the runtime models.
var darwinOptionalProperties = map[string]bool{
	"__C.NSDateFormatter.amSymbol": true,
	"__C.NSDateFormatter.pmSymbol": true,
}
