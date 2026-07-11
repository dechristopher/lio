package og

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io/fs"
	"math"
	"sync"

	"github.com/dechristopher/octad/v2"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/font"
)

// The card's board replicates the in-game octadground presentation 1:1 by
// compositing the same assets the game client uses: the green board theme's
// two square colors (res/img/board/green.svg), the alpha piece set SVGs
// (res/img/alpha/*.svg) rasterized per square, the Poppins-600 coordinate
// labels octadground.base.css draws at 4% board width / 0.8 opacity, the 38%
// amber last-move overlay, and the red radial check gradient from the board
// theme CSS.
const (
	boardPx  = 512 // rendered board edge; divisible by 4 for pixel-exact squares
	squarePx = boardPx / 4
)

// in-game overlay colors (see res/themes/board/green.css, dark tokens)
var (
	lastMoveColor = color.RGBA{0xfb, 0xbf, 0x24, 0xff} // --warn (dark), site draws it at 38%
	lastMoveAlpha = 0.38
)

// sprites holds the rasterized alpha piece set, keyed like the asset files
// ("wP" … "bK"), built once by LoadAssets.
var (
	spriteMu sync.RWMutex
	sprites  map[string]*image.RGBA
)

var pieceTypeLetter = map[octad.PieceType]string{
	octad.King:   "K",
	octad.Queen:  "Q",
	octad.Rook:   "R",
	octad.Bishop: "B",
	octad.Knight: "N",
	octad.Pawn:   "P",
}

// LoadAssets rasterizes the alpha piece set out of the served static
// filesystem (the same files the game client loads) for card composition.
// www.Serve calls it at startup right after the asset manifest build; cards
// cannot render before it succeeds.
func LoadAssets(fsys fs.FS) error {
	loaded := make(map[string]*image.RGBA, 12)
	for _, c := range []string{"w", "b"} {
		for _, t := range pieceTypeLetter {
			key := c + t
			raw, err := fs.ReadFile(fsys, "res/img/alpha/"+key+".svg")
			if err != nil {
				return fmt.Errorf("og: read piece asset %s: %w", key, err)
			}
			sprite, err := rasterizeSVG(raw, squarePx, squarePx)
			if err != nil {
				return fmt.Errorf("og: rasterize piece %s: %w", key, err)
			}
			loaded[key] = sprite
		}
	}
	spriteMu.Lock()
	sprites = loaded
	spriteMu.Unlock()
	return nil
}

// rasterizeSVG renders SVG bytes onto a transparent w×h canvas.
func rasterizeSVG(svg []byte, w, h int) (*image.RGBA, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(svg))
	if err != nil {
		return nil, err
	}
	icon.SetTarget(0, 0, float64(w), float64(h))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, img, img.Bounds())
	icon.Draw(rasterx.NewDasher(w, h, scanner), 1)
	return img, nil
}

// renderBoard composes the position exactly as the in-game board shows it
// (white orientation): theme squares, last-move and check overlays, coords,
// then the alpha piece sprites.
func renderBoard(ofen string, marks []octad.Square) (*image.RGBA, error) {
	spriteMu.RLock()
	pieceArt := sprites
	spriteMu.RUnlock()
	if pieceArt == nil {
		return nil, fmt.Errorf("og: assets not loaded (LoadAssets not run)")
	}

	var opts []func(*octad.Game)
	if ofen != "" {
		fn, err := octad.OFEN(ofen)
		if err != nil {
			return nil, fmt.Errorf("og: parse OFEN %q: %w", ofen, err)
		}
		opts = append(opts, fn)
	}
	g, err := octad.NewGame(opts...)
	if err != nil {
		return nil, fmt.Errorf("og: build game: %w", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, boardPx, boardPx))

	// theme squares
	for sq := octad.Square(0); sq < 16; sq++ {
		fill := darkSquare
		if isLightSquare(sq) {
			fill = lightSquare
		}
		draw.Draw(img, squareRect(sq), image.NewUniform(fill), image.Point{}, draw.Src)
	}

	// last-move overlay (site: color-mix(--warn 38%, transparent))
	for _, sq := range marks {
		if sq == octad.NoSquare {
			continue
		}
		overlay := withAlpha(lastMoveColor, lastMoveAlpha)
		draw.Draw(img, squareRect(sq), image.NewUniform(overlay), image.Point{}, draw.Over)
	}

	// red radial check gradient under the king in check
	position := g.Position()
	if position.InCheck() {
		if kingSq := findKing(position.Board(), position.Turn()); kingSq != octad.NoSquare {
			drawCheckGradient(img, squareRect(kingSq))
		}
	}

	drawCoords(img)

	// pieces, over everything (octadground z-order: overlays sit under pieces)
	boardMap := position.Board().SquareMap()
	for sq, p := range boardMap {
		key := p.Color().String() + pieceTypeLetter[p.Type()]
		if sprite := pieceArt[key]; sprite != nil {
			draw.Draw(img, squareRect(sq), sprite, image.Point{}, draw.Over)
		}
	}

	return img, nil
}

// squareRect maps a square to its pixel bounds in white orientation
// (a1 bottom-left, like the room board's default/spectator view).
func squareRect(sq octad.Square) image.Rectangle {
	x := int(sq.File()) * squarePx
	y := (3 - int(sq.Rank())) * squarePx
	return image.Rect(x, y, x+squarePx, y+squarePx)
}

