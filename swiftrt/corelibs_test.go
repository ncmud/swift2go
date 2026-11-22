package swiftrt

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNSNumberObjCType(t *testing.T) {
	for n, want := range map[*NSNumber]int8{{kind: 'b'}: 'c', {kind: 'i'}: 'q', {kind: 'd'}: 'd'} {
		if got := UnsafePointerPointee(NSNumberObjCType(*n)); got != want {
			t.Errorf("objCType of %c = %c, want %c", n.kind, got, want)
		}
	}
}

func TestMemoryLayoutSize(t *testing.T) {
	if MemoryLayoutSize[int32]() != 4 || MemoryLayoutSize[float64]() != 8 {
		t.Errorf("MemoryLayout sizes = %d, %d", MemoryLayoutSize[int32](), MemoryLayoutSize[float64]())
	}
}

func TestURLSessionDataTask(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()
	session := NSURLSessionInitConfiguration(NSURLSessionConfigurationEphemeral)
	done := make(chan string, 1)
	task := NSURLSessionDataTaskWithCompletionHandler(session, URLRequestInitUrl(URL{srv.URL}), func(data *[]byte, resp *URLResponse, err error) {
		if err != nil {
			done <- "error: " + err.Error()
			return
		}
		done <- string(*data)
	})
	NSURLSessionTaskResume(task)
	select {
	case got := <-done:
		if got != "ok" {
			t.Errorf("completion got %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no completion")
	}

	cancelled := NSURLSessionDataTaskWithCompletionHandler(session, URLRequestInitUrl(URL{srv.URL}), func(_ *[]byte, _ *URLResponse, err error) {
		if e, ok := err.(URLError); ok && e.Code == URLErrorCancelled {
			done <- "cancelled"
			return
		}
		done <- "not cancelled"
	})
	NSURLSessionTaskCancel(cancelled)
	if got := <-done; got != "cancelled" {
		t.Errorf("cancelled task: %s", got)
	}
	NSURLSessionFinishTasksAndInvalidate(session)
}
