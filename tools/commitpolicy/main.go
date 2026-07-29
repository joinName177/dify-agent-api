package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var titlePattern = regexp.MustCompile(`^(feat|fix|refactor|perf|test|docs|build|ci|chore|revert)\((user|auth|http|config|db|openapi|build|ci|deps|docs|security|observability|release|governance|test|architecture|migration|api|platform)\)(!)?: [a-z0-9][^\r\n]{4,71}$`)

func main() {
	title, err := titleFromEnvironment()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := validateTitle(title); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Commit title policy passed.")
}

func titleFromEnvironment() (string, error) {
	messageFile := os.Getenv("COMMIT_MESSAGE_FILE")
	environmentTitle := os.Getenv("COMMIT_TITLE")
	if messageFile != "" && environmentTitle != "" {
		return "", errors.New("set only one of COMMIT_MESSAGE_FILE or COMMIT_TITLE")
	}
	if messageFile != "" {
		content, err := os.ReadFile(messageFile)
		if err != nil {
			return "", fmt.Errorf("read commit message: %w", err)
		}
		return firstLine(string(content)), nil
	}
	if environmentTitle == "" {
		return "", errors.New("COMMIT_TITLE or COMMIT_MESSAGE_FILE is required")
	}
	return firstLine(environmentTitle), nil
}

func firstLine(value string) string {
	value = strings.TrimPrefix(value, "\ufeff")
	if index := strings.IndexAny(value, "\r\n"); index >= 0 {
		value = value[:index]
	}
	return value
}

func validateTitle(title string) error {
	if !titlePattern.MatchString(title) {
		return fmt.Errorf("invalid commit title %q; expected type(scope)!: imperative summary with an approved type and scope (5-72 summary characters, lowercase start)", title)
	}
	if len(title) > 100 {
		return fmt.Errorf("commit title is %d bytes; maximum is 100", len(title))
	}
	return nil
}
