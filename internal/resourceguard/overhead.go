package resourceguard

func treeOverhead(snapshot processSnapshot, rootPID int) (float64, uint64, int) {
	seen := map[int]bool{}
	pending := []int{rootPID}
	var cpu float64
	var rss uint64
	threads := 0
	for len(pending) > 0 {
		pid := pending[0]
		pending = pending[1:]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		current, exists := snapshot[pid]
		if !exists {
			continue
		}
		cpu += current.CPUPercent
		rss += current.RSSBytes
		threads += current.Threads
		for candidatePID, candidate := range snapshot {
			if candidate.PPID == pid {
				pending = append(pending, candidatePID)
			}
		}
	}
	return cpu, rss, threads
}
