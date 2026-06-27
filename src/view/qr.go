package view

import (
	"strconv"
	"strings"

	"github.com/a-h/templ"
	qrcode "github.com/skip2/go-qrcode"
)

// inviteQR renders the given URL as an inline SVG QR code for the challenger's
// waiting room, so an opponent can scan it from a phone instead of typing the
// invite link.
//
// The modules are drawn in a fixed dark fill on a white background (deliberately
// NOT the theme tokens) so the code stays high-contrast and scannable in both
// light and dark themes; the surrounding .qr-tile gives it a light card to sit
// on. qrcode.Bitmap() already includes the quiet-zone border, which the white
// background rect preserves.
//
// The QR is purely additive — the copyable invite link beside it always works —
// so on any encoding error we render nothing rather than failing the page.
func inviteQR(url string) templ.Component {
	q, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return templ.NopComponent
	}

	bitmap := q.Bitmap()
	dim := strconv.Itoa(len(bitmap))

	// One <path> of all dark modules keeps the SVG compact: each module is a
	// 1x1 unit cell at its (x, y) in the bitmap's own coordinate space.
	var path strings.Builder
	for y, row := range bitmap {
		ys := strconv.Itoa(y)
		for x, dark := range row {
			if dark {
				path.WriteString("M")
				path.WriteString(strconv.Itoa(x))
				path.WriteString(" ")
				path.WriteString(ys)
				path.WriteString("h1v1h-1z")
			}
		}
	}

	svg := `<svg class="invite-qr" viewBox="0 0 ` + dim + ` ` + dim + `" ` +
		`shape-rendering="crispEdges" xmlns="http://www.w3.org/2000/svg" ` +
		`role="img" aria-label="QR code linking to the game invite">` +
		`<rect width="` + dim + `" height="` + dim + `" fill="#ffffff"/>` +
		`<path d="` + path.String() + `" fill="#1c1917"/>` +
		`</svg>`

	return templ.Raw(svg)
}
