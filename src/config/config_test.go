package config

import "testing"

// TestGenerateCode tests code generation
func TestGenerateCode(t *testing.T) {
	if len(GenerateCode(5, Hex)) != 5 {
		t.Fatalf("Epic fail, code generation sucks!")
	}
	if len(GenerateCode(16, Base58)) != 16 {
		t.Fatalf("Epic fail, code generation sucks!")
	}
	if len(GenerateCode(10)) != 10 {
		t.Fatalf("Epic fail, code generation sucks!")
	}
}
