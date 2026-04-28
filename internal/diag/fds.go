package diag

import (
	"bufio"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// FDStats summarises the open file descriptors for the Orion process and how
// close we are to the per-process limit. macOS doesn't expose /proc, so we
// shell out to `lsof -p <pid>` and group the results by type.
type FDStats struct {
	Count       int           `json:"count"`
	SoftLimit   uint64        `json:"softLimit"`
	HardLimit   uint64        `json:"hardLimit"`
	UsagePct    float64       `json:"usagePct"`
	ByType      []FDTypeCount `json:"byType"`
	TopEntries  []FDEntry     `json:"topEntries"`
	GroupedDirs []FDDirCount  `json:"groupedDirs"`
	Truncated   bool          `json:"truncated"`
	Error       string        `json:"error,omitempty"`
}

type FDTypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type FDEntry struct {
	FD   string `json:"fd"`
	Type string `json:"type"`
	Name string `json:"name"`
}

// FDDirCount groups regular files by their parent directory so a dense tree
// shows up as one row instead of hundreds.
type FDDirCount struct {
	Dir   string `json:"dir"`
	Count int    `json:"count"`
}

const fdEntriesCap = 200

// SnapshotFDs collects FD info for the given pid. Returns a populated FDStats
// even on error (with Error set) so the UI can render something useful.
func SnapshotFDs(pid int) *FDStats {
	stats := &FDStats{}

	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err == nil {
		stats.SoftLimit = uint64(lim.Cur)
		stats.HardLimit = uint64(lim.Max)
	}

	cmd := exec.Command("lsof", "-p", strconv.Itoa(pid), "-F", "ftn")
	out, err := cmd.Output()
	if err != nil {
		stats.Error = err.Error()
		return stats
	}

	type entry struct{ fd, typ, name string }
	var entries []entry
	var cur entry
	flush := func() {
		if cur.fd != "" {
			entries = append(entries, cur)
		}
		cur = entry{}
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case 'p':
			// process line — ignore
		case 'f':
			flush()
			cur.fd = line[1:]
		case 't':
			cur.typ = line[1:]
		case 'n':
			cur.name = line[1:]
		}
	}
	flush()

	stats.Count = len(entries)
	if stats.SoftLimit > 0 {
		stats.UsagePct = float64(stats.Count) / float64(stats.SoftLimit) * 100
	}

	// Bucket by type.
	byType := make(map[string]int)
	dirCounts := make(map[string]int)
	for _, e := range entries {
		byType[e.typ]++
		if e.typ == "REG" || e.typ == "DIR" {
			dir := e.name
			if i := strings.LastIndex(dir, "/"); i > 0 {
				dir = dir[:i]
			}
			dirCounts[dir]++
		}
	}
	for typ, c := range byType {
		stats.ByType = append(stats.ByType, FDTypeCount{Type: typ, Count: c})
	}
	sort.Slice(stats.ByType, func(i, j int) bool { return stats.ByType[i].Count > stats.ByType[j].Count })

	for dir, c := range dirCounts {
		stats.GroupedDirs = append(stats.GroupedDirs, FDDirCount{Dir: dir, Count: c})
	}
	sort.Slice(stats.GroupedDirs, func(i, j int) bool { return stats.GroupedDirs[i].Count > stats.GroupedDirs[j].Count })
	if len(stats.GroupedDirs) > 30 {
		stats.GroupedDirs = stats.GroupedDirs[:30]
	}

	if len(entries) > fdEntriesCap {
		stats.Truncated = true
		entries = entries[:fdEntriesCap]
	}
	for _, e := range entries {
		stats.TopEntries = append(stats.TopEntries, FDEntry{FD: e.fd, Type: e.typ, Name: e.name})
	}
	return stats
}
