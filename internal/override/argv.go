package override

import (
	"path/filepath"
	"regexp"
	"strings"
)

var environmentAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// mapping relocates paths from the original checkout to the override
// checkout. Both values are physical, symlink-resolved top-level directories.
type mapping struct {
	original string
	override string
}

// mapPath maps a path inside the original checkout to the same relative path
// in the override checkout. Paths outside the original checkout stay put.
func (m mapping) mapPath(path string) string {
	relative, err := filepath.Rel(m.original, path)
	if err != nil || escapes(relative) {
		return path
	}
	return filepath.Join(m.override, relative)
}

// rewriteArgv re-anchors relative path arguments so that moving the working
// directory into the override never silently runs a different file. See
// rewriteValue for the exact rule. The input slice is never modified.
func (m mapping) rewriteArgv(argv []string, originalWorkingDirectory string) ([]string, []Reanchor) {
	result := append([]string(nil), argv...)
	var changes []Reanchor
	for index, argument := range argv {
		rewritten, changed := m.rewriteArgument(argument, originalWorkingDirectory)
		if !changed {
			continue
		}
		result[index] = rewritten
		changes = append(changes, Reanchor{Original: argument, Resolved: rewritten})
	}
	return result, changes
}

// rewriteArgument applies rewriteValue to a bare argument, to the value of a
// --flag=value argument, or to the value of a NAME=value environment
// assignment such as the operands of env(1).
func (m mapping) rewriteArgument(argument, originalWorkingDirectory string) (string, bool) {
	prefix, value := "", argument
	if strings.HasPrefix(argument, "-") {
		index := strings.IndexByte(argument, '=')
		if index < 0 {
			return argument, false
		}
		prefix, value = argument[:index+1], argument[index+1:]
	} else if environmentAssignment.MatchString(argument) {
		index := strings.IndexByte(argument, '=')
		prefix, value = argument[:index+1], argument[index+1:]
	}
	rewritten, changed := m.rewriteValue(value, originalWorkingDirectory)
	if !changed {
		return argument, false
	}
	return prefix + rewritten, true
}

// rewriteValue decides one relative path value. Lexically walking the value
// from the original working directory:
//
//  1. it never leaves the original checkout: unchanged, so it follows the
//     override because the working directory moved with it;
//  2. it leaves the checkout and comes back in: rewritten to the absolute
//     mapped path inside the override;
//  3. it ends outside the original checkout: rewritten to its absolute
//     original path, so the same file runs as before the override.
//
// Only values containing a ".." segment can leave, so ordinary arguments and
// absolute paths are never touched.
func (m mapping) rewriteValue(value, originalWorkingDirectory string) (string, bool) {
	if value == "" || filepath.IsAbs(value) || !hasParentSegment(value) {
		return value, false
	}
	start, err := filepath.Rel(m.original, originalWorkingDirectory)
	if err != nil || escapes(start) {
		return value, false
	}
	if !walkEscapes(start, value) {
		return value, false
	}
	absolute := filepath.Join(originalWorkingDirectory, value)
	relative, err := filepath.Rel(m.original, absolute)
	if err == nil && !escapes(relative) {
		return filepath.Join(m.override, relative), true
	}
	return absolute, true
}

func hasParentSegment(value string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(value), "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

// walkEscapes reports whether any prefix of value, walked from start, climbs
// above the checkout root.
func walkEscapes(start, value string) bool {
	depth := 0
	if start != "." {
		depth = len(strings.Split(filepath.ToSlash(start), "/"))
	}
	for _, segment := range strings.Split(filepath.ToSlash(value), "/") {
		switch segment {
		case "", ".":
		case "..":
			depth--
			if depth < 0 {
				return true
			}
		default:
			depth++
		}
	}
	return false
}

func escapes(relative string) bool {
	return relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative)
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && !escapes(relative)
}
