package workspacekey

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const metadataVersion = 1

type Metadata struct {
	Version          int    `json:"version"`
	ID               string `json:"id"`
	Name             string `json:"name"`
	EnvironmentName  string `json:"environmentName"`
	Branch           string `json:"branch"`
	BaseRef          string `json:"baseRef,omitempty"`
	MainWorktreePath string `json:"mainWorktreePath"`
	ManagedBy        string `json:"managedBy"`
	Provisioned      bool   `json:"provisioned"`
	ProvisionedAt    string `json:"provisionedAt,omitempty"`
}

func ID(workspacePath string) string {
	if metadata, ok := Load(workspacePath); ok && strings.TrimSpace(metadata.ID) != "" {
		return metadata.ID
	}
	return filepath.Base(filepath.Clean(workspacePath))
}

func Load(workspacePath string) (Metadata, bool) {
	data, err := os.ReadFile(metadataPath(workspacePath))
	if err != nil {
		return Metadata{}, false
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil || metadata.Version != metadataVersion {
		return Metadata{}, false
	}
	return metadata, true
}

func Save(workspacePath string, metadata Metadata) error {
	metadata.Version = metadataVersion
	if metadata.Provisioned && metadata.ProvisionedAt == "" {
		metadata.ProvisionedAt = time.Now().UTC().Format(time.RFC3339)
	}
	dir := filepath.Join(workspacePath, ".orion")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(metadataPath(workspacePath), data, 0644)
}

func metadataPath(workspacePath string) string {
	return filepath.Join(workspacePath, ".orion", "workspace.json")
}
