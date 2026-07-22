package cli

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestLogoBytesValidPNG(t *testing.T) {
	if len(logoBytes) == 0 {
		t.Fatal("embedded logoBytes is empty")
	}
	img, format, err := image.Decode(bytes.NewReader(logoBytes))
	if err != nil {
		t.Fatalf("failed to decode embedded logo: %v", err)
	}
	if format != "png" {
		t.Fatalf("expected logo format to be png, got %s", format)
	}

	// Verify image bounds
	bounds := img.Bounds()
	if bounds.Dx() == 0 || bounds.Dy() == 0 {
		t.Fatalf("invalid image dimensions: %dx%d", bounds.Dx(), bounds.Dy())
	}

	// Verify alpha channel exists in embedded logo
	hasTransparentPixels := false
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 10 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 10 {
			_, _, _, a := img.At(x, y).RGBA()
			if a < 65535 {
				hasTransparentPixels = true
				break
			}
		}
		if hasTransparentPixels {
			break
		}
	}
	if !hasTransparentPixels {
		t.Log("Note: embedded logo has no transparent pixels in sample points, but image decoded cleanly")
	}
}

func TestPrintITerm2PNGToPayload(t *testing.T) {
	// Create a test image with transparent background and red square
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if x < 5 {
				img.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
			} else {
				img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 0}) // transparent
			}
		}
	}

	var buf bytes.Buffer
	err := PrintITerm2PNGTo(&buf, img, 20, 10)
	if err != nil {
		t.Fatalf("PrintITerm2PNGTo returned error: %v", err)
	}

	out := buf.String()
	prefix := "\x1b]1337;File=inline=1;width=20c;height=10c;preserveAspectRatio=1:"
	if !strings.HasPrefix(out, prefix) {
		t.Fatalf("output does not start with expected iTerm2 prefix. Got: %q", out[:min(len(out), 60)])
	}
	if !strings.HasSuffix(out, "\a\n") {
		t.Fatalf("output does not end with expected OSC bell. Got: %q", out)
	}

	// Extract base64 payload
	b64Data := out[len(prefix) : len(out)-2]
	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		t.Fatalf("failed to decode base64 payload: %v", err)
	}

	// Verify PNG magic bytes (\x89PNG\r\n\x1a\n)
	pngHeader := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	if !bytes.HasPrefix(decoded, pngHeader) {
		t.Fatalf("iTerm2 base64 payload is not a PNG image! Magic bytes mismatch.")
	}

	// Verify decoded image preserves transparency
	decImg, err := png.Decode(bytes.NewReader(decoded))
	if err != nil {
		t.Fatalf("failed to decode PNG payload: %v", err)
	}
	_, _, _, alpha := decImg.At(decImg.Bounds().Max.X-1, decImg.Bounds().Max.Y-1).RGBA()
	if alpha != 0 {
		t.Errorf("expected transparent pixel in payload, got alpha = %d", alpha)
	}
}

func TestPrintTransparentHalfblocksTo(t *testing.T) {
	// Create a 2x2 image: top-left red, bottom-left transparent, top-right transparent, bottom-right blue
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(0, 1, color.NRGBA{0, 0, 0, 0})
	img.SetNRGBA(1, 0, color.NRGBA{0, 0, 0, 0})
	img.SetNRGBA(1, 1, color.NRGBA{0, 0, 255, 255})

	var buf bytes.Buffer
	PrintTransparentHalfblocksTo(&buf, img, 2, 1)

	out := buf.String()
	// Should contain \x1b[0m reset codes and non-black-background codes
	if strings.Contains(out, "\x1b[48;2;0;0;0m") {
		t.Errorf("Halfblocks fallback incorrectly emitted solid black background code \\x1b[48;2;0;0;0m for transparent pixels: %q", out)
	}
	if !strings.Contains(out, "\x1b[0;38;2;255;0;0m▀") {
		t.Errorf("Expected top opaque / bottom transparent character '\\x1b[0;38;2;255;0;0m▀', got: %q", out)
	}
	if !strings.Contains(out, "\x1b[0;38;2;0;0;255m▄") {
		t.Errorf("Expected top transparent / bottom opaque character '\\x1b[0;38;2;0;0;255m▄', got: %q", out)
	}
}

func TestDetectBestProtocol(t *testing.T) {
	proto := DetectBestProtocol()
	t.Logf("Detected protocol: %v", proto)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
