package swiftmod

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// jobCacheFormat changes whenever what a job file stores, or how its key is computed, changes.
const jobCacheFormat = "swift2go-job-cache-1"

type cachedJob struct {
	Key string `json:"key"`
	// Frontend identifies the frontend binary the job runs, which a toolchain update replaces.
	Frontend string   `json:"frontend"`
	Job      []string `json:"job"`
}

// cachedFrontendJob is [frontendJob], answered from file when an earlier call with the same
// arguments, directory and toolchain saved its answer there.
func cachedFrontendJob(file, dir string, driverArgs, sources []string) ([]string, error) {
	if file == "" {
		return frontendJob(dir, driverArgs, sources)
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00", jobCacheFormat, dir, toolchainID())
	for _, a := range driverArgs {
		fmt.Fprintf(h, "%s\x00", a)
	}
	key := hex.EncodeToString(h.Sum(nil))
	var c cachedJob
	if data, err := os.ReadFile(file); err == nil && json.Unmarshal(data, &c) == nil &&
		c.Key == key && len(c.Job) > 0 && c.Frontend == fileID(c.Job[0]) {
		return c.Job, nil
	}
	job, err := frontendJob(dir, driverArgs, sources)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(cachedJob{Key: key, Frontend: fileID(job[0]), Job: job})
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return nil, err
	}
	return job, os.Rename(tmp, file)
}

// toolchainID identifies the swiftc that PATH and the environment select: the file it resolves to,
// through Apple's xcrun shims where there are any, the variables that redirect those shims, and the
// developer directory xcode-select chose.
var toolchainID = sync.OnceValue(func() string {
	var b strings.Builder
	p, err := exec.LookPath("swiftc")
	if out, xerr := exec.Command("xcrun", "--find", "swiftc").Output(); xerr == nil {
		p, err = strings.TrimSpace(string(out)), nil
	}
	if err == nil {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		fmt.Fprintf(&b, "%s %s\x00", p, fileID(p))
	}
	for _, v := range []string{"DEVELOPER_DIR", "SDKROOT", "TOOLCHAINS"} {
		fmt.Fprintf(&b, "%s=%s\x00", v, os.Getenv(v))
	}
	link, _ := os.Readlink("/var/db/xcode_select_link")
	fmt.Fprintf(&b, "xcode-select=%s\x00", link)
	return b.String()
})

// fileID identifies a file's content by its size and modification time; "" when it is missing.
func fileID(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d %d", info.Size(), info.ModTime().UnixNano())
}

var frontendVersions sync.Map // frontend path + fileID → version output

// frontendVersion is the output of `frontend -frontend -version`, run once per frontend binary.
func frontendVersion(frontend string) string {
	key := frontend + "\x00" + fileID(frontend)
	if v, ok := frontendVersions.Load(key); ok {
		return v.(string)
	}
	out, _ := exec.Command(frontend, "-frontend", "-version").CombinedOutput()
	frontendVersions.Store(key, string(out))
	return string(out)
}
