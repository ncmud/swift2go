package swiftrt

import (
	"os"
	"testing"
)

func TestFstatReportsOpenFileSize(t *testing.T) {
	path := t.TempDir() + "/log"
	if err := os.WriteFile(path, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fd := OpenUnsafePointerIntInt(path, CMacroO_RDONLY, 0)
	if fd < 0 {
		t.Fatal("open failed")
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	st := CStatInit()
	if CFFstat(fd, &st) != 0 {
		t.Fatal("fstat failed")
	}
	if st.St_size != 6 {
		t.Errorf("St_size = %d, want 6", st.St_size)
	}
	if st.St_uid != uint32(os.Getuid()) { //nolint:gosec // uid fits
		t.Errorf("St_uid = %d, want %d", st.St_uid, os.Getuid())
	}
}

func TestFstatFailsOnClosedDescriptor(t *testing.T) {
	st := CStat{St_size: 7}
	if CFFstat(-1, &st) != -1 {
		t.Error("fstat(-1) succeeded")
	}
	if st.St_size != 7 {
		t.Errorf("St_size = %d, want unchanged 7", st.St_size)
	}
}
