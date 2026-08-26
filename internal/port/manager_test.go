package port

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestReconcileRedisDBsReleasesOnlyInactiveWorkspaces(t *testing.T) {
	registry := &Registry{
		allocations: make(map[string]Allocation),
		redisDBs: map[string]int{
			"active": 2,
			"stale":  3,
		},
		filePath: filepath.Join(t.TempDir(), "ports.json"),
	}

	registry.ReconcileRedisDBs(map[string]bool{"active": true})

	if db, ok := registry.GetRedisDB("active"); !ok || db != 2 {
		t.Fatalf("active Redis DB = %d, %v; want 2, true", db, ok)
	}
	if _, ok := registry.GetRedisDB("stale"); ok {
		t.Fatal("stale Redis DB allocation was not released")
	}
	db, err := registry.AllocateRedisDB("replacement")
	if err != nil {
		t.Fatal(err)
	}
	if db != 3 {
		t.Fatalf("replacement Redis DB = %d, want 3", db)
	}
}

func TestAllocateRedisDBReturnsErrorWhenPoolIsExhausted(t *testing.T) {
	registry := &Registry{
		allocations: make(map[string]Allocation),
		redisDBs:    make(map[string]int),
		filePath:    filepath.Join(t.TempDir(), "ports.json"),
	}
	for db := 2; db <= 15; db++ {
		registry.redisDBs[fmt.Sprintf("workspace-%d", db)] = db
	}

	if _, err := registry.AllocateRedisDB("overflow"); err == nil {
		t.Fatal("AllocateRedisDB succeeded with an exhausted pool")
	}
}
