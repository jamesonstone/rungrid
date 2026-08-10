package versions

import (
	"context"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/jamesonstone/rungrid/internal/subprocess"
)

func listeningPortsByPID(ctx context.Context, pids []int) map[int][]int {
	result := make(map[int][]int, len(pids))
	if len(pids) == 0 {
		return result
	}
	parts := make([]string, len(pids))
	for i, pid := range pids {
		parts[i] = strconv.Itoa(pid)
	}
	command := exec.CommandContext(ctx, "lsof", "-nP", "-a", "-p", strings.Join(parts, ","), "-iTCP", "-sTCP:LISTEN", "-F", "pn")
	capture, err := subprocess.Run(command)
	if err != nil {
		return result
	}
	return parseListeningPorts(string(capture.Stdout))
}

func parseListeningPorts(content string) map[int][]int {
	seen := map[int]map[int]bool{}
	currentPID := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "p") {
			currentPID, _ = strconv.Atoi(strings.TrimPrefix(line, "p"))
			continue
		}
		if currentPID <= 0 || !strings.HasPrefix(line, "n") {
			continue
		}
		address := strings.TrimPrefix(line, "n")
		index := strings.LastIndexByte(address, ':')
		if index < 0 {
			continue
		}
		portText := address[index+1:]
		if end := strings.IndexByte(portText, '-'); end >= 0 {
			portText = portText[:end]
		}
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		if seen[currentPID] == nil {
			seen[currentPID] = map[int]bool{}
		}
		seen[currentPID][port] = true
	}
	result := make(map[int][]int, len(seen))
	for pid, ports := range seen {
		for port := range ports {
			result[pid] = append(result[pid], port)
		}
		sort.Ints(result[pid])
	}
	return result
}
