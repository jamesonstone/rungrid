package warp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

const marker = "# rungrid-managed-tab-config"

type Template struct {
	Filename string
	TabName  string
	Title    string
	Service  string
	Content  []byte
}

type InstallRecord struct {
	APIVersion   string          `json:"api_version"`
	ProjectID    string          `json:"project_id"`
	GenerationID string          `json:"generation_id"`
	Artifacts    []InstalledFile `json:"artifacts"`
}

type InstalledFile struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Tab     string `json:"tab"`
	Service string `json:"service,omitempty"`
}

type renderedFile struct {
	template Template
	target   string
	content  []byte
}

func Templates(m *manifest.Manifest, generationID string) []Template {
	prefix := "rungrid_" + strings.ReplaceAll(m.Project.ID, "-", "_")
	result := []Template{
		newTemplate(prefix+"_00_overview", "Overview", "", m.Project.ID, generationID),
		newTemplate(prefix+"_01_versions", "Versions", "", m.Project.ID, generationID),
	}
	index := 2
	for _, service := range m.Services {
		if service.Activation != "tab" || service.Source == "external" {
			continue
		}
		tabName := fmt.Sprintf("%s_%02d_%s", prefix, index, strings.ReplaceAll(service.Name, "-", "_"))
		result = append(result, newTemplate(tabName, service.Terminal.Title, service.Name, m.Project.ID, generationID))
		index++
	}
	return result
}

func newTemplate(tabName, title, service, projectID, generationID string) Template {
	content := fmt.Sprintf(`%s
# project-id: %s
# generation-id: %s
name = %s
title = %s
color = "green"

[[panes]]
id = "main"
type = "terminal"
directory = "."
shell = "zsh"
commands = ["@@COMMAND@@"]
is_focused = true
`, marker, projectID, generationID, strconv.Quote(tabName), strconv.Quote(title))
	return Template{Filename: tabName + ".toml.tmpl", TabName: tabName, Title: title, Service: service, Content: []byte(content)}
}

func Install(layout state.Layout, m *manifest.Manifest, generationID, rungridExecutable string) (InstallRecord, error) {
	if err := layout.Ensure(); err != nil {
		return InstallRecord{}, err
	}
	templates := overrideTitles(layout, m, generationID, Templates(m, generationID))
	destination, err := tabConfigDirectory()
	if err != nil {
		return InstallRecord{}, err
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return InstallRecord{}, errs.Wrap(errs.ExitFailure, "RG901", "create Warp Tab Config directory", err)
	}
	previous, previousErr := readInstallRecord(layout)
	if previousErr != nil && !os.IsNotExist(previousErr) {
		return InstallRecord{}, previousErr
	}
	if previousErr == nil {
		if err := verifyInstalled(previous); err != nil {
			return InstallRecord{}, err
		}
	}

	record := InstallRecord{APIVersion: "rungrid/output/v1", ProjectID: layout.ProjectID, GenerationID: generationID}
	expected := map[string]bool{}
	rendered := make([]renderedFile, 0, len(templates))
	for _, template := range templates {
		command := commandFor(template, layout, rungridExecutable, m.Project.ID, generationID)
		content := []byte(strings.ReplaceAll(string(template.Content), "@@COMMAND@@", escapeTOML(command)))
		target := filepath.Join(destination, strings.TrimSuffix(template.Filename, ".tmpl"))
		expected[target] = true
		if _, err := os.Lstat(target); err == nil {
			prior, found := findInstalled(previous, target)
			if !found {
				return InstallRecord{}, errs.New(errs.ExitConflict, "RG902", "refusing to replace an unrecorded Warp Tab Config: "+target)
			}
			if err := verifyInstalledFile(prior); err != nil {
				return InstallRecord{}, err
			}
		} else if !os.IsNotExist(err) {
			return InstallRecord{}, errs.Wrap(errs.ExitConflict, "RG903", "inspect Warp Tab Config", err)
		}
		rendered = append(rendered, renderedFile{template: template, target: target, content: content})
	}
	if previousErr == nil {
		for _, old := range previous.Artifacts {
			if expected[old.Path] {
				continue
			}
			if err := verifyInstalledFile(old); err != nil {
				return InstallRecord{}, err
			}
		}
	}
	for _, item := range rendered {
		if err := atomicWrite(item.target, item.content, 0o600); err != nil {
			return InstallRecord{}, err
		}
		record.Artifacts = append(record.Artifacts, InstalledFile{Path: item.target, SHA256: hash(item.content), Tab: item.template.TabName, Service: item.template.Service})
	}
	if previousErr == nil {
		for _, old := range previous.Artifacts {
			if expected[old.Path] {
				continue
			}
			if err := os.Remove(old.Path); err != nil && !os.IsNotExist(err) {
				return InstallRecord{}, errs.Wrap(errs.ExitConflict, "RG904", "remove stale owned Warp Tab Config", err)
			}
		}
	}
	content, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return InstallRecord{}, errs.Wrap(errs.ExitFailure, "RG905", "encode Warp install ownership", err)
	}
	if err := state.WriteFileAtomic(layout.ProjectDir, "terminal-install.json", append(content, '\n'), 0o600); err != nil {
		return InstallRecord{}, err
	}
	return record, nil
}
