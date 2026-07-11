// Package og renders the OpenGraph preview cards (1200×630 PNGs) served at
// /og/*.png and referenced by the og:image meta tags, so shared lioctad links
// unfurl with a real board preview instead of bare text.
//
// The board replicates the in-game octadground presentation 1:1 by
// compositing the same assets the game client uses — the green board theme
// colors, the alpha piece set SVGs (rasterized in-process with oksvg, no
// headless browser), the octadground coordinate labels, and the last-move /
// check overlays (see board.go; www.Serve hands the served static FS to
// LoadAssets at startup). The card chrome (background, wordmark, title text)
// is composed around it with the standard image/draw + x/image/font stack
// using the dark-theme design tokens from view/app.css and the site's
// Poppins faces (embedded here; the woff2 files under static/res/fonts can't
// be parsed by x/image/font). Scrapers do not accept SVG og:images, which is
// why the output is PNG.
package og

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"sync"

	"github.com/dechristopher/octad/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Width and Height are the standard large-summary OpenGraph card dimensions;
// the meta tags in view/layout.templ advertise the same values.
const (
	Width  = 1200
	Height = 630
)

// Dark-theme design tokens from view/app.css. The card always renders in dark
// mode: link unfurls sit on the chat app's own background, and the dark
// palette reads best across Discord/Slack/Twitter in both of their themes.
var (
	bgColor    = color.RGBA{0x1c, 0x19, 0x17, 0xff} // --surface
	textColor  = color.RGBA{0xf5, 0xf5, 0xf4, 0xff} // --text
	mutedColor = color.RGBA{0xa8, 0xa2, 0x9e, 0xff} // --text-muted
	accent     = color.RGBA{0x34, 0xd3, 0x99, 0xff} // --accent (dark)

	// the default green board theme's square colors (res/img/board/green.svg)
	lightSquare = color.RGBA{0xff, 0xff, 0xdf, 0xff}
	darkSquare  = color.RGBA{0x93, 0xaf, 0x6c, 0xff}
)

//go:embed font/Poppins-Bold.ttf
var poppinsBold []byte

//go:embed font/Poppins-SemiBold.ttf
var poppinsSemiBold []byte

//go:embed font/Poppins-Regular.ttf
var poppinsRegular []byte

// faces holds the parsed font faces, built once on first render. A face is
// not safe for concurrent use (it caches glyph rasterizations), so all
// drawing happens under faceMu — card renders are rare (scraper fetches),
// serialization is not a throughput concern.
var (
	faceOnce     sync.Once
	faceErr      error
	faceMu       sync.Mutex
	faceWordmark font.Face // Poppins Bold 64
	faceTitle    font.Face // Poppins Bold 44
	faceSub      font.Face // Poppins Regular 30
	faceCoord    font.Face // Poppins SemiBold — board coords at 4% board width
)

func initFaces() error {
	faceOnce.Do(func() {
		bold, err := opentype.Parse(poppinsBold)
		if err != nil {
			faceErr = fmt.Errorf("og: parse Poppins-Bold: %w", err)
			return
		}
		semiBold, err := opentype.Parse(poppinsSemiBold)
		if err != nil {
			faceErr = fmt.Errorf("og: parse Poppins-SemiBold: %w", err)
			return
		}
		regular, err := opentype.Parse(poppinsRegular)
		if err != nil {
			faceErr = fmt.Errorf("og: parse Poppins-Regular: %w", err)
			return
		}
		mk := func(f *opentype.Font, size float64) font.Face {
			if faceErr != nil {
				return nil
			}
			face, err := opentype.NewFace(f, &opentype.FaceOptions{
				Size: size, DPI: 72, Hinting: font.HintingFull,
			})
			if err != nil {
				faceErr = fmt.Errorf("og: face %g: %w", size, err)
			}
			return face
		}
		faceWordmark = mk(bold, 64)
		faceTitle = mk(bold, 44)
		faceSub = mk(regular, 30)
		// octadground draws coords at font-size 4cqw = 4% of the board edge
		faceCoord = mk(semiBold, 0.04*boardPx)
	})
	return faceErr
}

// Card describes one preview image. OFEN is the position to render (empty
// renders the octad starting position); Marks are squares to tint with the
// last-move highlight. Title/Subtitle are the text column next to the board.
type Card struct {
	OFEN     string
	Marks    []octad.Square
	Title    string
	Subtitle string
}

var (
	defaultMu   sync.Mutex
	defaultCard []byte
)

// Default returns the site-wide card (starting position + tagline) used by
// every non-room page. A successful render is cached for the process; an
// error (e.g. called before LoadAssets) is not, so the card recovers once
// assets are available.
func Default() ([]byte, error) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultCard != nil {
		return defaultCard, nil
	}
	card, err := Render(Card{
		Title:    "Octad — 4x4 chess with a twist",
		Subtitle: "Play with the computer, friends, or random players. Free, no ads.",
	})
	if err == nil {
		defaultCard = card
	}
	return card, err
}

// Render composes the card into an encoded PNG.
func Render(c Card) ([]byte, error) {
	if err := initFaces(); err != nil {
		return nil, err
	}

	const (
		pad   = (Height - boardPx) / 2 // 59: centers the board vertically
		textX = pad + boardPx + 70
		textW = Width - textX - pad
	)

	// held for the whole composition: renderBoard's coord labels and the text
	// column below both draw with the shared (glyph-caching, not thread-safe)
	// font faces
	faceMu.Lock()
	defer faceMu.Unlock()

	board, err := renderBoard(c.OFEN, c.Marks)
	if err != nil {
		return nil, err
	}

	dst := image.NewRGBA(image.Rect(0, 0, Width, Height))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(bgColor), image.Point{}, draw.Src)

	// the board sits flat on the page background in-game; same here
	draw.Draw(dst, image.Rect(pad, pad, pad+boardPx, pad+boardPx),
		board, image.Point{}, draw.Src)

	// wordmark: "lioctad" in text color, ".org" in the accent
	wordY := pad + 76
	w := drawString(dst, faceWordmark, textColor, textX, wordY, "lioctad")
	drawString(dst, faceWordmark, accent, textX+w, wordY, ".org")
	// accent underline beneath the wordmark
	draw.Draw(dst, image.Rect(textX, wordY+22, textX+64, wordY+28),
		image.NewUniform(accent), image.Point{}, draw.Src)

	// title, wrapped to the text column, then the subtitle below it
	y := wordY + 130
	for _, line := range wrap(faceTitle, c.Title, textW) {
		drawString(dst, faceTitle, textColor, textX, y, line)
		y += 58
	}
	y += 16
	for _, line := range wrap(faceSub, c.Subtitle, textW) {
		drawString(dst, faceSub, mutedColor, textX, y, line)
		y += 42
	}

	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// drawString draws s at the baseline (x, y) and returns its advance in pixels.
func drawString(dst draw.Image, face font.Face, c color.Color, x, y int, s string) int {
	d := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(s)
	return (d.Dot.X - fixed.I(x)).Ceil()
}

// wrap greedily wraps s into lines no wider than maxWidth pixels.
func wrap(face font.Face, s string, maxWidth int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	d := font.Drawer{Face: face}
	var lines []string
	line := words[0]
	for _, word := range words[1:] {
		if d.MeasureString(line+" "+word).Ceil() > maxWidth {
			lines = append(lines, line)
			line = word
			continue
		}
		line += " " + word
	}
	return append(lines, line)
}
