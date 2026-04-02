package credential

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Store manages credential files in ~/.downleaf/credentials/.
type Store struct {
	dir string
}

// NewStore creates a Store and ensures the credentials directory exists.
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".downleaf", "credentials")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create credentials dir: %w", err)
	}
	return &Store{dir: dir}, nil
}

// List returns summaries of all saved credentials, sorted by last used (most recent first).
func (s *Store) List() ([]CredentialInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read credentials dir: %w", err)
	}

	var infos []CredentialInfo
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-5] // strip .json
		cred, err := s.Load(id)
		if err != nil {
			continue // skip corrupted files
		}
		infos = append(infos, cred.Info())
	}

	// Sort by last used, most recent first
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].LastUsedAt.After(infos[j].LastUsedAt)
	})

	return infos, nil
}

// Save writes a credential to disk with secure permissions.
// If a credential with the same ID exists, it is overwritten.
func (s *Store) Save(cred *Credential) error {
	if cred.ID == "" {
		return fmt.Errorf("credential ID is required")
	}
	data, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credential: %w", err)
	}
	path := filepath.Join(s.dir, cred.ID+".json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write credential file: %w", err)
	}
	return nil
}

// Load reads a credential by ID.
func (s *Store) Load(id string) (*Credential, error) {
	path := filepath.Join(s.dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("credential not found: %s", id)
		}
		return nil, fmt.Errorf("read credential file: %w", err)
	}
	var cred Credential
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, fmt.Errorf("unmarshal credential: %w", err)
	}
	return &cred, nil
}

// Delete removes a credential by ID.
func (s *Store) Delete(id string) error {
	path := filepath.Join(s.dir, id+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil // already deleted
		}
		return fmt.Errorf("remove credential file: %w", err)
	}
	return nil
}

// UpdateLastUsed updates the LastUsedAt timestamp and saves.
func (s *Store) UpdateLastUsed(id string) error {
	cred, err := s.Load(id)
	if err != nil {
		return err
	}
	cred.LastUsedAt = time.Now()
	return s.Save(cred)
}
