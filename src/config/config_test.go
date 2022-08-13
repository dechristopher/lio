package config

import "testing"

// TestGenerateCode tests code generation
func TestGenerateCode(t *testing.T) {
	if len(GenerateCode(5, false)) != 5 {
		t.Fatalf("Epic fail, code generation sucks!")
	}
}
