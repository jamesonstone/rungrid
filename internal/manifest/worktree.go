package manifest

// extractWorktreeDeclarations removes every service's worktree declaration
// from m and returns them by service name. A declaration selects a runtime
// override, so it must not change the normalized manifest or generation.
func extractWorktreeDeclarations(m *Manifest) map[string]string {
	declarations := map[string]string{}
	for index := range m.Services {
		if m.Services[index].Worktree != "" {
			declarations[m.Services[index].Name] = m.Services[index].Worktree
			m.Services[index].Worktree = ""
		}
	}
	return declarations
}
