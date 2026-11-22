package swiftrt

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSpawnWithFileActions(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	fds := []int32{0, 0}
	if CFPipe(&fds) != 0 {
		t.Fatal("pipe failed")
	}
	readFd, writeFd := fds[0], fds[1]
	if FcntlIntIntInt(readFd, CMacroF_SETFD, CMacroFD_CLOEXEC) != 0 || FcntlIntIntInt(writeFd, CMacroF_GETFD, 0) != 0 {
		t.Fatal("fcntl failed")
	}
	var fileActions *any
	if CFPosix_spawn_file_actions_init(&fileActions) != 0 {
		t.Fatal("file actions init failed")
	}
	CFPosix_spawn_file_actions_adddup2(&fileActions, writeFd, 1)
	CFPosix_spawn_file_actions_adddup2(&fileActions, writeFd, 2)
	CFPosix_spawn_file_actions_addclose(&fileActions, writeFd)
	CFPosix_spawn_file_actions_addclose(&fileActions, readFd)
	script := `printf 'out:%s:%s' "$1" "$GREETING"; printf ' err' >&2; exit 7`
	argv := SliceMap([]string{"sh", "-c", script, "sh", "arg one"}, func(s string) *any { return CFStrdup(CStringOf(s)) })
	argv = append(argv, nil)
	var envList any = []*any{CStringOf("GREETING=hi"), nil}
	envp := &envList
	var pid int32
	result := SliceWithUnsafeMutableBufferPointer(&argv, func(buf *any) int32 {
		return CFPosix_spawn(&pid, CStringOf("/bin/sh"), &fileActions, nil, ForceRef(Flat(UnsafeMutableBufferPointerBaseAddress(*buf))), &envp)
	})
	CFClose(writeFd)
	if result != 0 || pid <= 0 {
		t.Fatalf("posix_spawn = %d, pid %d", result, pid)
	}
	var output []uint8
	buf := SliceInitRepeatingCount[[]uint8](0, 64)
	for {
		n := SliceWithUnsafeMutableBytes(&buf, func(p any) int {
			return CFRead(readFd, UnsafeMutableRawBufferPointerBaseAddress(p), UnsafeMutableRawBufferPointerCount(p))
		})
		if n <= 0 {
			break
		}
		output = append(output, buf[:n]...)
	}
	CFClose(readFd)
	if got := string(output); got != "out:arg one:hi err" {
		t.Errorf("child wrote %q", got)
	}
	var status int32
	for CFWaitpid(pid, &status, 0) < 0 && CErrno() == CMacroEINTR {
	}
	if status>>8 != 7 || status&0x7f != 0 {
		t.Errorf("wait status = %#x, want exit 7", status)
	}
	if CFWaitpid(pid, &status, CMacroWNOHANG) != -1 || CErrno() != CMacroECHILD {
		t.Error("waiting for a reaped child did not fail with ECHILD")
	}
	if CFPosix_spawn_file_actions_destroy(&fileActions) != 0 || fileActions != nil {
		t.Error("file actions destroy failed")
	}

	missing := []*any{CStringOf("nope"), nil}
	if got := CFPosix_spawn(&pid, CStringOf(filepath.Join(t.TempDir(), "nope")), nil, nil, missing, nil); got != CMacroENOENT {
		t.Errorf("posix_spawn of a missing program = %d, want ENOENT", got)
	}
}

func TestKillAndWaitNoHang(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	var pid, status int32
	if got := CFPosix_spawn(&pid, CStringOf("/bin/sh"), nil, nil, []*any{CStringOf("sh"), CStringOf("-c"), CStringOf("sleep 30"), nil}, nil); got != 0 {
		t.Fatalf("posix_spawn = %d", got)
	}
	if got := CFWaitpid(pid, &status, CMacroWNOHANG); got != 0 {
		t.Errorf("waitpid(WNOHANG) on a running child = %d, want 0", got)
	}
	if CFKill(pid, CMacroSIGKILL) != 0 {
		t.Fatal("kill failed")
	}
	if got := CFWaitpid(pid, &status, 0); got != pid || status&0x7f != CMacroSIGKILL {
		t.Errorf("waitpid = %d with status %#x, want the child killed by SIGKILL", got, status)
	}
	if CFKill(pid, 0) != -1 || CErrno() != CMacroESRCH {
		t.Error("kill of a reaped child did not fail with ESRCH")
	}
}

