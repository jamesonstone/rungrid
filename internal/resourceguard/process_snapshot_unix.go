//go:build darwin || linux

package resourceguard

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/guardstate"
)

const maxSnapshotBytes = 8 << 20

type processInfo struct {
	PID             int
	PPID            int
	PGID            int
	StartIdentity   string
	CPUTotalSeconds float64
	CPUPercent      float64
	RSSBytes        uint64
	Threads         int
}

type processSnapshot map[int]processInfo

type processCandidate struct {
	Root    processInfo
	Tree    map[int]processInfo
	Groups  []int
	Metrics guardstate.Metrics
}

func captureProcesses(ctx context.Context, hostMemory uint64) (processSnapshot, error) {
	threadRows := runtime.GOOS == "darwin"
	arguments := []string{"-axo", "pid=,ppid=,pgid=,lstart=,time=,%cpu=,rss=,nlwp="}
	if threadRows {
		// Darwin ps exposes thread counts only by emitting one row per thread.
		// Parse from the fixed custom columns at the right edge and aggregate
		// duplicate PIDs without collecting the command text.
		arguments = []string{"-M", "-axo", "pid=,ppid=,pgid=,lstart=,time=,%cpu=,rss="}
	}
	command := exec.CommandContext(ctx, "ps", arguments...)
	command.Env = append(localeNeutralEnvironment(os.Environ()), "LC_ALL=C")
	output := &boundedBuffer{limit: maxSnapshotBytes}
	command.Stdout = output
	command.Stderr = &boundedBuffer{limit: 64 << 10}
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("capture process snapshot: %w", err)
	}
	return parseProcessSnapshot(output.Bytes(), hostMemory, threadRows)
}

func parseProcessSnapshot(content []byte, hostMemory uint64, threadRows bool) (processSnapshot, error) {
	result := processSnapshot{}
	for lineNumber, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		expected := 12
		if threadRows {
			expected = 11
			if len(fields) < expected {
				continue
			}
			fields = fields[len(fields)-expected:]
		}
		if len(fields) != expected {
			return nil, fmt.Errorf("parse process snapshot line %d", lineNumber+1)
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			if threadRows {
				continue
			}
			return nil, fmt.Errorf("parse process PID on line %d", lineNumber+1)
		}
		ppid, ppidErr := strconv.Atoi(fields[1])
		pgid, pgidErr := strconv.Atoi(fields[2])
		cpuTotal, totalErr := parseCPUTime(fields[8])
		cpu, cpuErr := strconv.ParseFloat(fields[9], 64)
		rssKB, rssErr := strconv.ParseUint(fields[10], 10, 64)
		threads, threadsErr := 1, error(nil)
		if !threadRows {
			threads, threadsErr = strconv.Atoi(fields[11])
		}
		if ppidErr != nil || pgidErr != nil || totalErr != nil || cpuErr != nil || rssErr != nil || threadsErr != nil {
			return nil, fmt.Errorf("parse process metrics on line %d", lineNumber+1)
		}
		current := processInfo{
			PID: pid, PPID: ppid, PGID: pgid, StartIdentity: strings.Join(fields[3:8], " "),
			CPUTotalSeconds: cpuTotal, CPUPercent: cpu, RSSBytes: rssKB * 1024, Threads: threads,
		}
		if existing, exists := result[pid]; threadRows && exists {
			if existing.PPID != current.PPID || existing.PGID != current.PGID || existing.StartIdentity != current.StartIdentity {
				return nil, fmt.Errorf("inconsistent Darwin thread identity on line %d", lineNumber+1)
			}
			existing.CPUPercent += current.CPUPercent
			existing.CPUTotalSeconds = max(existing.CPUTotalSeconds, current.CPUTotalSeconds)
			existing.Threads++
			result[pid] = existing
		} else {
			result[pid] = current
		}
	}
	if hostMemory == 0 {
		return nil, fmt.Errorf("host memory is unavailable")
	}
	return result, nil
}

func buildCandidate(snapshot processSnapshot, rootPID, runtimePID int, hostMemory uint64) (processCandidate, error) {
	if !descendsFrom(snapshot, rootPID, runtimePID) {
		return processCandidate{}, fmt.Errorf("managed root does not descend from the recorded runtime")
	}
	return buildTreeCandidate(snapshot, rootPID, hostMemory)
}

