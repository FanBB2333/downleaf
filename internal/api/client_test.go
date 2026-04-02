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

func TestListTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tag" {
			t.Fatalf("path = %q, want /tag", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("method = %q, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[{"_id":"tag1","name":"Research","project_ids":["p1","p2"]},{"_id":"tag2","name":"Course","project_ids":["p3"]}]`)
	}))
	defer server.Close()

	client := &Client{
		SiteURL:    server.URL,
		Identity:   &auth.Identity{Cookies: "test=1"},
		HTTPClient: server.Client(),
	}

	tags, err := client.ListTags()
	if err != nil {
		t.Fatalf("ListTags() error: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("got %d tags, want 2", len(tags))
	}
	if tags[0].Name != "Research" {
		t.Fatalf("tags[0].Name = %q, want Research", tags[0].Name)
	}
	if len(tags[0].ProjectIDs) != 2 {
		t.Fatalf("tags[0].ProjectIDs len = %d, want 2", len(tags[0].ProjectIDs))
	}
	if tags[1].Name != "Course" {
		t.Fatalf("tags[1].Name = %q, want Course", tags[1].Name)
	}
}
