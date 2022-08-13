package util

import (
	"embed"
	"os"
	"testing"

	"github.com/dechristopher/lioctad/env"
)

//go:embed data/*
var testFs embed.FS

func TestPickFSEmbedded(t *testing.T) {
	err := os.Setenv("DEPLOY", "prod")
	if err != nil {
		t.Fatal(err)
	}

	fs := PickFS(env.IsLocal(), testFs, "./data")
	f, err := fs.Open("sample.txt")
	if err != nil {
		t.Fatal(err)
	}

	stat, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if stat.Size() == 0 {
		t.Fatal("couldn't read embedded fs")
	}
}

func TestPickFSOnDisk(t *testing.T) {
	err := os.Setenv("DEPLOY", "local")
	if err != nil {
		t.Fatal(err)
	}

	fs := PickFS(env.IsLocal(), testFs, "./data")
	f, err := fs.Open("sample.txt")
	if err != nil {
		t.Fatal(err)
	}

	stat, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if stat.Size() == 0 {
		t.Fatal("couldn't read on-disk fs")
	}
}
