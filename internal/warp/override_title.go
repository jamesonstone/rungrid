package warp

import (
	"path/filepath"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/jamesonstone/rungrid/internal/state"
)

// overrideTitles marks the installed tab of every service that runs from a
// repository override. Generated templates stay undecorated because they are
// part of the deterministic generation; only the installed copy changes.
func overrideTitles(layout state.Layout, m *manifest.Manifest, generationID string, templates []Template) []Template {
	entries, err := override.Load(layout, generationID)
	if err != nil || len(entries) == 0 {
		return templates
	}
	labels := map[string]string{}
	for _, entry := range entries {
		label := entry.Branch
		if label == "" {
			label = filepath.Base(entry.Path)
		}
		for _, service := range entry.Services {
			labels[service] = label
		}
	}
	result := make([]Template, len(templates))
	for index, template := range templates {
		result[index] = template
		if label, exists := labels[template.Service]; exists && template.Service != "" {
			result[index] = newTemplate(template.TabName, template.Title+" (override: "+label+")", template.Service, m.Project.ID, generationID)
		}
	}
	return result
}
