package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/jamesonstone/rungrid/internal/maintenance"
)

// tree is the committed content of one commit: top-level-relative slash path
// to "<mode> <type> <object>". Fingerprints hash committed blobs, so they are
// stable across runs and independent of file modification times.
type tree map[string]string

func listTree(ctx context.Context, runner maintenance.Runner, directory, oid string) (tree, error) {
	content, err := runner.Run(ctx, directory, "git", "ls-tree", "-r", "--full-tree", "-z", oid)
	if err != nil {
		return nil, fmt.Errorf("list tree %s: %w", oid, err)
	}
	result := tree{}
	for _, record := range bytes.Split(content, []byte{0}) {
		meta, name, found := strings.Cut(string(record), "\t")
		if found && name != "" {
			result[name] = meta
		}
	}
	return result, nil
}

func (t tree) has(name string) bool {
	_, ok := t[name]
	return ok
}

// fingerprint hashes every committed path below base that matches one of the
// base-relative patterns.
func (t tree) fingerprint(base string, patterns []string) string {
	names := make([]string, 0, len(t))
	for name := range t {
		names = append(names, name)
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		relative, ok := below(base, name)
		if !ok || !matchesAny(patterns, relative) {
			continue
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%s\n", name, t[name])
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

// below returns name relative to the base directory ("" is the top level).
func below(base, name string) (string, bool) {
	if base == "" {
		return name, true
	}
	if strings.HasPrefix(name, base+"/") {
		return strings.TrimPrefix(name, base+"/"), true
	}
	return "", false
}

func join(base, name string) string {
	if base == "" {
		return name
	}
	return base + "/" + name
}

func matchesAny(patterns []string, relative string) bool {
	for _, pattern := range patterns {
		if matchPattern(pattern, relative) {
			return true
		}
	}
	return false
}

// matchPattern matches one slash path. "." matches everything; a literal path
// matches that file or everything below that directory; a glob (* ? [ ] **)
// matches whole paths, with ** spanning any number of directories.
func matchPattern(pattern, relative string) bool {
	clean := path.Clean(pattern)
	if clean == "." {
		return true
	}
	if !strings.ContainsAny(clean, "*?[") {
		return relative == clean || strings.HasPrefix(relative, clean+"/")
	}
	return matchSegments(strings.Split(clean, "/"), strings.Split(relative, "/"))
}

func matchSegments(pattern, segments []string) bool {
	if len(pattern) == 0 {
		return len(segments) == 0
	}
	if pattern[0] == "**" {
		for skip := 0; skip <= len(segments); skip++ {
			if matchSegments(pattern[1:], segments[skip:]) {
				return true
			}
		}
		return false
	}
	if len(segments) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], segments[0])
	return err == nil && matched && matchSegments(pattern[1:], segments[1:])
}
