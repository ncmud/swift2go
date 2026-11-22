package swiftrt

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func optString(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestURLParts(t *testing.T) {
	cases := []struct {
		in, scheme, host, last, parent, stem string
	}{
		{"https://example.com/a/b.mp3", "https", "example.com", "b.mp3", "https://example.com/a/", "https://example.com/a/b"},
		{"https://example.com/a/", "https", "example.com", "a", "https://example.com/", "https://example.com/a/"},
		{"https://example.com", "https", "example.com", "", "https://example.com", "https://example.com"},
		{"https://example.com/", "https", "example.com", "/", "https://example.com/", "https://example.com/"},
		{"HTTP://user:pw@Host.example:8080/x%20y.tar.gz?q=1#f", "HTTP", "Host.example", "x y.tar.gz", "HTTP://user:pw@Host.example:8080/?q=1#f", "HTTP://user:pw@Host.example:8080/x%20y.tar?q=1#f"},
		{"http://[::1]:80/a.d/", "http", "::1", "a.d", "http://[::1]:80/", "http://[::1]:80/a/"},
		{"file:///tmp/.hidden", "file", "<nil>", ".hidden", "file:///tmp/", "file:///tmp/.hidden"},
		{"/only/a/path", "<nil>", "<nil>", "path", "/only/a/", "/only/a/path"},
		{"https://example.com/a/..", "https", "example.com", "..", "https://example.com/a/../", "https://example.com/a/.."},
		{"https://example.com/a/b c.x y", "https", "example.com", "b c.x y", "https://example.com/a/", "https://example.com/a/b c.x y"},
	}
	for _, c := range cases {
		u := URL{c.in}
		if got := optString(URLScheme(u)); got != c.scheme {
			t.Errorf("URLScheme(%q) = %q, want %q", c.in, got, c.scheme)
		}
		if got := optString(URLHost(u)); got != c.host {
			t.Errorf("URLHost(%q) = %q, want %q", c.in, got, c.host)
		}
		if got := URLLastPathComponent(u); got != c.last {
			t.Errorf("URLLastPathComponent(%q) = %q, want %q", c.in, got, c.last)
		}
		if got := URLDeletingLastPathComponent(u).Raw; got != c.parent {
			t.Errorf("URLDeletingLastPathComponent(%q) = %q, want %q", c.in, got, c.parent)
		}
		if got := URLDeletingPathExtension(u).Raw; got != c.stem {
			t.Errorf("URLDeletingPathExtension(%q) = %q, want %q", c.in, got, c.stem)
		}
		if got := URLAbsoluteString(u); got != c.in {
			t.Errorf("URLAbsoluteString(%q) = %q", c.in, got)
		}
	}
}

func TestNSStringPathComponents(t *testing.T) {
	cases := []struct{ in, last, parent string }{
		{"/tmp/scratch.tiff", "scratch.tiff", "/tmp"},
		{"/tmp/lock/", "lock", "/tmp"},
		{"/tmp/", "tmp", "/"},
		{"/tmp", "tmp", "/"},
		{"/", "/", "/"},
		{"scratch.tiff", "scratch.tiff", ""},
		{"a//b//", "b", "a"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := NSStringLastPathComponent(c.in); got != c.last {
			t.Errorf("NSStringLastPathComponent(%q) = %q, want %q", c.in, got, c.last)
		}
		if got := NSStringDeletingLastPathComponent(c.in); got != c.parent {
			t.Errorf("NSStringDeletingLastPathComponent(%q) = %q, want %q", c.in, got, c.parent)
		}
	}
}

func TestURLAppendingPath(t *testing.T) {
	cases := []struct{ base, path, want string }{
		{"https://mudvault.org/api/rewards", "characters/unlink", "https://mudvault.org/api/rewards/characters/unlink"},
		{"https://mudvault.org/api/rewards/", "/pending/7", "https://mudvault.org/api/rewards/pending/7"},
		{"https://mudvault.org/api/", "a b%?#", "https://mudvault.org/api/a%20b%25%3F%23"},
		{"https://mudvault.org", "x", "https://mudvault.org/x"},
		{"https://mudvault.org/api?k=v", "x/", "https://mudvault.org/api/x/?k=v"},
	}
	for _, c := range cases {
		if got := URLAppendingPath(URL{c.base}, c.path).Raw; got != c.want {
			t.Errorf("URLAppendingPath(%q, %q) = %q, want %q", c.base, c.path, got, c.want)
		}
	}
}

func TestURLComponentsQueryItems(t *testing.T) {
	c := URLComponentsOptInitUrlResolvingAgainstBaseURL(URL{"https://example.com/api/unsubscribe"}, false)
	if c == nil || c.QueryItems != nil {
		t.Fatalf("components = %+v, want no query items", c)
	}
	c.QueryItems = new([]URLQueryItem{
		URLQueryItemInitNameValue("token", new("a b&c=d+e/é")),
		URLQueryItemInitNameValue("flag", nil),
	})
	if got, want := optURL(URLComponentsUrl(*c)), "https://example.com/api/unsubscribe?token=a%20b%26c%3Dd+e/%C3%A9&flag"; got != want {
		t.Errorf("url = %q, want %q", got, want)
	}
	c.QueryItems = new([]URLQueryItem{})
	if got, want := optURL(URLComponentsUrl(*c)), "https://example.com/api/unsubscribe?"; got != want {
		t.Errorf("url with no items = %q, want %q", got, want)
	}

	parsed := URLComponentsOptInitString("http://h/p q?a=1%2B1&b&c=#frag ment")
	if parsed == nil || parsed.QueryItems == nil {
		t.Fatalf("components = %+v", parsed)
	}
	items := *parsed.QueryItems
	if len(items) != 3 || items[0].Name != "a" || optString(items[0].Value) != "1+1" || items[1].Value != nil || optString(items[2].Value) != "" {
		t.Errorf("queryItems = %+v", items)
	}
	if got, want := optURL(URLComponentsUrl(*parsed)), "http://h/p%20q?a=1%2B1&b&c=#frag%20ment"; got != want {
		t.Errorf("round trip = %q, want %q", got, want)
	}
	parsed.QueryItems = nil
	if got, want := optURL(URLComponentsUrl(*parsed)), "http://h/p%20q#frag%20ment"; got != want {
		t.Errorf("without query = %q, want %q", got, want)
	}
	if got := URLComponentsUrl(URLComponents{}); got != nil {
		t.Errorf("empty components url = %q, want nil", got.Raw)
	}
}

func optURL(u *URL) string {
	if u == nil {
		return "<nil>"
	}
	return u.Raw
}

func TestURLFilePaths(t *testing.T) {
	if got, want := URLStandardizedFileURL(URL{"file:///a/./b/../c//d/"}).Raw, "file:///a/c/d/"; got != want {
		t.Errorf("standardizedFileURL = %q, want %q", got, want)
	}
	if got, want := URLStandardizedFileURL(URL{"https://h/a/../b"}).Raw, "https://h/a/../b"; got != want {
		t.Errorf("standardizedFileURL of a web URL = %q, want %q", got, want)
	}

	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("caf\xe9"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	resolved := URLPath(URLResolvingSymlinksInPath(URLInitFileURLWithPath(filepath.Join(dir, "sub", "..", "link.txt"))))
	want := URLPath(URLResolvingSymlinksInPath(URLInitFileURLWithPath(target)))
	if resolved != want || filepath.Base(resolved) != "target.txt" {
		t.Errorf("resolvingSymlinksInPath = %q, want %q", resolved, want)
	}
	missing := filepath.Join(dir, "missing", ".", "file")
	if got, want := URLPath(URLResolvingSymlinksInPath(URL{"file://" + missing})), filepath.Join(dir, "missing", "file"); got != want {
		t.Errorf("resolvingSymlinksInPath of a missing file = %q, want %q", got, want)
	}

	if got := StringInitContentsOfEncoding(URLInitFileURLWithPath(link), EncodingIsoLatin1); got != "café" {
		t.Errorf("Latin-1 contents = %q", got)
	}
	if err := Catch(func() { _ = StringInitContentsOfEncoding(URLInitFileURLWithPath(link), EncodingUtf8) }); err == nil {
		t.Error("reading Latin-1 bytes as UTF-8 did not throw")
	}
}

func TestBundle(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	flat := filepath.Join(root, "ncmud_WorldDatabase.bundle")
	nested := filepath.Join(root, "App.bundle", "Contents", "Resources")
	for _, dir := range []string{flat, nested} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "baseline.sqlite"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if NSBundleOptInitURL(URLInitFileURLWithPath(filepath.Join(root, "missing.bundle"))) != nil {
		t.Error("Bundle(url:) of a missing directory is not nil")
	}
	if NSBundleOptInitURL(URLInitFileURLWithPath(filepath.Join(flat, "baseline.sqlite"))) != nil {
		t.Error("Bundle(url:) of a file is not nil")
	}
	b := NSBundleOptInitURL(URLAppendingPathComponentString(URLInitFileURLWithPath(root+"/"), "ncmud_WorldDatabase.bundle"))
	if b == nil {
		t.Fatal("Bundle(url:) of a directory is nil")
	}
	if got, want := NSBundleBundleURL(*b).Raw, "file://"+flat+"/"; got != want {
		t.Errorf("bundleURL = %q, want %q", got, want)
	}
	if got, want := optURL(NSBundleResourceURL(*b)), "file://"+flat+"/"; got != want {
		t.Errorf("resourceURL = %q, want %q", got, want)
	}
	if got, want := optURL(NSBundleUrlForResourceWithExtension(*b, new("baseline"), new("sqlite"))), "file://"+flat+"/baseline.sqlite"; got != want {
		t.Errorf("url(forResource:withExtension:) = %q, want %q", got, want)
	}
	if got, want := optURL(NSBundleUrlForResourceWithExtension(*b, new("baseline.sqlite"), nil)), "file://"+flat+"/baseline.sqlite"; got != want {
		t.Errorf("url(forResource:) with the extension in the name = %q, want %q", got, want)
	}
	if got, want := optURL(NSBundleUrlForResourceWithExtension(*b, nil, new("sqlite"))), "file://"+flat+"/baseline.sqlite"; got != want {
		t.Errorf("url(forResource: nil) = %q, want %q", got, want)
	}
	if got := NSBundleUrlForResourceWithExtension(*b, new("baseline"), new("json")); got != nil {
		t.Errorf("missing resource = %q, want nil", got.Raw)
	}
	app := Bundle{filepath.Join(root, "App.bundle")}
	if got, want := optURL(NSBundleUrlForResourceWithExtension(app, new("baseline"), new("sqlite"))), "file://"+nested+"/baseline.sqlite"; got != want {
		t.Errorf("macOS bundle resource = %q, want %q", got, want)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		t.Fatal(err)
	}
	if got, want := URLPath(NSBundleBundleURL(NSBundleMain)), filepath.Dir(exe); got != want {
		t.Errorf("Bundle.main.bundleURL path = %q, want %q", got, want)
	}
}

// TestBundleBesideTheRealExecutable copies the test binary beside a resource bundle and runs it
// through a symlink in another directory, by a relative path, from a third working directory. The
// child finds the bundle the way a generated Bundle.module accessor does.
func TestBundleBesideTheRealExecutable(t *testing.T) {
	const bundle = "pkg_Target.bundle"
	if os.Getenv("SWIFTRT_BUNDLE_CHILD") != "" {
		main := NSBundleBundleURL(NSBundleMain)
		found := NSBundleOptInitURL(URLAppendingPathComponentString(main, bundle)) != nil
		fmt.Printf("%s %v", URLPath(main), found)
		os.Exit(0)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app, bin, lib := filepath.Join(root, "app"), filepath.Join(root, "bin"), filepath.Join(root, "lib")
	for _, dir := range []string{filepath.Join(app, bundle), bin, lib} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	copyExecutable(t, filepath.Join(app, "server"))
	if err := os.Symlink(filepath.Join(app, "server"), filepath.Join(bin, "server")); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("../bin/server", "-test.run=^TestBundleBesideTheRealExecutable$")
	cmd.Dir = lib
	cmd.Env = append(os.Environ(), "SWIFTRT_BUNDLE_CHILD=1")
	out, err := cmd.Output()
	if want := app + " true"; err != nil || string(out) != want {
		t.Fatalf("child printed %q, err %v; want %q", out, err, want)
	}
}

func copyExecutable(t *testing.T, dst string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, src); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOptionalCasesAndUnitFns(t *testing.T) {
	var inner *int64
	if p := RefSome(inner); p == nil || *p != nil {
		t.Error("RefSome(nil) is not a present optional holding nil")
	}
	if p := OptSome("x"); p == nil || *p != "x" {
		t.Error("OptSome lost its value")
	}
	if got := IntDescription(uint64(1<<63 + 5)); got != "9223372036854775813" {
		t.Errorf("IntDescription = %q", got)
	}
	if got := IntDescription(int8(-128)); got != "-128" {
		t.Errorf("IntDescription = %q", got)
	}
	sum := 0
	UnitFn2(func(a, b int) { sum += a + b })(1, 2)
	UnitFn3(func(a, b, c int) { sum += a + b + c })(1, 2, 3)
	UnitFn4(func(a, b, c, d int) { sum += a + b + c + d })(1, 2, 3, 4)
	if sum != 19 {
		t.Errorf("UnitFn closures ran to %d, want 19", sum)
	}
}
