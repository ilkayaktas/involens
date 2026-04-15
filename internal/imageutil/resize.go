package imageutil

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/disintegration/imaging"
)

const DefaultMaxPx = 1568

// ResizeForLLM resizes imageData so the longest edge is at most maxPx pixels,
// then encodes the result as JPEG. Always returns "image/jpeg" as the mime type.
// If the image is already within bounds the original pixels are preserved but
// the format is still normalised to JPEG.
func ResizeForLLM(imageData []byte, mimeType string, maxPx int) ([]byte, string, error) {
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, "", fmt.Errorf("imageutil: decode image: %w", err)
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Resize if needed, preserving aspect ratio.
	out := img
	if w > maxPx || h > maxPx {
		if w >= h {
			out = imaging.Resize(img, maxPx, 0, imaging.Lanczos)
		} else {
			out = imaging.Resize(img, 0, maxPx, imaging.Lanczos)
		}
	}

	// Always encode output as JPEG regardless of input format.
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, out, imaging.JPEG); err != nil {
		return nil, "", fmt.Errorf("imageutil: encode jpeg: %w", err)
	}

	return buf.Bytes(), "image/jpeg", nil
}
