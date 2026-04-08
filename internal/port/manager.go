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

// registryData is the JSON structure persisted to ports.json.
type registryData struct {
	Allocations map[string]Allocation `json:"allocations"`
	RedisDBs    map[string]int        `json:"redisDBs,omitempty"`
}

// Registry manages port and Redis DB allocations across workspaces.
type Registry struct {
	allocations map[string]Allocation
	redisDBs    map[string]int // workspaceID -> Redis DB number (2-15)
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
		redisDBs:    make(map[string]int),
		filePath:    fp,
	}
	r.load()
	return r
}

// --- Port allocation ---

// AllocateForWorkspace assigns ports for all servers in a workspace.
func (r *Registry) AllocateForWorkspace(wsID string, serverNames []string) (Allocation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.allocations[wsID]; ok {
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

func (r *Registry) GetAllocation(wsID string) Allocation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.allocations[wsID]
}

func (r *Registry) GetAllAllocations() map[string]Allocation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]Allocation)
	for k, v := range r.allocations {
		result[k] = v
	}
	return result
}

func (r *Registry) ReleaseWorkspace(wsID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.allocations, wsID)
	r.save()
}

// --- Redis DB allocation ---

// AllocateRedisDB assigns a Redis DB number (2-15) for a workspace.
// Main workspace uses DB 1 by convention and should NOT call this.
func (r *Registry) AllocateRedisDB(wsID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Already allocated
	if db, ok := r.redisDBs[wsID]; ok {
		return db, nil
	}

	// Find used DBs
	usedDBs := make(map[int]bool)
	for _, db := range r.redisDBs {
		usedDBs[db] = true
	}

	// Allocate from 2-15 (0=production, 1=dev main)
	for db := 2; db <= 15; db++ {
		if !usedDBs[db] {
			r.redisDBs[wsID] = db
			r.save()
			return db, nil
		}
	}

	return 0, fmt.Errorf("all Redis DBs (2-15) are in use")
}

// GetRedisDB returns the allocated Redis DB for a workspace.
func (r *Registry) GetRedisDB(wsID string) (int, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	db, ok := r.redisDBs[wsID]
	return db, ok
}

// ReleaseRedisDB frees the Redis DB for a workspace.
func (r *Registry) ReleaseRedisDB(wsID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.redisDBs, wsID)
	r.save()
}

// --- internal ---

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
	for attempts := 0; attempts < 100; attempts++ {
		port := portMin + rand.Intn(portMax-portMin)
		if used[port] {
			continue
		}
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
	// Try new format first
	var rd registryData
	if err := json.Unmarshal(data, &rd); err == nil && rd.Allocations != nil {
		r.allocations = rd.Allocations
		if rd.RedisDBs != nil {
			r.redisDBs = rd.RedisDBs
		}
		return
	}
	// Fall back to old format (just allocations map)
	json.Unmarshal(data, &r.allocations)
}

func (r *Registry) save() {
	rd := registryData{
		Allocations: r.allocations,
		RedisDBs:    r.redisDBs,
	}
	data, _ := json.MarshalIndent(rd, "", "  ")
	os.WriteFile(r.filePath, data, 0644)
}
