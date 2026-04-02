package model

import "time"

// Project represents an Overleaf project.
type Project struct {
	ID            string `json:"_id"`
	Name          string `json:"name"`
	LastUpdated   string `json:"lastUpdated"`
	AccessLevel   string `json:"accessLevel"`
	Source        string `json:"source"`
	Archived      bool   `json:"archived"`
	Trashed       bool   `json:"trashed"`
	RootDocID     string `json:"rootDoc_id"`
	RootFolderID  string `json:"-"`
}

// Tag represents an Overleaf tag (label) that groups projects.
type Tag struct {
	ID         string   `json:"_id"`
	Name       string   `json:"name"`
	ProjectIDs []string `json:"project_ids"`
}

// Folder represents a folder in the project file tree.
type Folder struct {
	ID       string    `json:"_id"`
	Name     string    `json:"name"`
	Docs     []Doc     `json:"docs"`
	FileRefs []FileRef `json:"fileRefs"`
	Folders  []Folder  `json:"folders"`
}

// Doc represents a .tex document (content retrieved via Socket.IO).
type Doc struct {
	ID   string `json:"_id"`
	Name string `json:"name"`
}

// FileRef represents a binary file (images, PDFs, etc).
type FileRef struct {
	ID      string    `json:"_id"`
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
}

// Entity represents a flat file tree entry from /project/:id/entities.
type Entity struct {
	Path string `json:"path"`
	Type string `json:"type"` // "doc" or "file"
}

// ProjectDetail holds the full project metadata parsed from the editor page.
type ProjectDetail struct {
	Project   ProjectMeta `json:"project"`
	CSRFToken string      `json:"-"`
}

// ProjectMeta is the project JSON embedded in the editor HTML page.
type ProjectMeta struct {
	ID         string `json:"_id"`
	Name       string `json:"name"`
	RootDocID  string `json:"rootDoc_id"`
	RootFolder []Folder `json:"rootFolder"`
}