// isLightSquare mirrors the green.svg pattern: a4 (top-left) is light and
// colors alternate from there.
func isLightSquare(sq octad.Square) bool {
	return (int(sq.File())+3-int(sq.Rank()))%2 == 0
}

func findKing(b *octad.Board, c octad.Color) octad.Square {
	for sq, p := range b.SquareMap() {
		if p.Type() == octad.King && p.Color() == c {
			return sq
		}
	}
	return octad.NoSquare
}

// drawCheckGradient replicates the board theme's square.check style:
// radial-gradient(ellipse at center, rgba(255,0,0,1) 0%, rgba(231,0,0,1) 25%,
// rgba(169,0,0,0) 89%, rgba(158,0,0,0) 100%), radius to the farthest corner.
func drawCheckGradient(img *image.RGBA, rect image.Rectangle) {
	cx := float64(rect.Min.X+rect.Max.X) / 2
	cy := float64(rect.Min.Y+rect.Max.Y) / 2
	// farthest-corner radius of a square cell: half the diagonal
	maxDist := math.Hypot(float64(rect.Dx()), float64(rect.Dy())) / 2

	type stop struct {
		t       float64
		r, g, b float64
		a       float64
	}
	stops := []stop{
		{0.00, 255, 0, 0, 1},
		{0.25, 231, 0, 0, 1},
		{0.89, 169, 0, 0, 0},
		{1.00, 158, 0, 0, 0},
	}

	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			t := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) / maxDist
			if t > 1 {
				t = 1
			}
			// find the surrounding stops and interpolate
			var s0, s1 stop
			for i := 1; i < len(stops); i++ {
				if t <= stops[i].t {
					s0, s1 = stops[i-1], stops[i]
					break
				}
			}
			f := (t - s0.t) / (s1.t - s0.t)
			cr := s0.r + (s1.r-s0.r)*f
			cg := s0.g + (s1.g-s0.g)*f
			cb := s0.b + (s1.b-s0.b)*f
			ca := s0.a + (s1.a-s0.a)*f
			if ca <= 0 {
				continue
			}
			over := color.RGBA{
				uint8(cr + 0.5), uint8(cg + 0.5), uint8(cb + 0.5),
				uint8(ca*255 + 0.5),
			}
			blendPixel(img, x, y, over)
		}
	}
}

// withAlpha returns c at the given opacity as a properly alpha-premultiplied
// color.RGBA (what image/draw and font.Drawer expect; passing straight
// component values with a reduced A produces out-of-range channels and wildly
// wrong hues).
func withAlpha(c color.RGBA, alpha float64) color.RGBA {
	return color.RGBA{
		uint8(float64(c.R)*alpha + 0.5),
		uint8(float64(c.G)*alpha + 0.5),
		uint8(float64(c.B)*alpha + 0.5),
		uint8(255*alpha + 0.5),
	}
}

// blendPixel source-over composites c onto the pixel at (x, y).
func blendPixel(img *image.RGBA, x, y int, c color.RGBA) {
	dst := img.RGBAAt(x, y)
	a := float64(c.A) / 255
	img.SetRGBA(x, y, color.RGBA{
		uint8(float64(c.R)*a + float64(dst.R)*(1-a) + 0.5),
		uint8(float64(c.G)*a + float64(dst.G)*(1-a) + 0.5),
		uint8(float64(c.B)*a + float64(dst.B)*(1-a) + 0.5),
		255,
	})
}

// drawCoords draws the rank/file labels exactly as octadground.base.css
// positions them (white orientation): Poppins 600 at 4% of board width, 0.8
// opacity, ranks up the left edge (top-left of each cell), files along the
// bottom (bottom-right of each cell), alternating the two square colors so a
// label always contrasts its square (see the board theme css nth-child rules).
func drawCoords(img *image.RGBA) {
	if faceCoord == nil {
		return
	}
	// 0.5cqw ≈ boardPx/200 side padding; 4cqw font ≈ boardPx/25
	sidePad := boardPx / 200
	metrics := faceCoord.Metrics()
	ascent := metrics.Ascent.Ceil()
	descent := metrics.Descent.Ceil()

	coordColor := func(odd bool) color.Color {
		// nth-child(odd) → light color, nth-child(even) → dark color, with
		// the 0.8 opacity octadground applies to the whole coords layer
		c := darkSquare
		if odd {
			c = lightSquare
		}
		return withAlpha(c, 0.8)
	}

	// ranks 1–4 bottom-to-top on the left edge, top-aligned in each cell
	for r := 0; r < 4; r++ {
		label := string(rune('1' + r))
		y := (3-r)*squarePx + ascent + sidePad/2
		drawString(img, faceCoord, coordColor(r%2 == 0), sidePad, y, label)
	}

	// files a–d along the bottom edge, right-aligned in each cell
	d := font.Drawer{Face: faceCoord}
	for f := 0; f < 4; f++ {
		label := string(rune('a' + f))
		w := d.MeasureString(label).Ceil()
		x := (f+1)*squarePx - w - sidePad
		y := boardPx - descent
		drawString(img, faceCoord, coordColor(f%2 == 0), x, y, label)
	}
}
