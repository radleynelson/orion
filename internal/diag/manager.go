package diag

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Manager collects process-level memory stats for the Orion app and its
// descendants, plus per-tmux-session process trees.
type Manager struct {
	ctx context.Context
}

func NewManager() *Manager { return &Manager{} }

func (m *Manager) SetContext(ctx context.Context) { m.ctx = ctx }

// MemorySnapshot is the top-level payload returned to the frontend.
type MemorySnapshot struct {
	Go        GoStats        `json:"go"`
	Orion     ProcessStats   `json:"orion"`       // the Orion Go process itself
	Webview   []ProcessStats `json:"webview"`     // WebKit/helper processes spawned by Orion
	Helpers   []ProcessStats `json:"helpers"`     // other Orion descendants (ripgrep, tmux clients, etc.)
	Sessions  []SessionMem   `json:"sessions"`    // orion-* tmux sessions
	FDs       *FDStats       `json:"fds"`         // open file descriptors for Orion main
	Totals    Totals         `json:"totals"`
	Timestamp int64          `json:"timestamp"`
}

type GoStats struct {
	HeapAllocMB  float64 `json:"heapAllocMB"`
	HeapSysMB    float64 `json:"heapSysMB"`
	StackInUseMB float64 `json:"stackInUseMB"`
	SysMB        float64 `json:"sysMB"`
	NumGC        uint32  `json:"numGC"`
	NumGoroutine int     `json:"numGoroutine"`
}

type ProcessStats struct {
	PID    int     `json:"pid"`
	PPID   int     `json:"ppid"`
	Name   string  `json:"name"`
	RSSMB  float64 `json:"rssMB"`
	CPUPct float64 `json:"cpuPct"`
}

type SessionMem struct {
	SessionName string         `json:"sessionName"`
	Kind        string         `json:"kind"` // "server" | "shell" | "web" | "agent"
	PanePID     int            `json:"panePID"`
	Processes   []ProcessStats `json:"processes"`
	TotalRSSMB  float64        `json:"totalRSSMB"`
}

type Totals struct {
	OrionMB    float64 `json:"orionMB"`    // Orion main only
	WebviewMB  float64 `json:"webviewMB"`  // sum of WebKit helpers
	HelpersMB  float64 `json:"helpersMB"`  // sum of other descendants
	SessionsMB float64 `json:"sessionsMB"` // sum across all tmux sessions
	GrandMB    float64 `json:"grandMB"`    // everything above
}

// Snapshot runs a single `ps` + tmux query and returns the current state.
func (m *Manager) Snapshot() (*MemorySnapshot, error) {
	procs, err := snapshotProcs()
	if err != nil {
		return nil, err
	}

	orionPID := os.Getpid()
	children := buildChildIndex(procs)

	snap := &MemorySnapshot{
		Go:        readGoStats(),
		Timestamp: time.Now().UnixMilli(),
	}

	// Orion main process
	if p, ok := procs[orionPID]; ok {
		snap.Orion = toStats(p)
	} else {
		snap.Orion = ProcessStats{PID: orionPID, Name: "Orion"}
	}

	snap.FDs = SnapshotFDs(orionPID)

	// Pane PIDs for orion-* tmux sessions — we need these to avoid double-counting
	// descendants that are technically children of tmux (they aren't, since tmux
	// sessions live in the tmux server, but a pane PID can still show up in our
	// Orion descendant walk in rare setups — treat sessions as authoritative).
	sessionPanes := listOrionSessionPanes()
	paneRoots := make(map[int]bool)
	for _, sp := range sessionPanes {
		paneRoots[sp.panePID] = true
	}

	// Walk all Orion descendants (skip anything rooted at a tmux pane PID).
	descendants := walkDescendants(children, orionPID)
	for _, pid := range descendants {
		p, ok := procs[pid]
		if !ok {
			continue
		}
		stats := toStats(p)
		if isWebKitHelper(p.command) {
			snap.Webview = append(snap.Webview, stats)
		} else if !paneRoots[pid] {
			snap.Helpers = append(snap.Helpers, stats)
		}
	}

	// Build per-session trees.
	for _, sp := range sessionPanes {
		sess := SessionMem{
			SessionName: sp.sessionName,
			Kind:        classifySession(sp.sessionName),
			PanePID:     sp.panePID,
		}
		// The pane shell itself
		if p, ok := procs[sp.panePID]; ok {
			sess.Processes = append(sess.Processes, toStats(p))
		}
		// And its descendants
		for _, pid := range walkDescendants(children, sp.panePID) {
			if p, ok := procs[pid]; ok {
				sess.Processes = append(sess.Processes, toStats(p))
			}
		}
		for _, ps := range sess.Processes {
			sess.TotalRSSMB += ps.RSSMB
		}
		snap.Sessions = append(snap.Sessions, sess)
	}

	// Sort sessions biggest first
	sort.Slice(snap.Sessions, func(i, j int) bool {
		return snap.Sessions[i].TotalRSSMB > snap.Sessions[j].TotalRSSMB
	})
	sort.Slice(snap.Webview, func(i, j int) bool {
		return snap.Webview[i].RSSMB > snap.Webview[j].RSSMB
	})
	sort.Slice(snap.Helpers, func(i, j int) bool {
		return snap.Helpers[i].RSSMB > snap.Helpers[j].RSSMB
	})

	// Totals
	snap.Totals.OrionMB = snap.Orion.RSSMB
	for _, p := range snap.Webview {
		snap.Totals.WebviewMB += p.RSSMB
	}
	for _, p := range snap.Helpers {
		snap.Totals.HelpersMB += p.RSSMB
	}
	for _, s := range snap.Sessions {
		snap.Totals.SessionsMB += s.TotalRSSMB
	}
	snap.Totals.GrandMB = snap.Totals.OrionMB + snap.Totals.WebviewMB + snap.Totals.HelpersMB + snap.Totals.SessionsMB

	return snap, nil
}

