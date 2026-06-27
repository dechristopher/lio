package view

import (
	"strconv"
	"strings"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
)

// TestInviteQR verifies the inline SVG faithfully transcribes the encoder's
// module bitmap: the viewBox spans the full bitmap (incl. quiet zone) and there
// is exactly one dark path segment per dark module. The encoding itself is the
// library's (well-tested) responsibility, so matching its bitmap is what proves
// the rendered code is scannable.
func TestInviteQR(t *testing.T) {
	const url = "https://lioctad.org/abc123"
	out := renderSmoke(t, inviteQR(url))

	if !strings.HasPrefix(out, `<svg class="invite-qr"`) {
		t.Fatalf("expected svg prefix, got: %.48q", out)
	}
	if !strings.HasSuffix(out, "</svg>") {
		t.Errorf("svg not closed: %.48q", out[len(out)-48:])
	}
	mustContain(t, out, `fill="#ffffff"`) // light background for scannability
	mustContain(t, out, `fill="#1c1917"`) // dark modules

	q, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		t.Fatal(err)
	}
	bm := q.Bitmap()
	n := len(bm)
	mustContain(t, out, `viewBox="0 0 `+strconv.Itoa(n)+` `+strconv.Itoa(n)+`"`)

	dark := 0
	for y := range bm {
		for x := range bm[y] {
			if bm[y][x] {
				dark++
			}
		}
	}
	if got := strings.Count(out, "h1v1h-1z"); got != dark {
		t.Errorf("module mismatch: svg has %d dark modules, bitmap has %d", got, dark)
	}
}

// TestInviteQREmptyOnError keeps the QR additive: a bad input must not blow up
// the page, just render nothing (the copyable invite link still works).
func TestInviteQREmptyOnError(t *testing.T) {
	// content that exceeds QR capacity at every version forces an encode error.
	huge := strings.Repeat("x", 8000)
	if out := renderSmoke(t, inviteQR(huge)); out != "" {
		t.Errorf("expected empty render on encode error, got %d bytes", len(out))
	}
}
