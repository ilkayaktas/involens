package imageutil

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// makeJPEG creates a minimal JPEG image of size w x h filled with the given color.
func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 100, G: 149, B: 237, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("makeJPEG: encode: %v", err)
	}
	return buf.Bytes()
}

func imageSize(t *testing.T, data []byte) (w, h int) {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("imageSize: decode: %v", err)
	}
	b := img.Bounds()
	return b.Dx(), b.Dy()
}

func TestResizeForLLM_AlreadyWithinBounds(t *testing.T) {
	// 100x100 is well within 1568px — no resize should occur, but format
	// is always normalised to JPEG so bytes may differ from input.
	original := makeJPEG(t, 100, 100)

	out, mime, err := ResizeForLLM(original, "image/jpeg", DefaultMaxPx)
	if err != nil {
		t.Fatalf("ResizeForLLM returned error: %v", err)
	}

	if mime != "image/jpeg" {
		t.Errorf("mime = %q, want %q", mime, "image/jpeg")
	}

	// Dimensions must be preserved.
	w, h := imageSize(t, out)
	if w != 100 || h != 100 {
		t.Errorf("expected 100x100, got %dx%d", w, h)
	}
}

func TestResizeForLLM_LargeImageGetsResized(t *testing.T) {
	// 3000x2000 exceeds 1568px on the longest edge.
	original := makeJPEG(t, 3000, 2000)

	out, mime, err := ResizeForLLM(original, "image/jpeg", DefaultMaxPx)
	if err != nil {
		t.Fatalf("ResizeForLLM returned error: %v", err)
	}

	if mime != "image/jpeg" {
		t.Errorf("mime = %q, want %q", mime, "image/jpeg")
	}

	w, h := imageSize(t, out)

	// Longest edge must be at most maxPx.
	if w > DefaultMaxPx || h > DefaultMaxPx {
		t.Errorf("resized image %dx%d exceeds maxPx=%d", w, h, DefaultMaxPx)
	}

	// Aspect ratio should be preserved (approximately): original is 3:2.
	// After resize: longest edge = 1568, shorter edge ~= 1045.
	if w != DefaultMaxPx {
		t.Errorf("expected width = %d (longest edge), got %d", DefaultMaxPx, w)
	}

	// The output should be different from the original.
	if bytes.Equal(out, original) {
		t.Error("expected resized output to differ from original")
	}
}

func TestResizeForLLM_TallImageGetsResized(t *testing.T) {
	// 1000x3000: height is the longest edge.
	original := makeJPEG(t, 1000, 3000)

	out, mime, err := ResizeForLLM(original, "image/jpeg", DefaultMaxPx)
	if err != nil {
		t.Fatalf("ResizeForLLM returned error: %v", err)
	}

	if mime != "image/jpeg" {
		t.Errorf("mime = %q, want %q", mime, "image/jpeg")
	}

	w, h := imageSize(t, out)

	if w > DefaultMaxPx || h > DefaultMaxPx {
		t.Errorf("resized image %dx%d exceeds maxPx=%d", w, h, DefaultMaxPx)
	}

	if h != DefaultMaxPx {
		t.Errorf("expected height = %d (longest edge), got %d", DefaultMaxPx, h)
	}
}

func TestResizeForLLM_PNGBecomesJPEG(t *testing.T) {
	// Even though the input mime is image/png, output should always be image/jpeg.
	// We still supply a JPEG-encoded image here because makeJPEG uses jpeg.Encode —
	// image.Decode supports it regardless of the reported mime type.
	original := makeJPEG(t, 3000, 3000)

	_, mime, err := ResizeForLLM(original, "image/png", DefaultMaxPx)
	if err != nil {
		t.Fatalf("ResizeForLLM returned error: %v", err)
	}
	if mime != "image/jpeg" {
		t.Errorf("mime = %q, want %q", mime, "image/jpeg")
	}
}
