package main

import (
	"reflect"
	"testing"
)

func TestSplitNull(t *testing.T) {
	t.Parallel()
	got := splitNull([]byte("first.go\x00dir/second.go\x00"))
	want := []string{"first.go", "dir/second.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitNull() = %#v, want %#v", got, want)
	}
}

func TestGofmt(t *testing.T) {
	t.Parallel()
	got, err := gofmt([]byte("package sample\n\nfunc value( )int{return 1}\n"))
	if err != nil {
		t.Fatalf("gofmt() error = %v", err)
	}
	want := "package sample\n\nfunc value() int { return 1 }\n"
	if string(got) != want {
		t.Fatalf("gofmt() = %q, want %q", got, want)
	}
}
