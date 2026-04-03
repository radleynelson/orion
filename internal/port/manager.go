package port

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"sync"
)

const (
	portMin = 10000
	portMax = 60000
)

// Allocation maps server names to assigned ports for a workspace.
type Allocation map[string]int

// Registry manages port allocations across workspaces.
type Registry struct {
	// workspaceID -> serverName -> port
	allocations map[string]Allocation
	mu          sync.RWMutex
	filePath    string
}

// NewRegistry creates a port registry, loading from disk if available.
func NewRegistry() *Registry {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".orion")
	os.MkdirAll(dir, 0755)
	fp := filepath.Join(dir, "ports.json")

	r := &Registry{
		allocations: make(map[string]Allocation),
		filePath:    fp,
	}
	r.load()
	return r
}

// AllocateForWorkspace assigns ports for all servers in a workspace.
// Returns the allocation map (serverName -> port).
func (r *Registry) AllocateForWorkspace(wsID string, serverNames []string) (Allocation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// If already allocated, return existing
	if existing, ok := r.allocations[wsID]; ok {
		// Check if all servers are covered
		allCovered := true
		for _, name := range serverNames {
			if _, ok := existing[name]; !ok {
				allCovered = false
				break
			}
		}
		if allCovered {
			return existing, nil
		}
	}

	alloc := make(Allocation)
	usedPorts := r.allUsedPorts()

	for _, name := range serverNames {
		// Check if this server already has a port in existing allocation
		if existing, ok := r.allocations[wsID]; ok {
			if port, ok := existing[name]; ok {
				alloc[name] = port
				continue
			}
		}

		port, err := findAvailablePort(usedPorts)
		if err != nil {
			return nil, fmt.Errorf("failed to allocate port for %s: %w", name, err)
		}
		alloc[name] = port
		usedPorts[port] = true
	}

	r.allocations[wsID] = alloc
	r.save()
	return alloc, nil
}

// GetAllocation returns the port allocation for a workspace, or nil if none.
func (r *Registry) GetAllocation(wsID string) Allocation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.allocations[wsID]
}

// GetAllAllocations returns all workspace allocations.
func (r *Registry) GetAllAllocations() map[string]Allocation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]Allocation)
	for k, v := range r.allocations {
		result[k] = v
	}
	return result
}

// ReleaseWorkspace frees all ports for a workspace.
func (r *Registry) ReleaseWorkspace(wsID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.allocations, wsID)
	r.save()
}

func (r *Registry) allUsedPorts() map[int]bool {
	used := make(map[int]bool)
	for _, alloc := range r.allocations {
		for _, port := range alloc {
			used[port] = true
		}
	}
	return used
}

func findAvailablePort(used map[int]bool) (int, error) {
	// Try random ports
	for attempts := 0; attempts < 100; attempts++ {
		port := portMin + rand.Intn(portMax-portMin)
		if used[port] {
			continue
		}
		// Verify port is actually available on the system
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			continue
		}
		ln.Close()
		return port, nil
	}
	return 0, fmt.Errorf("could not find available port after 100 attempts")
}

func (r *Registry) load() {
	data, err := os.ReadFile(r.filePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &r.allocations)
}

func (r *Registry) save() {
	data, _ := json.MarshalIndent(r.allocations, "", "  ")
	os.WriteFile(r.filePath, data, 0644)
}
