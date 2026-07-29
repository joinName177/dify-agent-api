package main

import "testing"

func TestValidateTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		title string
		valid bool
	}{
		{name: "feature", title: "feat(auth): add refresh token rotation", valid: true},
		{name: "breaking", title: "refactor(api)!: remove legacy session endpoint", valid: true},
		{name: "unknown type", title: "feature(auth): add refresh token rotation"},
		{name: "unknown scope", title: "feat(random): add refresh token rotation"},
		{name: "missing scope", title: "fix: reject expired sessions"},
		{name: "uppercase summary", title: "fix(auth): Reject expired sessions"},
		{name: "short summary", title: "fix(auth): no"},
		{name: "newline", title: "fix(auth): reject expired sessions\nbody"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateTitle(test.title)
			if test.valid && err != nil {
				t.Fatalf("validateTitle() error = %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("validateTitle() unexpectedly passed")
			}
		})
	}
}

func TestFirstLine(t *testing.T) {
	t.Parallel()
	if got := firstLine("\ufefffix(auth): reject expired sessions\r\nbody"); got != "fix(auth): reject expired sessions" {
		t.Fatalf("firstLine() = %q", got)
	}
}
