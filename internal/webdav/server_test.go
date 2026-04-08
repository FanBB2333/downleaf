package webdav

import (
	"context"
	"encoding/json"
	"html"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/FanBB2333/downleaf/internal/api"
	"github.com/FanBB2333/downleaf/internal/auth"
	"github.com/FanBB2333/downleaf/internal/model"
)

func TestResolveLoadsTreeFromProjectMetaWithoutSocket(t *testing.T) {
	projectTree := model.ProjectMeta{
		ID:   "p1",
		Name: "test0325",
		RootFolder: []model.Folder{
			{
				ID:   "root",
				Name: "root",
				Folders: []model.Folder{
					{
						ID:   "chapters-id",
						Name: "chapters",
						Docs: []model.Doc{
							{ID: "doc-1", Name: "main.tex"},
						},
					},
				},
				FileRefs: []model.FileRef{
					{ID: "file-1", Name: "refs.bib", Created: time.Unix(1700000000, 0)},
				},
			},
		},
	}

	projectJSON, err := json.Marshal(projectTree)
	if err != nil {
		t.Fatalf("marshal project tree: %v", err)
	}

	projectPage := `<html><head>` +
		`<meta name="ol-csrfToken" content="csrf-token">` +
		`<meta name="ol-project" content="` + html.EscapeString(string(projectJSON)) + `">` +
		`</head></html>`

	projectDetailHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/project/p1":
			projectDetailHits++
			_, _ = w.Write([]byte(projectPage))
		default:
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := api.NewClient(server.URL, &auth.Identity{Cookies: "session=1"})
	ofs := NewOverleafFS(client)
	ofs.projects = []model.Project{{ID: "p1", Name: "test0325"}}
	ofs.projectsFetched = time.Now()

	info, err := ofs.resolve("/test0325/chapters/main.tex")
	if err != nil {
		t.Fatalf("resolve existing doc: %v", err)
	}
	if info.doc == nil || info.doc.ID != "doc-1" {
		t.Fatalf("expected doc-1, got %#v", info.doc)
	}
	if info.parentFolder == nil || info.parentFolder.ID != "chapters-id" {
		t.Fatalf("expected parent folder chapters-id, got %#v", info.parentFolder)
	}
	if len(ofs.sioConn) != 0 {
		t.Fatalf("metadata-only resolve should not open a socket connection")
	}

	_, err = ofs.resolve("/test0325/chapters/missing.tex")
	if !os.IsNotExist(err) {
		t.Fatalf("expected missing entry to return os.ErrNotExist, got %v", err)
	}
	if projectDetailHits != 1 {
		t.Fatalf("expected cached project tree after first resolve, got %d project detail requests", projectDetailHits)
	}
}

func TestBuildProjectTreeStateIndexesNestedEntries(t *testing.T) {
	state := buildProjectTreeState(&model.ProjectMeta{
		RootFolder: []model.Folder{
			{
				ID:   "root",
				Name: "root",
				Folders: []model.Folder{
					{
						ID:   "folder-1",
						Name: "figures",
						FileRefs: []model.FileRef{
							{ID: "file-1", Name: "plot.pdf"},
						},
					},
				},
			},
		},
	})

	if state.root == nil || state.root.ID != "root" {
		t.Fatalf("expected root folder to be indexed")
	}

	entry, ok := state.entries["figures/plot.pdf"]
	if !ok {
		t.Fatalf("expected figures/plot.pdf to be indexed")
	}
	if entry.fileRef == nil || entry.fileRef.ID != "file-1" {
		t.Fatalf("expected file ref file-1, got %#v", entry.fileRef)
	}
	if entry.parentFolder == nil || entry.parentFolder.ID != "folder-1" {
		t.Fatalf("expected parent folder folder-1, got %#v", entry.parentFolder)
	}
}

func TestServerShutdownReleasesPort(t *testing.T) {
	seed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve test port: %v", err)
	}
	addr := seed.Addr().String()
	if err := seed.Close(); err != nil {
		t.Fatalf("release reserved port: %v", err)
	}

	server := NewServer(addr, NewOverleafFS(nil))
	errCh, err := server.Start()
	if err != nil {
		t.Fatalf("start server: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown server: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("serve loop returned error: %v", err)
	}

	reuse, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("port should be reusable after shutdown: %v", err)
	}
	_ = reuse.Close()
}
