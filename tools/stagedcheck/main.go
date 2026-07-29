package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	root := flag.String("root", "", "root of the staged worktree snapshot")
	pathsFile := flag.String("paths", "", "NUL-delimited staged path list")
	flag.Parse()
	if *root == "" || *pathsFile == "" {
		fail(fmt.Errorf("both -root and -paths are required"))
	}
	pathsOutput, err := os.ReadFile(*pathsFile)
	if err != nil {
		fail(fmt.Errorf("read staged paths: %w", err))
	}
	for _, path := range splitNull(pathsOutput) {
		if filepath.Ext(path) != ".go" {
			continue
		}
		if filepath.IsAbs(path) || filepath.Clean(path) != path || path == ".." {
			fail(fmt.Errorf("unsafe staged path: %q", path))
		}
		content, err := os.ReadFile(filepath.Join(*root, filepath.FromSlash(path)))
		if err != nil {
			fail(fmt.Errorf("read staged %s: %w", path, err))
		}
		formatted, err := gofmt(content)
		if err != nil {
			fail(fmt.Errorf("format staged %s: %w", path, err))
		}
		if !bytes.Equal(content, formatted) {
			fail(fmt.Errorf("staged Go file is not formatted: %s", path))
		}
	}
	fmt.Println("Staged Go formatting passed.")
}

func gofmt(source []byte) ([]byte, error) {
	command := exec.Command("gofmt")
	command.Stdin = bytes.NewReader(source)
	return command.Output()
}

func splitNull(value []byte) []string {
	parts := bytes.Split(value, []byte{0})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 0 {
			result = append(result, string(part))
		}
	}
	return result
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