func TestFileStatusFunctions(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(dir, "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(dir, "data")
	if err := os.WriteFile(data, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(tool, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	st, same := CStatInit(), CStatInit()
	if CFStat(CStringOf(data), &st) != 0 || CFStat(CStringOf(data), &same) != 0 {
		t.Fatal("stat failed")
	}
	info, err := os.Stat(data)
	if err != nil {
		t.Fatal(err)
	}
	if st.St_size != 5 || st.St_mode&0o777 != 0o600 || st.St_ino == 0 || st.St_mtimespec.Tv_sec != info.ModTime().Unix() || st.St_ctimespec.Tv_sec == 0 || st != same {
		t.Errorf("stat = %+v", st)
	}
	other := CStatInit()
	if CFStat(CStringOf(tool), &other) != 0 || other.St_ino == st.St_ino || other.St_dev != st.St_dev {
		t.Errorf("stat of another file in the directory = %+v", other)
	}
	if CFStat(CStringOf(filepath.Join(dir, "gone")), &other) != -1 || CErrno() != CMacroENOENT {
		t.Error("stat of a missing file did not fail with ENOENT")
	}

	if CFAccess(CStringOf(tool), CMacroX_OK) != 0 || CFAccess(CStringOf(data), CMacroR_OK|CMacroW_OK) != 0 {
		t.Error("access denied what the mode allows")
	}
	if CFAccess(CStringOf(data), CMacroX_OK) != -1 || CErrno() != CMacroEACCES {
		t.Error("access(X_OK) on a data file did not fail with EACCES")
	}

	resolved := CFRealpath(CStringOf(filepath.Join(dir, ".", "link")), nil)
	if resolved == nil || StringInitCString(resolved) != tool {
		t.Errorf("realpath = %v, want %q", resolved, tool)
	}
	CFFree(Some[any](resolved))
	if CFRealpath(CStringOf(filepath.Join(dir, "gone")), nil) != nil || CErrno() != CMacroENOENT {
		t.Error("realpath of a missing file did not fail with ENOENT")
	}

	first, err := os.OpenFile(data, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := os.OpenFile(data, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if CFFlock(int32(first.Fd()), CMacroLOCK_EX|CMacroLOCK_NB) != 0 {
		t.Fatal("flock failed")
	}
	if CFFlock(int32(second.Fd()), CMacroLOCK_EX|CMacroLOCK_NB) != -1 || CErrno() != CMacroEWOULDBLOCK {
		t.Error("a second exclusive lock did not fail with EWOULDBLOCK")
	}
	if CFFlock(int32(first.Fd()), CMacroLOCK_UN) != 0 || CFFlock(int32(second.Fd()), CMacroLOCK_EX|CMacroLOCK_NB) != 0 {
		t.Error("the lock was not released")
	}
}

func TestArgvAndStrdup(t *testing.T) {
	argv := CommandLineUnsafeArgv
	if int(CommandLineArgc) != len(os.Args) {
		t.Errorf("argc = %d, want %d", CommandLineArgc, len(os.Args))
	}
	if got := StringInitCString(UnsafeMutablePointerPointerSubscriptInt(argv, 0)); got != os.Args[0] {
		t.Errorf("argv[0] = %q, want %q", got, os.Args[0])
	}
	if UnsafeMutablePointerPointerSubscriptInt(argv, int(CommandLineArgc)) != nil {
		t.Error("argv is not NULL-terminated")
	}
	original := UnsafeMutablePointerPointerSubscriptInt(argv, 0)
	UnsafeMutablePointerPointerSetSubscriptInt(argv, 0, CFStrdup(CStringOf("-dlib")))
	if got := StringInitCString(UnsafeMutablePointerPointerSubscriptInt(CommandLineUnsafeArgv, 0)); got != "-dlib" {
		t.Errorf("a write through argv did not persist: %q", got)
	}
	UnsafeMutablePointerPointerSetSubscriptInt(*argv, 0, original)
	if got := cStringList(argv); !reflect.DeepEqual(got, os.Args) {
		t.Errorf("argv = %q, want %q", got, os.Args)
	}

	source := CStringOf("abc")
	copied := CFStrdup(source)
	(*source).([]byte)[0] = 'X'
	if StringInitCString(copied) != "abc" || CFStrlen(copied) != 3 {
		t.Errorf("strdup shares its source: %q", StringInitCString(copied))
	}
}

func TestPointerConversions(t *testing.T) {
	var memory any = []int8{'h', -61, -87, 0, 'x'}
	input := &memory
	n := CFStrlen(input)
	bytes := SliceInitUnsafeBufferPointer[[]uint8](UnsafeBufferPointerInitStartCount(UnsafePointerInitOpaquePointer(OpaquePointerInitUnsafeMutablePointer(input)), n))
	if string(bytes) != "hé" {
		t.Errorf("bytes = %q", bytes)
	}
	if !panics(func() { UnsafeBufferPointerInitStartCount(OpaquePointerInitUnsafeRawPointer(input), 6) }) {
		t.Error("a buffer past the end of its memory did not trap")
	}
	b := UnsafeRawPointerAssumingMemoryBoundTo(UnsafeRawPointerInitUnsafeRawPointer(input), TypeOf[uint8]())
	if uint8(UnsafePointerSubscriptInt(b, 0)) != 'h' || uint8(UnsafePointerSubscriptInt(b, 1)) != 0xC3 || uint8(UnsafePointerSubscriptInt(b, 2)) != 0xA9 {
		t.Error("assumingMemoryBound(to: UInt8.self) does not read the buffer's bytes")
	}
	*b = 'H'
	if memory.([]int8)[0] != 'H' {
		t.Error("the bound pointer does not share the buffer's memory")
	}

	data := []uint8("hello")
	wrote := SliceWithUnsafeBufferPointer(data, func(buf any) int {
		base := Flat(UnsafeBufferPointerBaseAddress(buf))
		if base == nil {
			return -1
		}
		p, _ := cBuffer(base, UnsafeBufferPointerCount(buf))
		return len(p) + int(*base)
	})
	if wrote != 5+'h' {
		t.Errorf("withUnsafeBufferPointer saw %d", wrote)
	}
	if SliceWithUnsafeBufferPointer([]uint8{}, func(buf any) bool { return Flat(UnsafeBufferPointerBaseAddress(buf)) == nil }) != true {
		t.Error("an empty buffer has a base address")
	}
	cstr := []int8{'a', 'b', 0, 'c'}
	text := SliceWithUnsafeBufferPointer(cstr, func(ptr any) string {
		return StringInitDecodingAs(SliceMap(UnsafeBufferPointerPrefixWhile(ptr, func(c int8) bool { return c != 0 }), func(c int8) uint8 { return uint8(c) }), nil)
	})
	if text != "ab" {
		t.Errorf("prefix(while:) = %q", text)
	}
	words := []uint16{0, 0}
	SliceWithUnsafeMutableBytes(&words, func(p any) struct{} {
		raw := UnsafeMutableRawBufferPointerBaseAddress(p).([]byte)
		raw[0], raw[3] = 1, 2
		return struct{}{}
	})
	if UnsafeMutableRawBufferPointerCount(any(make([]byte, 4))) != 4 || words[0] != 1 || words[1] != 0x200 {
		t.Errorf("withUnsafeMutableBytes wrote %v", words)
	}
}

func TestLocaltimeAndStrftime(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("NST", -(3*3600 + 30*60))
	defer func() { time.Local = previous }()
	now := 1609459199 // 2020-12-31 23:59:59 UTC, a Thursday
	tm := **CFLocaltime(&now)
	if tm.Tm_year != 120 || tm.Tm_mon != 11 || tm.Tm_mday != 31 || tm.Tm_hour != 20 || tm.Tm_min != 29 || tm.Tm_sec != 59 || tm.Tm_wday != 4 || tm.Tm_yday != 365 || tm.Tm_gmtoff != -12600 {
		t.Errorf("localtime = %+v", tm)
	}
	cases := []struct{ format, want string }{
		{"%b %d %H:%M", "Dec 31 20:29"},
		{"%a %A %B %h", "Thu Thursday December Dec"},
		{"%Y-%m-%d %T", "2020-12-31 20:29:59"},
		{"%F %R %D %y %C", "2020-12-31 20:29 12/31/20 20 20"},
		{"%I:%M %p|%l|%k|%e", "08:29 PM| 8|20|31"},
		{"%r", "08:29:59 PM"},
		{"%c", "Thu Dec 31 20:29:59 2020"},
		{"%j %u %w %U %W", "366 4 4 52 52"},
		{"%G-W%V %g", "2020-W53 20"},
		{"%z %Z %s", "-0330 NST 1609459199"},
		{"100%% %Q %", "100% %Q %"},
		{"%Ey %Od%n%t", "20 31\n\t"},
	}
	for _, c := range cases {
		buf := SliceInitRepeatingCount[[]int8](0, 64)
		n := StringWithCString(c.format, func(format *any) int { return CFStrftime(&buf, len(buf), format, &tm) })
		if got := StringInitCString(buf); got != c.want || n != len(c.want) {
			t.Errorf("strftime(%q) = %q (%d), want %q", c.format, got, n, c.want)
		}
	}
	small := SliceInitRepeatingCount[[]int8](7, 4)
	if n := CFStrftime(&small, len(small), CStringOf("%Y"), &tm); n != 0 || small[0] != 7 {
		t.Errorf("strftime into a short buffer returned %d and wrote %v", n, small)
	}
	january := 1609632000 + 12600 // 2021-01-03 00:00 local, a Sunday in ISO week 53 of 2020
	early := **CFLocaltime(&january)
	if got := strftime("%G-W%V-%u %U %W", &early); got != "2020-W53-7 01 00" {
		t.Errorf("week numbers = %q", got)
	}
}
