package cli

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"io"

	"github.com/yourorg/qsync/internal/output"
)

//go:embed logo.png
var logoBytes []byte

// printLogo prints the embedded logo.png as truecolor ANSI blocks
// if the output writer is a terminal.
func printLogo(w io.Writer) {
	if !output.IsTTY.IsTerminal() {
		return
	}

	img, _, err := image.Decode(bytes.NewReader(logoBytes))
	if err != nil {
		// Fallback silently if image decoding fails
		return
	}

	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	// 40 columns wide, 20 rows of half-blocks (40x40 pixel resolution grid)
	targetW := 40
	targetH := 20

	for y := 0; y < targetH; y++ {
		fmt.Fprint(w, "  ")
		for x := 0; x < targetW; x++ {
			srcX1 := bounds.Min.X + (x*srcW)/targetW
			srcY1 := bounds.Min.Y + ((2*y)*srcH)/(2*targetH)

			srcX2 := bounds.Min.X + (x*srcW)/targetW
			srcY2 := bounds.Min.Y + ((2*y+1)*srcH)/(2*targetH)

			c1 := img.At(srcX1, srcY1)
			c2 := img.At(srcX2, srcY2)

			r1, g1, b1, a1 := rgba(c1)
			r2, g2, b2, a2 := rgba(c2)

			topColored := a1 >= 128
			botColored := a2 >= 128

			if !topColored && !botColored {
				fmt.Fprint(w, "\x1b[0m ")
			} else if topColored && !botColored {
				fmt.Fprintf(w, "\x1b[0m\x1b[38;2;%d;%d;%dm▀", r1, g1, b1)
			} else if !topColored && botColored {
				fmt.Fprintf(w, "\x1b[0m\x1b[38;2;%d;%d;%dm▄", r2, g2, b2)
			} else {
				fmt.Fprintf(w, "\x1b[48;2;%d;%d;%dm\x1b[38;2;%d;%d;%dm▄", r1, g1, b1, r2, g2, b2)
			}
		}
		fmt.Fprintln(w, "\x1b[0m")
	}
	fmt.Fprintln(w)
}

func rgba(c color.Color) (r, g, b, a uint32) {
	r32, g32, b32, a32 := c.RGBA()
	return r32 >> 8, g32 >> 8, b32 >> 8, a32 >> 8
}
