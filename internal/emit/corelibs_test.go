package emit

import "testing"

func TestDarwinFoundationName(t *testing.T) {
	for in, want := range map[string]string{
		"Foundation.Bundle":                                           "__C.NSBundle",
		"Foundation.Bundle.main : Foundation.Bundle":                  "__C.NSBundle.main : __C.NSBundle",
		"FoundationEssentials.URL?":                                   "Foundation.URL?",
		"FoundationNetworking.HTTPURLResponse.statusCode : Swift.Int": "__C.NSHTTPURLResponse.statusCode : Swift.Int",
		"Foundation.Process.run() throws -> ()":                       "__C.NSTask.run() throws -> ()",
		"Foundation.NSLock":                                           "__C.NSLock",
		"Foundation.JSONSerialization.WritingOptions.sortedKeys":      "__C.NSJSONWritingOptions.sortedKeys",
		"Foundation.JSONSerialization.data(withJSONObject:options:)":  "__C.NSJSONSerialization.data(withJSONObject:options:)",
		"FoundationEssentials.String.CompareOptions":                  "__C.NSStringCompareOptions",
		"static (extension in FoundationEssentials):Swift.String.CompareOptions.regularExpression : (extension in FoundationEssentials):Swift.String.CompareOptions": "static __C.NSStringCompareOptions.regularExpression : __C.NSStringCompareOptions",
		"Foundation._NSRange":                   "__C._NSRange",
		"Dispatch.DispatchQueue.sync(execute:)": "__C.OS_dispatch_queue.sync(execute:)",
		"Dispatch.DispatchTime":                 "Dispatch.DispatchTime",
		"Foundation.JSONEncoder":                "Foundation.JSONEncoder",
		"Foundation.BundleKit.Thing":            "Foundation.BundleKit.Thing",
		"MyFoundation.Bundle":                   "MyFoundation.Bundle",
	} {
		if got := darwinFoundationName(in); got != want {
			t.Errorf("darwinFoundationName(%q) = %q, want %q", in, got, want)
		}
	}
}
