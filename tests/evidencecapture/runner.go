//go:build darwin || linux

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const hardOutputLimit int64 = 64 << 20

type runnerOptions struct {
	repositoryRoot string
	evidenceRoot   string
	outputLimit    int64
	command        []string
}

type commandOutcome struct {
	exitCode    int
	failureKind string
	signal      string
}

type testResult struct {
	SchemaVersion    string   `json:"schema_version"`
	Project          string   `json:"project"`
	Suite            string   `json:"suite"`
	TestID           string   `json:"test_id"`
	Environment      string   `json:"environment"`
	RunID            string   `json:"run_id"`
	RunNumber        int      `json:"run_number"`
	StartedAt        string   `json:"started_at"`
	FinishedAt       string   `json:"finished_at"`
	DurationSeconds  int64    `json:"duration_seconds"`
	Result           string   `json:"result"`
	ExitCode         int      `json:"exit_code"`
	FailureKind      string   `json:"failure_kind"`
	Signal           string   `json:"signal,omitempty"`
	SourceCommit     string   `json:"source_commit"`
	SourceTree       string   `json:"source_tree"`
	DeployedVersion  string   `json:"deployed_version"`
	TargetIdentity   string   `json:"target_identity"`
	AssertionSummary string   `json:"assertion_summary"`
	CleanupStatus    string   `json:"cleanup_status"`
	OutputBytes      int64    `json:"output_bytes"`
	OutputLimitBytes int64    `json:"output_limit_bytes"`
	OutputTruncated  bool     `json:"output_truncated"`
	Artifacts        []string `json:"artifacts"`
}

type boundedFile struct {
	mu        sync.Mutex
	file      *os.File
	limit     int64
	written   int64
	truncated bool
	overflow  chan struct{}
	once      sync.Once
}

func (o runnerOptions) validate() error {
	if o.repositoryRoot == "" || o.evidenceRoot == "" || len(o.command) == 0 {
		return fmt.Errorf("repository root, evidence root, and command are required")
	}
	if o.outputLimit < 1 || o.outputLimit > hardOutputLimit {
		return fmt.Errorf("output limit must be between 1 and %d bytes", hardOutputLimit)
	}
	return nil
}

func runEvidence(options runnerOptions) (testResult, string, error) {
	started := time.Now().UTC()
	runNumber, runDirectory, err := reserveRunDirectory(options.evidenceRoot)
	if err != nil {
		return testResult{}, "", err
	}
	outputPath := filepath.Join(runDirectory, "output.txt")
	output, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return testResult{}, runDirectory, err
	}
	bounded := &boundedFile{file: output, limit: options.outputLimit, overflow: make(chan struct{})}
	outcome := runCommand(options, bounded)
	closeErr := output.Close()
	finished := time.Now().UTC()
	result := newResult(options, runNumber, started, finished, outcome, bounded)
	if err := writeResult(filepath.Join(runDirectory, "result.json"), result); err != nil {
		return result, runDirectory, err
	}
	if closeErr != nil {
		return result, runDirectory, closeErr
	}
	return result, runDirectory, nil
}

func newResult(options runnerOptions, runNumber int, started, finished time.Time, outcome commandOutcome, output *boundedFile) testResult {
	result := "PASS"
	assertion := "Headless lifecycle, runtime identity, session quiescence, emergency and sustained resource containment, circuit reset, external-process survival, and exact shutdown passed."
	if outcome.exitCode != 0 {
		result = "FAIL"
		assertion = "The Go end-to-end test failed; inspect output.txt."
	}
	return testResult{
		SchemaVersion: "rungrid/test-result/v1", Project: "rungrid", Suite: "headless lifecycle",
		TestID: "rungrid-headless-e2e", Environment: "local", RunID: runID(started), RunNumber: runNumber,
		StartedAt: started.Format(time.RFC3339), FinishedAt: finished.Format(time.RFC3339),
		DurationSeconds: int64(finished.Sub(started).Seconds()), Result: result, ExitCode: outcome.exitCode,
		FailureKind: outcome.failureKind, Signal: outcome.signal, SourceCommit: gitValue(options.repositoryRoot, "rev-parse", "HEAD"),
		SourceTree: sourceTree(options.repositoryRoot), DeployedVersion: "NOT_APPLICABLE",
		TargetIdentity: "temporary XDG state with local Process Compose", AssertionSummary: assertion,
		CleanupStatus: "asserted by the test and temporary-directory cleanup", OutputBytes: output.size(),
		OutputLimitBytes: options.outputLimit, OutputTruncated: output.wasTruncated(),
		Artifacts: []string{"output.txt", "result.json"},
	}
}

func reserveRunDirectory(root string) (int, string, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return 0, "", err
	}
	for number := 1; ; number++ {
		directory := filepath.Join(root, strconv.Itoa(number))
		if err := os.Mkdir(directory, 0o700); err == nil {
			return number, directory, nil
		} else if !os.IsExist(err) {
			return 0, "", err
		}
	}
}

func (b *boundedFile) Write(content []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(content)
	remaining := b.limit - b.written
	if remaining > 0 {
		writeLength := int64(len(content))
		if writeLength > remaining {
			writeLength = remaining
		}
		written, err := b.file.Write(content[:writeLength])
		b.written += int64(written)
		if err != nil {
			return written, err
		}
	}
	if int64(original) > remaining {
		b.truncated = true
		b.once.Do(func() { close(b.overflow) })
	}
	return original, nil
}

func (b *boundedFile) size() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.written
}

func (b *boundedFile) wasTruncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

func writeResult(filename string, result testResult) error {
	temporary := filename + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(result)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporary, filename)
}

func runID(started time.Time) string {
	return fmt.Sprintf("%s-%06d", started.Format("20060102T150405Z"), os.Getpid())
}

func sourceTree(root string) string {
	if gitValue(root, "status", "--porcelain") == "" {
		return "clean"
	}
	return "dirty"
}

func gitValue(root string, arguments ...string) string {
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	var output strings.Builder
	command.Stdout = &limitedStringWriter{target: &output, remaining: 64 << 10}
	command.Stderr = &limitedStringWriter{remaining: 64 << 10}
	if command.Run() != nil {
		return "unknown"
	}
	return strings.TrimSpace(output.String())
}

type limitedStringWriter struct {
	target    *strings.Builder
	remaining int
}

func (w *limitedStringWriter) Write(content []byte) (int, error) {
	original := len(content)
	if len(content) > w.remaining {
		content = content[:w.remaining]
	}
	if w.target != nil {
		_, _ = w.target.Write(content)
	}
	w.remaining -= len(content)
	return original, nil
}
