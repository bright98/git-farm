package farm

import (
	"bytes"
	"image/png"
	"testing"
)

func drawPNG(t *testing.T, o PNGOptions) *bytes.Buffer {
	t.Helper()
	var b bytes.Buffer
	if err := sample().WritePNG(&b, o); err != nil {
		t.Fatal(err)
	}
	return &b
}

// The whole point of the file is that a card renderer can decode it, so decode
// it — and check it came out at the size that was asked for, since a preview
// card is cropped against its dimensions.
func TestPNGDecodesAtTheAskedForSize(t *testing.T) {
	b := drawPNG(t, PNGOptions{Theme: &Full, Cols: 190, Rows: 50, Scale: 10})

	img, err := png.Decode(bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatalf("the file does not decode as a PNG: %v", err)
	}
	got := img.Bounds().Size()
	if got.X != 1900 || got.Y != 1000 {
		t.Errorf("got %dx%d, want 1900x1000", got.X, got.Y)
	}
}

// One canvas pixel is a square of PNG pixels, filled in rather than resampled.
// A blurred edge would be invisible in a size check and is the whole difference
// between pixel art and a smudge, so look at the square itself: every pixel
// inside one has to be the same colour as its corner.
func TestPNGDoesNotResample(t *testing.T) {
	const scale = 8
	b := drawPNG(t, PNGOptions{Theme: &Full, Cols: 120, Rows: 50, Scale: scale})

	img, err := png.Decode(bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range [][2]int{{0, 0}, {40, 60}, {119, 99}} {
		x, y := at[0]*scale, at[1]*scale
		want := img.At(x, y)
		for dy := 0; dy < scale; dy++ {
			for dx := 0; dx < scale; dx++ {
				if got := img.At(x+dx, y+dy); got != want {
					t.Fatalf("canvas pixel %d,%d is not one flat square: %v at +%d,+%d, %v at its corner",
						at[0], at[1], got, dx, dy, want)
				}
			}
		}
	}
}

// The Action and the site both rely on a re-run over an unchanged repository
// writing the same bytes; writeIfChanged turns that into a genuine no-op.
func TestPNGIsDeterministic(t *testing.T) {
	o := PNGOptions{Theme: &Full, Cols: 120, Rows: 50, Scale: 4}
	if a, b := drawPNG(t, o).Bytes(), drawPNG(t, o).Bytes(); !bytes.Equal(a, b) {
		t.Error("two runs over the same scene wrote different bytes")
	}
}

// A PNG has no character overlay to put names in, so asking for the quiet
// theme's hairline fences gets a farm with no fences at all — which is the one
// claim the README says must never be wrong in public. The default theme is
// therefore the caller's problem, and the CLI hands it Full; all this can
// promise is that it draws something either way.
func TestPNGDrawsWithAQuietTheme(t *testing.T) {
	if b := drawPNG(t, PNGOptions{Theme: &Quiet, Cols: 120, Rows: 50, Scale: 4}); b.Len() == 0 {
		t.Error("the quiet theme wrote an empty file")
	}
}
