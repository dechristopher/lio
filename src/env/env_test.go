package env

import (
	"os"
	"testing"
)

func TestGetEnv(t *testing.T) {
	err := os.Setenv("DEPLOY", "prod")
	if err != nil {
		t.Fatal(err)
	}

	if GetEnv() != Prod {
		t.Fatal("GetEnv not prod env=", GetEnv())
	}
}

func TestIsProd(t *testing.T) {
	err := os.Setenv("DEPLOY", "prod")
	if err != nil {
		t.Fatal(err)
	}

	if !IsProd() {
		t.Fatal("IsProd not prod env=", GetEnv())
	}
}

func TestIsDev(t *testing.T) {
	err := os.Setenv("DEPLOY", "dev")
	if err != nil {
		t.Fatal(err)
	}

	if !IsDev() {
		t.Fatal("IsDev not dev env=", GetEnv())
	}
}

func TestIsLocal(t *testing.T) {
	err := os.Setenv("DEPLOY", "local")
	if err != nil {
		t.Fatal(err)
	}

	if !IsLocal() {
		t.Fatal("IsLocal not local env=", GetEnv())
	}
}
