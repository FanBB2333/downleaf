package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FanBB2333/downleaf/internal/auth"
)

func TestUploadFileIncludesNameField(t *testing.T) {
	const (
		projectID = "project123"
		folderID  = "folder456"
		fileName  = "main.tex"
		csrfToken = "csrf-token"
		content   = "hello world"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/project/"+projectID+"/upload"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("folder_id"), folderID; got != want {
			t.Fatalf("folder_id = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("X-Csrf-Token"), csrfToken; got != want {
			t.Fatalf("X-Csrf-Token = %q, want %q", got, want)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		if got, want := r.FormValue("_csrf"), csrfToken; got != want {
			t.Fatalf("_csrf = %q, want %q", got, want)
		}
		if got, want := r.FormValue("name"), fileName; got != want {
			t.Fatalf("name = %q, want %q", got, want)
		}
		if got, want := r.FormValue("targetFolderId"), folderID; got != want {
			t.Fatalf("targetFolderId = %q, want %q", got, want)
		}
		if got, want := r.FormValue("qqfilename"), fileName; got != want {
			t.Fatalf("qqfilename = %q, want %q", got, want)
		}
		if got, want := r.FormValue("qqtotalfilesize"), "11"; got != want {
			t.Fatalf("qqtotalfilesize = %q, want %q", got, want)
		}

		file, _, err := r.FormFile("qqfile")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		defer file.Close()

		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if got, want := string(body), content; got != want {
			t.Fatalf("uploaded content = %q, want %q", got, want)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, &auth.Identity{
		Cookies:   "sharelatex.sid=abc123",
		CSRFToken: csrfToken,
	})

	if err := client.UploadFile(projectID, folderID, fileName, []byte(content)); err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
}
