package news

import "testing"

func TestLatestClampsToBounds(t *testing.T) {
	cases := []struct {
		name string
		n    int
		want int
	}{
		{"negative", -3, 0},
		{"zero", 0, 0},
		{"three", 3, 3},
		{"exact", len(Items), len(Items)},
		{"over", len(Items) + 10, len(Items)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(Latest(tc.n)); got != tc.want {
				t.Fatalf("Latest(%d) len = %d, want %d", tc.n, got, tc.want)
			}
		})
	}

	// The home page shows the three newest, in feed order.
	got := Latest(3)
	if len(got) != 3 || got[0] != Items[0] || got[2] != Items[2] {
		t.Fatalf("Latest(3) did not return the first three items newest-first")
	}
}

func TestPaginateClampsAndPages(t *testing.T) {
	wantTotal := (len(Items) + PerPage - 1) / PerPage
	if wantTotal < 1 {
		wantTotal = 1
	}

	// Out-of-range low clamps to page 1.
	first := Paginate(0)
	if first.Number != 1 || first.HasPrev {
		t.Fatalf("Paginate(0) = page %d hasPrev=%v, want page 1 hasPrev=false", first.Number, first.HasPrev)
	}
	if first.Total != wantTotal {
		t.Fatalf("Paginate(0).Total = %d, want %d", first.Total, wantTotal)
	}

	// Out-of-range high clamps to the last page and has no next.
	last := Paginate(wantTotal + 5)
	if last.Number != wantTotal || last.HasNext {
		t.Fatalf("Paginate(over) = page %d hasNext=%v, want page %d hasNext=false", last.Number, last.HasNext, wantTotal)
	}

	// Every page but the last is full; the union of all pages reconstructs the
	// feed in order with no gaps or overlaps.
	var rebuilt []Item
	for p := 1; p <= wantTotal; p++ {
		page := Paginate(p)
		if page.Number != p {
			t.Fatalf("Paginate(%d).Number = %d", p, page.Number)
		}
		if p < wantTotal && len(page.Items) != PerPage {
			t.Fatalf("Paginate(%d) len = %d, want full page of %d", p, len(page.Items), PerPage)
		}
		rebuilt = append(rebuilt, page.Items...)
	}
	if len(rebuilt) != len(Items) {
		t.Fatalf("pages cover %d items, want %d", len(rebuilt), len(Items))
	}
	for i := range Items {
		if rebuilt[i] != Items[i] {
			t.Fatalf("item %d out of order across pages", i)
		}
	}
}
