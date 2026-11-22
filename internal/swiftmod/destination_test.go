package swiftmod

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ncmud/swift2go/internal/swifttest"
)

// fakeSwiftSDKs writes an artifact bundle with a two-triple Linux SDK and one with a single-triple SDK,
// laid out as `swift sdk install` leaves them, and returns their directory.
func fakeSwiftSDKs(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	now := time.Now()
	static := filepath.Join(dir, "static-linux.artifactbundle")
	write(t, filepath.Join(static, "info.json"), `{"schemaVersion": "1.0", "artifacts": {
		"static-linux": {"type": "swiftSDK", "version": "0.1.0", "variants": [{"path": "static/swift-linux-musl"}]},
		"tool": {"type": "executable", "version": "1.0", "variants": [{"path": "tool"}]}}}`, now)
	write(t, filepath.Join(static, "static/swift-linux-musl/swift-sdk.json"), `{"schemaVersion": "4.0", "targetTriples": {
		"x86_64-swift-linux-musl": {"sdkRootPath": "musl.sdk/x86_64", "swiftResourcesPath": "musl.sdk/x86_64/usr/lib/swift_static"},
		"aarch64-swift-linux-musl": {"sdkRootPath": "musl.sdk/aarch64", "swiftResourcesPath": "musl.sdk/aarch64/usr/lib/swift_static",
			"includeSearchPaths": ["musl.sdk/aarch64/usr/include/extra"], "librarySearchPaths": ["/opt/lib"]}}}`, now)
	noble := filepath.Join(dir, "noble.artifactbundle")
	write(t, filepath.Join(noble, "info.json"), `{"schemaVersion": "1.0", "artifacts": {
		"noble_x86_64": {"type": "swiftSDK", "version": "0.0.1", "variants": [{"path": "noble/x86_64-unknown-linux-gnu"}]}}}`, now)
	write(t, filepath.Join(noble, "noble/x86_64-unknown-linux-gnu/swift-sdk.json"), `{"schemaVersion": "4.0", "targetTriples": {
		"x86_64-unknown-linux-gnu": {"sdkRootPath": "ubuntu.sdk", "swiftResourcesPath": "ubuntu.sdk/usr/lib/swift"}}}`, now)
	return dir
}

