// check-file-lines проверяет, что ручные файлы проекта не разрастаются сверх лимита.
//
// Инструмент намеренно живёт в репозитории, а не в отдельном линтере: правило простое,
// а список старых исключений должен быть виден рядом с кодом.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const maxLines = 300

func main() {
	files, err := trackedFiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot list files: %v\n", err)
		os.Exit(1)
	}

	ignored, err := readIgnoreFile(".line-limit-ignore")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read .line-limit-ignore: %v\n", err)
		os.Exit(1)
	}

	seenIgnored := map[string]bool{}
	var violations []fileLines
	var staleIgnores []string

	for _, path := range files {
		if isGeneratedOrBundled(path) {
			continue
		}

		lines, err := countLines(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot count lines in %s: %v\n", path, err)
			os.Exit(1)
		}

		if ignored[path] {
			seenIgnored[path] = true
			if lines <= maxLines {
				staleIgnores = append(staleIgnores, path)
			}
			continue
		}

		if lines > maxLines {
			violations = append(violations, fileLines{path: path, lines: lines})
		}
	}

	for path := range ignored {
		if !seenIgnored[path] {
			staleIgnores = append(staleIgnores, path)
		}
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].lines == violations[j].lines {
			return violations[i].path < violations[j].path
		}
		return violations[i].lines > violations[j].lines
	})
	sort.Strings(staleIgnores)

	if len(violations) == 0 && len(staleIgnores) == 0 {
		fmt.Printf("file line check passed: handwritten files are within %d lines or in baseline\n", maxLines)
		return
	}

	if len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "files over %d lines must be decomposed:\n", maxLines)
		for _, item := range violations {
			fmt.Fprintf(os.Stderr, "  %4d %s\n", item.lines, item.path)
		}
	}

	if len(staleIgnores) > 0 {
		fmt.Fprintln(os.Stderr, "remove stale entries from .line-limit-ignore:")
		for _, path := range staleIgnores {
			fmt.Fprintf(os.Stderr, "  %s\n", path)
		}
	}

	os.Exit(1)
}

type fileLines struct {
	path  string
	lines int
}

func trackedFiles() ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	raw := bytes.Split(out, []byte{0})
	files := make([]string, 0, len(raw))
	for _, item := range raw {
		if len(item) == 0 {
			continue
		}
		files = append(files, normalizePath(string(item)))
	}
	sort.Strings(files)
	return files, nil
}

func readIgnoreFile(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	ignored := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ignored[normalizePath(line)] = true
	}
	return ignored, nil
}

func countLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return 0, nil
	}

	lines := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		lines++
	}
	return lines, nil
}

func isGeneratedOrBundled(path string) bool {
	return strings.HasPrefix(path, "api/gen/") ||
		strings.HasPrefix(path, "internal/dbgen/") ||
		path == "api/openapi/openapi.yaml"
}

func normalizePath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}
