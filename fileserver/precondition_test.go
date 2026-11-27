package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/haiwen/seafile-server/fileserver/fsmgr"
)

func newReq() *http.Request {
	return httptest.NewRequest(http.MethodPost, "/seafhttp/upload-api/tok", nil)
}

func TestParseUploadPrecondition(t *testing.T) {
	req := newReq()
	pre, appErr := parseUploadPrecondition(req)
	if appErr != nil || pre.active() {
		t.Fatalf("no header: want inactive and no error, got %+v %v", pre, appErr)
	}

	req.Header.Set("If-None-Match", "*")
	pre, appErr = parseUploadPrecondition(req)
	if appErr != nil || !pre.active() {
		t.Fatalf("If-None-Match: *: want active, got %+v %v", pre, appErr)
	}

	bad := []struct {
		name string
		set  func(*http.Request)
	}{
		{"entity tag", func(r *http.Request) { r.Header.Set("If-None-Match", `"abc"`) }},
		{"blank", func(r *http.Request) { r.Header.Set("If-None-Match", "   ") }},
		{"list", func(r *http.Request) { r.Header.Set("If-None-Match", `*, "abc"`) }},
		{"repeated", func(r *http.Request) {
			r.Header.Add("If-None-Match", "*")
			r.Header.Add("If-None-Match", "*")
		}},
		{"If-Match", func(r *http.Request) { r.Header.Set("If-Match", "*") }},
		{"both", func(r *http.Request) {
			r.Header.Set("If-Match", "*")
			r.Header.Set("If-None-Match", "*")
		}},
	}
	for _, c := range bad {
		r := newReq()
		c.set(r)
		if _, appErr := parseUploadPrecondition(r); appErr == nil || appErr.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %+v", c.name, appErr)
		}
	}
}

func TestRejectPreconditionHeaders(t *testing.T) {
	if appErr := rejectPreconditionHeaders(newReq(), "x"); appErr != nil {
		t.Fatalf("no header: want nil, got %+v", appErr)
	}
	r := newReq()
	r.Header.Set("If-None-Match", "*")
	if appErr := rejectPreconditionHeaders(r, "x"); appErr == nil || appErr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %+v", appErr)
	}
}

func TestCheckPreconditionAgainstDir(t *testing.T) {
	file := func(name string) *fsmgr.SeafDirent {
		return fsmgr.NewDirent("id-"+name, name, uint32(syscall.S_IFREG|0644), 1, "", 1)
	}
	subdir := fsmgr.NewDirent("id-sub", "sub", uint32(syscall.S_IFDIR), 1, "", 0)
	dir := &fsmgr.SeafDir{Entries: []*fsmgr.SeafDirent{file("a.txt"), subdir}}
	pre := uploadPrecondition{ifNoneMatch: true}

	cases := []struct {
		name  string
		dir   *fsmgr.SeafDir
		files []string
		pre   uploadPrecondition
		fail  bool
	}{
		{"inactive ignores existing", dir, []string{"a.txt"}, uploadPrecondition{}, false},
		{"free name", dir, []string{"b.txt"}, pre, false},
		{"existing file", dir, []string{"a.txt"}, pre, true},
		{"existing directory", dir, []string{"sub"}, pre, true},
		{"partial collision", dir, []string{"b.txt", "a.txt"}, pre, true},
		{"duplicate names in request", dir, []string{"b.txt", "b.txt"}, pre, true},
		{"missing parent dir", nil, []string{"a.txt"}, pre, false},
	}
	for _, c := range cases {
		err := checkPreconditionAgainstDir(c.dir, c.files, c.pre)
		if c.fail && !errors.Is(err, ErrPreconditionFailed) {
			t.Errorf("%s: want ErrPreconditionFailed, got %v", c.name, err)
		}
		if !c.fail && err != nil {
			t.Errorf("%s: want nil, got %v", c.name, err)
		}
	}
}
