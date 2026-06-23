package rng

import "testing"

func TestIntnBounds(t *testing.T) {
	for _, n := range []int{1, 2, 5, 58, 256} {
		for i := 0; i < 1000; i++ {
			v := Intn(n)
			if v < 0 || v >= n {
				t.Fatalf("Intn(%d) = %d, out of range [0, %d)", n, v, n)
			}
		}
	}
}

func TestIntnNonPositive(t *testing.T) {
	for _, n := range []int{0, -1, -100} {
		if v := Intn(n); v != 0 {
			t.Errorf("Intn(%d) = %d, want 0", n, v)
		}
	}
}

func TestIntnUniformish(t *testing.T) {
	// every value in [0, n) should appear at least once over enough draws
	const n = 6
	seen := make(map[int]bool)
	for i := 0; i < 5000; i++ {
		seen[Intn(n)] = true
	}
	if len(seen) != n {
		t.Errorf("Intn(%d) produced %d distinct values, want %d", n, len(seen), n)
	}
}

func TestBoolBothValues(t *testing.T) {
	var sawTrue, sawFalse bool
	for i := 0; i < 1000 && !(sawTrue && sawFalse); i++ {
		if Bool() {
			sawTrue = true
		} else {
			sawFalse = true
		}
	}
	if !sawTrue || !sawFalse {
		t.Errorf("Bool() did not produce both values (true=%v false=%v)", sawTrue, sawFalse)
	}
}
