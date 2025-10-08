package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type result struct {
	file string
	line int
	text string
}

var errorTokens = []string{
	"error",
	"fail",
	"denied",
	"timeout",
	"stale",
	"not responding",
	"unreachable",
	"unable",
	"refused",
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [file or directory]...\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "If no arguments are supplied, input is read from stdin.")
	}

	flag.Parse()

	tokens := prepareTokens(errorTokens)

	var matches []result
	var err error

	if flag.NArg() == 0 {
		matches, err = scanReader("<stdin>", os.Stdin, tokens)
	} else {
		matches, err = scanPaths(flag.Args(), tokens)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "scan error: %v\n", err)
		os.Exit(1)
	}

	report(matches)
}

func scanPaths(paths []string, tokens []string) ([]result, error) {
	if len(paths) == 0 {
		return nil, errors.New("no paths provided")
	}

	var matches []result

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return matches, fmt.Errorf("stat %s: %w", path, err)
		}

		if info.IsDir() {
			err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if d.IsDir() {
					return nil
				}

				fileMatches, err := scanFile(p, tokens)
				if err != nil {
					return err
				}

				matches = append(matches, fileMatches...)
				return nil
			})
			if err != nil {
				return matches, fmt.Errorf("walk %s: %w", path, err)
			}
			continue
		}

		fileMatches, err := scanFile(path, tokens)
		if err != nil {
			return matches, err
		}

		matches = append(matches, fileMatches...)
	}

	return matches, nil
}

func scanFile(path string, tokens []string) ([]result, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	return scanReader(path, file, tokens)
}

func scanReader(name string, reader io.Reader, tokens []string) ([]result, error) {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var matches []result
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		text := scanner.Text()
		if hasNFSError(text, tokens) {
			matches = append(matches, result{file: name, line: lineNumber, text: text})
		}
	}

	if err := scanner.Err(); err != nil {
		return matches, fmt.Errorf("scan %s: %w", name, err)
	}

	return matches, nil
}

func hasNFSError(line string, tokens []string) bool {
	normalized := strings.ToLower(line)
	if !strings.Contains(normalized, "nfs") {
		return false
	}

	for _, token := range tokens {
		if strings.Contains(normalized, token) {
			return true
		}
	}

	return false
}

func prepareTokens(tokens []string) []string {
	prepared := make([]string, len(tokens))
	for i, token := range tokens {
		prepared[i] = strings.ToLower(token)
	}

	return prepared
}

func report(matches []result) {
	if len(matches) == 0 {
		fmt.Println("No potential NFS errors found.")
		return
	}

	fmt.Println("Potential NFS errors:")
	for _, m := range matches {
		fmt.Printf("%s:%d: %s\n", m.file, m.line, m.text)
	}
	fmt.Printf("\nTotal matches: %d\n", len(matches))
}
