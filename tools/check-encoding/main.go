// check-encoding проверяет текстовые файлы на невалидный UTF-8 и типичные следы mojibake.
//
// Mojibake появляется, когда русский UTF-8 текст ошибочно прочитали как Windows-1251
// и потом сохранили обратно в UTF-8. Проверка ищет повторяющиеся пары символов
// из кириллических букв "Р" и "С", которые часто встречаются в таком повреждённом тексте.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var mojibakePattern = regexp.MustCompile(`(?:\x{0420}[\p{Cyrillic}\x{00b0}\x{00b1}\x{0451}\x{0401}]|\x{0421}[\p{Cyrillic}]){2,}`)

func main() {
	staged := len(os.Args) == 2 && os.Args[1] == "--staged"
	if len(os.Args) > 2 || (len(os.Args) == 2 && !staged) {
		fmt.Fprintln(os.Stderr, "использование: check-encoding [--staged]")
		os.Exit(2)
	}

	files, err := listedFiles(staged)
	if err != nil {
		fmt.Fprintf(os.Stderr, "не удалось получить список файлов: %v\n", err)
		os.Exit(1)
	}

	var problems []fileProblem
	for _, path := range files {
		if shouldSkip(path) {
			continue
		}

		data, err := readFile(path, staged)
		if err != nil {
			fmt.Fprintf(os.Stderr, "не удалось прочитать %s: %v\n", path, err)
			os.Exit(1)
		}
		if isBinary(data) {
			continue
		}

		if !utf8.Valid(data) {
			problems = append(problems, fileProblem{path: path, message: "файл содержит невалидный UTF-8"})
			continue
		}

		line, sample := findMojibake(string(data))
		if line > 0 {
			problems = append(problems, fileProblem{
				path:    path,
				line:    line,
				message: fmt.Sprintf("возможный mojibake: %q", sample),
			})
		}
	}

	if len(problems) == 0 {
		fmt.Println("проверка кодировки пройдена: текстовые файлы в UTF-8, явный mojibake не найден")
		return
	}

	sort.Slice(problems, func(i, j int) bool {
		if problems[i].path == problems[j].path {
			return problems[i].line < problems[j].line
		}
		return problems[i].path < problems[j].path
	})

	fmt.Fprintln(os.Stderr, "найдены проблемы с кодировкой:")
	for _, problem := range problems {
		if problem.line > 0 {
			fmt.Fprintf(os.Stderr, "  %s:%d %s\n", problem.path, problem.line, problem.message)
			continue
		}
		fmt.Fprintf(os.Stderr, "  %s %s\n", problem.path, problem.message)
	}
	os.Exit(1)
}

type fileProblem struct {
	path    string
	line    int
	message string
}

func listedFiles(staged bool) ([]string, error) {
	if staged {
		return stagedFiles()
	}
	return trackedFiles()
}

func trackedFiles() ([]string, error) {
	out, err := gitOutput("ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	return splitGitPaths(out), nil
}

func stagedFiles() ([]string, error) {
	// В pre-commit важно смотреть индекс, потому что именно он попадёт в коммит.
	out, err := gitOutput("diff", "--cached", "--name-only", "-z", "--diff-filter=ACMR")
	if err != nil {
		return nil, err
	}
	return splitGitPaths(out), nil
}

func splitGitPaths(out []byte) []string {
	raw := bytes.Split(out, []byte{0})
	files := make([]string, 0, len(raw))
	for _, item := range raw {
		if len(item) == 0 {
			continue
		}
		files = append(files, normalizePath(string(item)))
	}
	sort.Strings(files)
	return files
}

func readFile(path string, staged bool) ([]byte, error) {
	if !staged {
		return os.ReadFile(path)
	}
	return gitOutput("show", ":"+path)
}

func gitOutput(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	return cmd.Output()
}

func findMojibake(text string) (int, string) {
	for index, line := range strings.Split(text, "\n") {
		match := mojibakePattern.FindString(line)
		if match == "" {
			continue
		}
		return index + 1, trimSample(match)
	}
	return 0, ""
}

func trimSample(sample string) string {
	const maxRunes = 80
	runes := []rune(strings.TrimSpace(sample))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func shouldSkip(path string) bool {
	return strings.HasPrefix(path, "api/gen/") ||
		strings.HasPrefix(path, "internal/dbgen/") ||
		path == "api/openapi/openapi.yaml" ||
		path == "tmp-auth-stderr.log" ||
		path == "tmp-auth-stdout.log"
}

func isBinary(data []byte) bool {
	return bytes.IndexByte(data, 0) >= 0
}

func normalizePath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}
