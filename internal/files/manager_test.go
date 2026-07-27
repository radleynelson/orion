package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListDirectoryShowsVendorButSkipsVendorBundle(t *testing.T) {
	root := t.TempDir()
	backend := filepath.Join(root, "backend")
	mustMkdirAll(t, filepath.Join(backend, "vendor", "gems", "local_engine"))
	mustMkdirAll(t, filepath.Join(backend, "vendor", "bundle", "ruby"))
	mustMkdirAll(t, filepath.Join(backend, "node_modules", "package"))

	manager := NewManager()
	entries, err := manager.ListDirectory(backend, 0)
	if err != nil {
		t.Fatalf("ListDirectory backend: %v", err)
	}

	if !hasEntry(entries, "vendor") {
		t.Fatalf("expected vendor to be visible in backend listing, got %#v", entryNames(entries))
	}
	if hasEntry(entries, "node_modules") {
		t.Fatalf("expected node_modules to stay hidden, got %#v", entryNames(entries))
	}

	vendorEntries, err := manager.ListDirectory(filepath.Join(backend, "vendor"), 0)
	if err != nil {
		t.Fatalf("ListDirectory vendor: %v", err)
	}

	if !hasEntry(vendorEntries, "gems") {
		t.Fatalf("expected vendor/gems to be visible, got %#v", entryNames(vendorEntries))
	}
	if hasEntry(vendorEntries, "bundle") {
		t.Fatalf("expected vendor/bundle to stay hidden, got %#v", entryNames(vendorEntries))
	}
}

func TestSearchFilesIncludesVendorGemsButSkipsVendorBundle(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "backend", "vendor", "gems", "local_engine", "app", "models", "engine_widget.rb"))
	mustWriteFile(t, filepath.Join(root, "backend", "vendor", "bundle", "ruby", "gems", "hidden_widget.rb"))

	manager := NewManager()
	results, err := manager.SearchFiles(root, "widget", 20)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}

	if !hasSearchPath(results, filepath.Join("backend", "vendor", "gems", "local_engine", "app", "models", "engine_widget.rb")) {
		t.Fatalf("expected search to include vendor/gems file, got %#v", searchPaths(results))
	}
	if hasSearchPath(results, filepath.Join("backend", "vendor", "bundle", "ruby", "gems", "hidden_widget.rb")) {
		t.Fatalf("expected search to skip vendor/bundle file, got %#v", searchPaths(results))
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("MkdirAll %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func hasEntry(entries []FileEntry, name string) bool {
	for _, entry := range entries {
		if entry.Name == name {
			return true
		}
	}
	return false
}

func entryNames(entries []FileEntry) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name
	}
	return names
}

func hasSearchPath(results []SearchResult, path string) bool {
	for _, result := range results {
		if result.Path == path {
			return true
		}
	}
	return false
}

func searchPaths(results []SearchResult) []string {
	paths := make([]string, len(results))
	for i, result := range results {
		paths[i] = result.Path
	}
	return paths
}