// --- process table ---

type procInfo struct {
	pid, ppid int
	rssKB     int
	cpuPct    float64
	command   string
}

// snapshotProcs runs a single `ps` call and parses the result.
func snapshotProcs() (map[int]procInfo, error) {
	// `=` suppresses the column header on macOS/BSD ps.
	out, err := exec.Command("ps", "-Ao", "pid=,ppid=,rss=,%cpu=,command=").Output()
	if err != nil {
		return nil, err
	}
	procs := make(map[int]procInfo, 512)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		rss, err3 := strconv.Atoi(fields[2])
		cpu, err4 := strconv.ParseFloat(fields[3], 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			continue
		}
		cmd := strings.Join(fields[4:], " ")
		procs[pid] = procInfo{pid: pid, ppid: ppid, rssKB: rss, cpuPct: cpu, command: cmd}
	}
	return procs, nil
}

func buildChildIndex(procs map[int]procInfo) map[int][]int {
	idx := make(map[int][]int, len(procs))
	for pid, p := range procs {
		idx[p.ppid] = append(idx[p.ppid], pid)
	}
	return idx
}

// walkDescendants returns all pids below root (root itself excluded).
func walkDescendants(children map[int][]int, root int) []int {
	var out []int
	seen := make(map[int]bool)
	stack := append([]int(nil), children[root]...)
	for len(stack) > 0 {
		pid := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		out = append(out, pid)
		stack = append(stack, children[pid]...)
	}
	return out
}

func toStats(p procInfo) ProcessStats {
	return ProcessStats{
		PID:    p.pid,
		PPID:   p.ppid,
		Name:   friendlyName(p.command),
		RSSMB:  float64(p.rssKB) / 1024.0,
		CPUPct: p.cpuPct,
	}
}

func friendlyName(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	// Drop everything after the first space (args) to get just the executable
	first := cmd
	if idx := strings.IndexByte(cmd, ' '); idx > 0 {
		first = cmd[:idx]
	}
	base := filepath.Base(first)
	// Known long macOS helper paths: keep the .app or framework segment for clarity
	if strings.Contains(first, "WebKit.WebContent") {
		return "WebKit.WebContent"
	}
	if strings.Contains(first, "WebKit.Networking") {
		return "WebKit.Networking"
	}
	if strings.Contains(first, "WebKit.GPU") {
		return "WebKit.GPU"
	}
	return base
}

func isWebKitHelper(cmd string) bool {
	return strings.Contains(cmd, "com.apple.WebKit") ||
		strings.Contains(cmd, "WebKit.WebContent") ||
		strings.Contains(cmd, "WebKit.Networking") ||
		strings.Contains(cmd, "WebKit.GPU") ||
		strings.Contains(cmd, "Safari Web Content")
}

// --- tmux ---

type sessionPane struct {
	sessionName string
	panePID     int
}

func listOrionSessionPanes() []sessionPane {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{session_name} #{pane_pid}").Output()
	if err != nil {
		return nil
	}
	var result []sessionPane
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		if !strings.HasPrefix(name, "orion-") {
			continue
		}
		// Only take the first pane per session (they usually have one; if
		// there are multiple we still capture the first shell tree).
		if seen[name] {
			continue
		}
		seen[name] = true
		pid, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		result = append(result, sessionPane{sessionName: name, panePID: pid})
	}
	return result
}

func classifySession(name string) string {
	switch {
	case strings.HasPrefix(name, "orion-srv-"):
		return "server"
	case strings.HasPrefix(name, "orion-shell-"):
		return "shell"
	case strings.HasPrefix(name, "orion-web-"):
		return "web"
	default:
		return "agent"
	}
}

// --- runtime ---

func readGoStats() GoStats {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	const mb = 1024.0 * 1024.0
	return GoStats{
		HeapAllocMB:  float64(ms.HeapAlloc) / mb,
		HeapSysMB:    float64(ms.HeapSys) / mb,
		StackInUseMB: float64(ms.StackInuse) / mb,
		SysMB:        float64(ms.Sys) / mb,
		NumGC:        ms.NumGC,
		NumGoroutine: runtime.NumGoroutine(),
	}
}