func TestFindSwiftSDK(t *testing.T) {
	dir := fakeSwiftSDKs(t)
	static := filepath.Join(dir, "static-linux.artifactbundle", "static", "swift-linux-musl")

	d, err := FindSwiftSDK("static-linux", "aarch64-swift-linux-musl", dir)
	if err != nil {
		t.Fatal(err)
	}
	want := Destination{
		SDK: "static-linux", Triple: "aarch64-swift-linux-musl",
		Root:      filepath.Join(static, "musl.sdk/aarch64"),
		Resources: filepath.Join(static, "musl.sdk/aarch64/usr/lib/swift_static"),
		Includes:  []string{filepath.Join(static, "musl.sdk/aarch64/usr/include/extra")},
		Libraries: []string{"/opt/lib"},
	}
	if !slices.Equal(d.Includes, want.Includes) || !slices.Equal(d.Libraries, want.Libraries) || d.SDK != want.SDK || d.Triple != want.Triple || d.Root != want.Root || d.Resources != want.Resources {
		t.Errorf("FindSwiftSDK = %+v, want %+v", d, want)
	}
	if got := strings.Join(d.Flags(), " "); !strings.HasPrefix(got, "-target aarch64-swift-linux-musl -sdk "+want.Root+" -resource-dir "+want.Resources) {
		t.Errorf("Flags = %s", got)
	}
	if got := strings.Join(d.SwiftBuildArgs(), " "); got != "--swift-sdk static-linux --triple aarch64-swift-linux-musl" {
		t.Errorf("SwiftBuildArgs = %s", got)
	}
	if d.OS() != "linux" || d.Host() || d.ResourceBundleSuffix() != ".resources" {
		t.Errorf("OS = %q, Host = %v, ResourceBundleSuffix = %q", d.OS(), d.Host(), d.ResourceBundleSuffix())
	}

	// As in `swift build --swift-sdk`, a triple names the SDK that has it, and an SDK with one triple needs no -triple.
	if d, err := FindSwiftSDK("x86_64-swift-linux-musl", "", dir); err != nil || d.SDK != "static-linux" {
		t.Errorf("FindSwiftSDK by triple = %+v, %v", d, err)
	}
	if d, err := FindSwiftSDK("", "x86_64-unknown-linux-gnu", dir); err != nil || d.SDK != "noble_x86_64" {
		t.Errorf("FindSwiftSDK by -triple alone = %+v, %v", d, err)
	}
	if d, err := FindSwiftSDK("noble_x86_64", "", dir); err != nil || d.Triple != "x86_64-unknown-linux-gnu" {
		t.Errorf("FindSwiftSDK of a one-triple SDK = %+v, %v", d, err)
	}

	for _, c := range []struct{ sdk, triple, want string }{
		{"static-linux", "", "pick one with -triple"},
		{"static-linux", "x86_64-unknown-linux-gnu", "has no target triple"},
		{"wasm", "", "no installed Swift SDK matches wasm"},
	} {
		if _, err := FindSwiftSDK(c.sdk, c.triple, dir); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("FindSwiftSDK(%q, %q) err = %v, want %q", c.sdk, c.triple, err, c.want)
		}
	}

	host, err := FindSwiftSDK("", "", dir)
	if err != nil || !host.Host() || host.Flags() != nil || host.SwiftBuildArgs() != nil || host.ResourceBundleSuffix() != ".bundle" {
		t.Errorf("FindSwiftSDK with no SDK = %+v, %v; want the host", host, err)
	}
}

func TestBuildLayoutForDestination(t *testing.T) {
	build := "/pkg/.build"
	linux := Destination{SDK: "static-linux", Triple: "aarch64-swift-linux-musl"}
	if got := ModulesDir(build, Destination{}); got != "/pkg/.build/out/Products/Debug" {
		t.Errorf("host ModulesDir = %s", got)
	}
	if got := ModulesDir(build, linux); got != "/pkg/.build/aarch64-swift-linux-musl/debug/Modules" {
		t.Errorf("Linux ModulesDir = %s", got)
	}

	derived := t.TempDir()
	tg := Target{Name: "Assets", Dir: "/src/pkg", Resources: []Resource{{Path: "a.txt"}}}
	for _, c := range []struct {
		d    Destination
		want string
	}{{Destination{}, `bundleName + ".bundle"`}, {linux, `bundleName + ".resources"`}} {
		path, err := writeBundleAccessor(tg, derived, c.d.ResourceBundleSuffix())
		if err != nil {
			t.Fatal(err)
		}
		src, _ := os.ReadFile(path)
		if !strings.Contains(string(src), c.want) {
			t.Errorf("accessor for %q does not look for %s:\n%s", c.d.Triple, c.want, src)
		}
	}
}

// TestDumperKeyNamesTheTarget checks that an -incremental cache made for one platform is not reused for
// another: the key covers the frontend command, which names the target triple.
func TestDumperKeyNamesTheTarget(t *testing.T) {
	swifttest.Require(t, "xcrun", "swiftc")
	pkg := t.TempDir()
	write(t, filepath.Join(pkg, "Sources/Core/Core.swift"), "public let x = 1\n", time.Now())
	tg := Target{Name: "Core", Path: "Sources/Core", Sources: []string{"Core.swift"}, Dir: pkg}
	host, err := NewDumper(tg, "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	linux, err := NewDumper(tg, "", t.TempDir(), Destination{Triple: "aarch64-swift-linux-musl"}.Flags()...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(linux.Key(), "aarch64-swift-linux-musl") {
		t.Errorf("the Linux key does not name its triple")
	}
	if host.Key() == linux.Key() {
		t.Errorf("the host and Linux dumps share a cache key")
	}
}
