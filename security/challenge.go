package security

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"path/filepath"
	"sort"
	"strconv"
)

type SampleLocation struct {
	DirStack  []int // every index is level and value is dir num
	FileStack []int // file index, line index, left line border, right line border
}

func generateSample(codebase FileSystem, dir string, dirStack []int) (_ string, result SampleLocation, err error) {
	entries, err := fs.ReadDir(codebase, dir)
	if err != nil {
		return "", result, fmt.Errorf("challenge: read dir %s: %w", dir, err)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var dirs, files []fs.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}

	perm := rand.Perm(len(dirs))
	for _, dirIndex := range perm {
		selectedDir := dirs[dirIndex]
		subPath := filepath.Join(dir, selectedDir.Name())

		subEntries, err := fs.ReadDir(codebase, subPath)
		if err != nil {
			continue
		}

		var fileCount int
		for _, e := range subEntries {
			if !e.IsDir() {
				fileCount++
			}
		}
		if fileCount == 0 {
			continue
		}

		// рекурсивный вызов с обновлённым DirStack
		sample, subResult, err := generateSample(codebase, subPath, append(dirStack, dirIndex))
		if err == nil {
			return sample, subResult, nil
		}
	}

	if len(files) == 0 {
		return "", result, errors.New("challenge: no usable files or subdirectories")
	}

	fileIndex := rand.IntN(len(files))
	selectedFile := files[fileIndex]
	fullPath := filepath.Join(dir, selectedFile.Name())

	line, lineNum, err := getRandomLine(codebase, fullPath)
	if err != nil {
		return "", result, fmt.Errorf("challenge: read random line from %s: %w", fullPath, err)
	}
	if len(line) == 0 {
		return "", result, fmt.Errorf("challenge: empty line in %s", fullPath)
	}

	lineLen := len(line)
	var left, right int
	if lineLen == 1 {
		left, right = 0, 1
	} else {
		left = rand.IntN(lineLen - 1)
		right = left + 1 + rand.IntN(lineLen-left-1)
	}

	sample := line[left:right]

	return sample, SampleLocation{
		DirStack:  dirStack,
		FileStack: []int{fileIndex, lineNum, left, right},
	}, nil
}

func getRandomLine(codebase FileSystem, path string) (string, int, error) {
	lines, err := readLines(codebase, path)
	if err != nil {
		return "", 0, err
	}
	if len(lines) == 0 {
		return "", 0, fmt.Errorf("challenge: no non-empty lines in %s", path)
	}

	index := rand.IntN(len(lines))
	return lines[index], index, nil
}

func GenerateChallenge(codebase FileSystem, nonce int64) ([]byte, SampleLocation, error) {
	sample, location, err := generateSample(codebase, ".", []int{})
	if err != nil {
		return nil, SampleLocation{}, err
	}
	challengeResult := ConvertToSHA256([]byte(sample + strconv.FormatInt(nonce, 10)))
	return challengeResult, location, nil
}

func findSample(codebase FileSystem, loc SampleLocation) (string, error) {
	currentDir := "."

	for level, dirIndex := range loc.DirStack {
		entries, err := fs.ReadDir(codebase, currentDir)
		if err != nil {
			return "", fmt.Errorf("challenge: read dir %s: %w", currentDir, err)
		}

		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].Name() < entries[j].Name()
		})

		var dirs []fs.DirEntry
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, e)
			}
		}
		if dirIndex >= len(dirs) {
			return "", fmt.Errorf("challenge: dir index %d out of bounds at level %d", dirIndex, level)
		}

		nextDir := dirs[dirIndex].Name()
		currentDir = filepath.Join(currentDir, nextDir)
	}

	entries, err := fs.ReadDir(codebase, currentDir)
	if err != nil {
		return "", fmt.Errorf("challenge: read files from %s: %w", currentDir, err)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var regularFiles []fs.DirEntry
	for _, e := range entries {
		if !e.IsDir() {
			regularFiles = append(regularFiles, e)
		}
	}
	if len(loc.FileStack) != 4 {
		return "", fmt.Errorf("challenge: invalid file stack size - expected 4 elements")
	}

	fileIndex := loc.FileStack[0]
	lineIndex := loc.FileStack[1]
	left := loc.FileStack[2]
	right := loc.FileStack[3]

	if fileIndex >= len(regularFiles) {
		return "", fmt.Errorf("challenge: file index %d out of bounds - found %d files", fileIndex, len(regularFiles))
	}

	targetFile := regularFiles[fileIndex].Name()
	fullPath := filepath.Join(currentDir, targetFile)

	lines, err := readLines(codebase, fullPath)
	if err != nil {
		return "", fmt.Errorf("challenge: read lines from %s: %w", fullPath, err)
	}
	if lineIndex >= len(lines) {
		return "", fmt.Errorf("challenge: line index %d out of bounds, len=%d", lineIndex, len(lines))
	}

	line := lines[lineIndex]
	if left > right || left < 0 || right > len(line) {
		return "", fmt.Errorf("challenge: invalid substring bounds: [%d:%d] on len=%d", left, right, len(line))
	}

	return line[left:right], nil
}

func readLines(codebase FileSystem, path string) ([]string, error) {
	f, err := codebase.Open(path)
	if err != nil {
		return nil, err
	}

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		if len(text) <= 2 { // drop '}',')', '\t', '\n' etc.
			continue
		}

		lines = append(lines, text)
	}

	_ = f.Close()

	return lines, scanner.Err()
}

func ResolveChallenge(codebase FileSystem, location SampleLocation, nonce int64) ([]byte, error) {
	sample, err := findSample(codebase, location)
	if err != nil {
		return nil, err
	}
	challengeResult := ConvertToSHA256([]byte(sample + strconv.FormatInt(nonce, 10)))
	return challengeResult, nil
}
