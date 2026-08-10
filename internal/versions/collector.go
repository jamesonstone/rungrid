package versions

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/serviceexec"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

const (
	sourceRefreshInterval   = 10 * time.Second
	listenerRefreshInterval = 5 * time.Second
	externalRefreshInterval = 5 * time.Second
)

type Collector struct {
	now               func() time.Time
	captureSource     func(context.Context, string, string) sourceVersion
	captureListeners  func(context.Context, []int) map[int][]int
	checkExternal     func(context.Context, *manifest.Manifest, string, *manifest.Service) error
	sources           map[string]cachedSource
	listeners         map[int][]int
	listenerPIDs      string
	listenersCaptured time.Time
	externals         map[string]cachedExternal
}

type cachedSource struct {
	value      sourceVersion
	capturedAt time.Time
}

type cachedExternal struct {
	ready      bool
	capturedAt time.Time
}

func NewCollector() *Collector {
	return &Collector{
		now:              time.Now,
		captureSource:    captureGitVersion,
		captureListeners: listeningPortsByPID,
		checkExternal:    serviceexec.CheckExternal,
		sources:          map[string]cachedSource{},
		listeners:        map[int][]int{},
		externals:        map[string]cachedExternal{},
	}
}

func (c *Collector) Capture(ctx context.Context, m *manifest.Manifest, runtimeState supervisor.Runtime, client processcompose.Client) Snapshot {
	now := c.now().UTC()
	result := Snapshot{CapturedAt: now.Format(time.RFC3339), Runtime: "running", Generation: runtimeState.GenerationID}
	states, _, err := client.List(ctx)
	if err != nil {
		result.Runtime = "unavailable"
	}
	byName := make(map[string]processcompose.ProcessState, len(states))
	pids := make([]int, 0, len(states))
	for _, state := range states {
		byName[state.Name] = state
		if state.PID > 0 {
			pids = append(pids, state.PID)
		}
	}
	c.refreshListeners(ctx, now, pids)
	for i := range m.Services {
		service := &m.Services[i]
		if service.Terminal.IncludeInVersions != nil && !*service.Terminal.IncludeInVersions {
			continue
		}
		item := c.captureService(ctx, now, m, runtimeState, service, byName)
		result.Services = append(result.Services, item)
	}
	return result
}

func (c *Collector) captureService(ctx context.Context, now time.Time, m *manifest.Manifest, runtimeState supervisor.Runtime, service *manifest.Service, states map[string]processcompose.ProcessState) ServiceVersion {
	repository := service.Repository
	if repository == "" {
		repository = manifest.WorkspaceRepository
	}
	item := ServiceVersion{Name: service.Name, Repository: repository, State: "unknown", GitState: "unavailable", Ports: append([]int(nil), service.Ports...)}
	if service.Source == "external" {
		ready := c.externalReady(ctx, now, m, runtimeState.WorkspaceRoot, service)
		item.State, item.Health = "external-unavailable", "unhealthy"
		if ready {
			item.State, item.Health = "external-ready", "healthy"
		}
	} else if state, exists := states[service.Name]; exists {
		item.State, item.Health, item.PID = state.Status, state.Health, state.PID
		if ports := c.listeners[state.PID]; len(ports) > 0 {
			item.Ports = append([]int(nil), ports...)
		}
	}
	if directory, err := manifest.ServiceWorkingDirectory(m, runtimeState.WorkspaceRoot, service); err == nil {
		source := c.source(ctx, now, directory)
		item.Branch, item.Commit, item.GitState, item.Worktree = source.branch, source.commit, source.gitState, source.worktree
	}
	return item
}

func (c *Collector) source(ctx context.Context, now time.Time, directory string) sourceVersion {
	cached, exists := c.sources[directory]
	if exists && now.Sub(cached.capturedAt) < sourceRefreshInterval {
		return cached.value
	}
	value := c.captureSource(ctx, directory, cached.value.root)
	c.sources[directory] = cachedSource{value: value, capturedAt: now}
	return value
}

func (c *Collector) externalReady(ctx context.Context, now time.Time, m *manifest.Manifest, root string, service *manifest.Service) bool {
	cached, exists := c.externals[service.Name]
	if exists && now.Sub(cached.capturedAt) < externalRefreshInterval {
		return cached.ready
	}
	ready := c.checkExternal(ctx, m, root, service) == nil
	c.externals[service.Name] = cachedExternal{ready: ready, capturedAt: now}
	return ready
}

func (c *Collector) refreshListeners(ctx context.Context, now time.Time, pids []int) {
	sort.Ints(pids)
	parts := make([]string, len(pids))
	for i, pid := range pids {
		parts[i] = strconv.Itoa(pid)
	}
	key := strings.Join(parts, ",")
	if key == c.listenerPIDs && !c.listenersCaptured.IsZero() && now.Sub(c.listenersCaptured) < listenerRefreshInterval {
		return
	}
	c.listeners = c.captureListeners(ctx, pids)
	c.listenerPIDs, c.listenersCaptured = key, now
}
