package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/FanBB2333/downleaf/internal/auth"
	"github.com/FanBB2333/downleaf/internal/model"
)

// Client is the Overleaf REST API client.
type Client struct {
	SiteURL    string
	Identity   *auth.Identity
	HTTPClient *http.Client
}

// NewClient creates a new API client with the given identity.
func NewClient(siteURL string, identity *auth.Identity) *Client {
	return &Client{
		SiteURL:    strings.TrimRight(siteURL, "/"),
		Identity:   identity,
		HTTPClient: &http.Client{},
	}
}

// request creates an authenticated HTTP request.
func (c *Client) request(method, path string, body io.Reader) (*http.Request, error) {
	url := c.SiteURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", c.Identity.Cookies)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("User-Agent", "downleaf/0.1")
	return req, nil
}

// doJSON sends a request and decodes the JSON response into dst.
func (c *Client) doJSON(req *http.Request, dst interface{}) error {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", req.URL.Path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s returned %d: %s", req.URL.Path, resp.StatusCode, string(body))
	}

	if dst != nil {
		return json.NewDecoder(resp.Body).Decode(dst)
	}
	return nil
}

// ListProjects returns all projects for the authenticated user.
func (c *Client) ListProjects() ([]model.Project, error) {
	req, err := c.request("GET", "/user/projects", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Projects []model.Project `json:"projects"`
	}
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	return resp.Projects, nil
}

// GetProjectEntities returns the flat file tree for a project.
func (c *Client) GetProjectEntities(projectID string) ([]model.Entity, error) {
	req, err := c.request("GET", "/project/"+projectID+"/entities", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Entities []model.Entity `json:"entities"`
	}
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("get entities: %w", err)
	}

	return resp.Entities, nil
}

// DownloadFile downloads a binary file from the project.
func (c *Client) DownloadFile(projectID, fileID string) ([]byte, error) {
	req, err := c.request("GET", "/project/"+projectID+"/file/"+fileID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return nil, fmt.Errorf("download file returned %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// CreateDoc creates a new .tex document in the project.
func (c *Client) CreateDoc(projectID, name, parentFolderID string) error {
	payload := fmt.Sprintf(`{"name":%q,"parent_folder_id":%q,"_csrf":%q}`,
		name, parentFolderID, c.Identity.CSRFToken)

	req, err := c.request("POST", "/project/"+projectID+"/doc", strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doJSON(req, nil)
}

// CreateFolder creates a new folder in the project.
func (c *Client) CreateFolder(projectID, name, parentFolderID string) (*model.Folder, error) {
	payload := fmt.Sprintf(`{"name":%q,"parent_folder_id":%q,"_csrf":%q}`,
		name, parentFolderID, c.Identity.CSRFToken)

	req, err := c.request("POST", "/project/"+projectID+"/folder", strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	var folder model.Folder
	if err := c.doJSON(req, &folder); err != nil {
		return nil, fmt.Errorf("create folder: %w", err)
	}
	return &folder, nil
}

// DeleteEntity deletes a file, doc, or folder from the project.
// entityType should be "file", "doc", or "folder".
func (c *Client) DeleteEntity(projectID, entityType, entityID string) error {
	req, err := c.request("DELETE", "/project/"+projectID+"/"+entityType+"/"+entityID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Csrf-Token", c.Identity.CSRFToken)

	return c.doJSON(req, nil)
}

// RenameEntity renames a file, doc, or folder.
func (c *Client) RenameEntity(projectID, entityType, entityID, newName string) error {
	payload := fmt.Sprintf(`{"name":%q,"_csrf":%q}`, newName, c.Identity.CSRFToken)

	req, err := c.request("POST", "/project/"+projectID+"/"+entityType+"/"+entityID+"/rename",
		strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doJSON(req, nil)
}

// MoveEntity moves a file, doc, or folder to a new parent folder.
func (c *Client) MoveEntity(projectID, entityType, entityID, newFolderID string) error {
	payload := fmt.Sprintf(`{"folder_id":%q,"_csrf":%q}`, newFolderID, c.Identity.CSRFToken)

	req, err := c.request("POST", "/project/"+projectID+"/"+entityType+"/"+entityID+"/move",
		strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doJSON(req, nil)
}
