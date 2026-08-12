package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

func openSamples(directory string) (*os.File, *csv.Writer, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(filepath.Join(directory, "samples.csv"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"captured_at", "guard_cpu_percent", "guard_rss_bytes", "sampler_ms", "state_bytes", "restarts", "circuits"}); err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, writer, nil
}

func writeSample(writer *csv.Writer, item sample) error {
	if err := writer.Write([]string{item.At.UTC().Format(time.RFC3339Nano), fmt.Sprintf("%.3f", item.CPU), strconv.FormatUint(item.RSS, 10), fmt.Sprintf("%.3f", item.SamplerMS), strconv.FormatUint(item.StateBytes, 10), strconv.Itoa(item.Restarts), strconv.Itoa(item.Circuits)}); err != nil {
		return err
	}
	writer.Flush()
	return writer.Error()
}

func (r *result) add(item sample) {
	r.Samples++
	r.AverageGuardCPU += item.CPU
	r.MaximumRSSBytes = max(r.MaximumRSSBytes, item.RSS)
	r.MaximumState = max(r.MaximumState, item.StateBytes)
	r.RestartEvents = max(r.RestartEvents, item.Restarts)
	r.CircuitOpenings = max(r.CircuitOpenings, item.Circuits)
	r.cpuSamples = append(r.cpuSamples, item.CPU)
	r.samplerSamples = append(r.samplerSamples, item.SamplerMS)
}

func (r *result) evaluate() {
	if r.Samples == 0 {
		r.Failures = append(r.Failures, "no samples")
		return
	}
	r.AverageGuardCPU /= float64(r.Samples)
	r.P99GuardCPU = percentile(r.cpuSamples, 0.99)
	r.P99SamplerMS = percentile(r.samplerSamples, 0.99)
	checks := []struct {
		failed bool
		name   string
	}{{r.AverageGuardCPU >= 1, "average guard CPU is not below 1% of one core"}, {r.P99GuardCPU >= 5, "p99 guard CPU is not below 5% of one core"}, {r.MaximumRSSBytes >= 64<<20, "guard RSS is not below 64 MiB"}, {r.P99SamplerMS >= 250, "sampler p99 is not below 250 ms"}, {r.MaximumState >= 10<<20, "resource guard state is not below 10 MiB"}, {r.RestartEvents != 0, "resource restarts occurred"}, {r.CircuitOpenings != 0, "resource circuits opened"}}
	for _, check := range checks {
		if check.failed {
			r.Failures = append(r.Failures, check.name)
		}
	}
}

func percentile(values []float64, quantile float64) float64 {
	items := append([]float64(nil), values...)
	sort.Float64s(items)
	index := int(float64(len(items)-1) * quantile)
	return items[index]
}

func directorySize(root string) (uint64, error) {
	var total uint64
	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("resource guard state contains a symlink")
		}
		if info.Mode().IsRegular() {
			total += uint64(info.Size())
		}
		return nil
	})
	return total, err
}

func writeResult(directory string, outcome result) error {
	outcome.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	content, err := json.MarshalIndent(outcome, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".result-*.json")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() { _ = os.Remove(name) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(content, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(directory, "result.json"))
}