func buildControlCandidate(snapshot processSnapshot, registration guardstate.ControlClient, hostMemory uint64) (processCandidate, error) {
	root, exists := snapshot[registration.PID]
	parent, parentExists := snapshot[registration.ParentPID]
	if !exists || !parentExists || root.PPID != registration.ParentPID || root.PGID != registration.PGID ||
		root.StartIdentity != registration.ProcessIdentity || parent.StartIdentity != registration.ParentIdentity {
		return processCandidate{}, fmt.Errorf("control client ancestry or process identity does not match its registration")
	}
	return buildTreeCandidate(snapshot, registration.PID, hostMemory)
}

func buildTreeCandidate(snapshot processSnapshot, rootPID int, hostMemory uint64) (processCandidate, error) {
	root, exists := snapshot[rootPID]
	if !exists || root.PID <= 1 || root.PGID != root.PID {
		return processCandidate{}, fmt.Errorf("managed root is missing or lacks an isolated process group")
	}
	tree := map[int]processInfo{}
	pending := []int{root.PID}
	for len(pending) > 0 {
		pid := pending[0]
		pending = pending[1:]
		if _, seen := tree[pid]; seen {
			continue
		}
		current, found := snapshot[pid]
		if !found {
			continue
		}
		tree[pid] = current
		for candidatePID, candidate := range snapshot {
			if candidate.PPID == pid {
				pending = append(pending, candidatePID)
			}
		}
	}
	groups := map[int]bool{}
	metrics := guardstate.Metrics{Processes: len(tree)}
	for _, process := range tree {
		if process.PGID <= 1 {
			return processCandidate{}, fmt.Errorf("managed tree contains an unsafe process group")
		}
		groups[process.PGID] = true
		metrics.CPUPercent += process.CPUPercent / float64(runtime.NumCPU())
		metrics.CPUTotalSeconds += process.CPUTotalSeconds
		metrics.RSSBytes += process.RSSBytes
		metrics.Threads += process.Threads
	}
	for _, process := range snapshot {
		if groups[process.PGID] {
			if _, owned := tree[process.PID]; !owned {
				return processCandidate{}, fmt.Errorf("managed process group contains an unowned member")
			}
		}
	}
	metrics.MemoryPercent = float64(metrics.RSSBytes) * 100 / float64(hostMemory)
	groupList := make([]int, 0, len(groups))
	for group := range groups {
		groupList = append(groupList, group)
	}
	return processCandidate{Root: root, Tree: tree, Groups: groupList, Metrics: metrics}, nil
}

func parseCPUTime(value string) (float64, error) {
	days := 0.0
	clock := value
	if rawDays, remainder, found := strings.Cut(value, "-"); found {
		parsed, err := strconv.ParseFloat(rawDays, 64)
		if err != nil {
			return 0, err
		}
		days, clock = parsed, remainder
	}
	parts := strings.Split(clock, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid CPU time")
	}
	seconds := days * 86400
	for index, part := range parts {
		parsed, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return 0, err
		}
		switch len(parts) - index - 1 {
		case 2:
			seconds += parsed * 3600
		case 1:
			seconds += parsed * 60
		default:
			seconds += parsed
		}
	}
	return seconds, nil
}

func descendsFrom(snapshot processSnapshot, pid, ancestor int) bool {
	seen := map[int]bool{}
	for pid > 1 && !seen[pid] {
		seen[pid] = true
		process, exists := snapshot[pid]
		if !exists {
			return false
		}
		if process.PPID == ancestor {
			return true
		}
		pid = process.PPID
	}
	return false
}

func localeNeutralEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, value := range environment {
		if !strings.HasPrefix(value, "LC_ALL=") && !strings.HasPrefix(value, "LANG=") {
			result = append(result, value)
		}
	}
	return result
}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(content []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		return 0, fmt.Errorf("process snapshot exceeded %d bytes", b.limit)
	}
	if len(content) > remaining {
		_, _ = b.Buffer.Write(content[:remaining])
		return remaining, fmt.Errorf("process snapshot exceeded %d bytes", b.limit)
	}
	return b.Buffer.Write(content)
}

func snapshotContext(parent context.Context, interval time.Duration) (context.Context, context.CancelFunc) {
	timeout := 2 * interval
	if timeout < 500*time.Millisecond {
		timeout = 500 * time.Millisecond
	}
	if timeout > 2*time.Second {
		timeout = 2 * time.Second
	}
	return context.WithTimeout(parent, timeout)
}
