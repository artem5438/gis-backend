package parcels

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"
)

// chartBox keeps the picture's own proportions and the same size in PDF and DOCX.
func chartBox(pxW, pxH int) (float64, float64) {
	const maxW = 110.0
	const maxH = 80.0
	if pxW <= 0 || pxH <= 0 {
		return maxW, 40
	}
	width := maxW
	height := maxW * float64(pxH) / float64(pxW)
	if height > maxH {
		height = maxH
		width = maxH * float64(pxW) / float64(pxH)
	}
	return width, height
}

func circlePNG(chart reportChart, donut bool) ([]byte, int, int, error) {
	const size = 260
	canvas := image.NewRGBA(image.Rect(0, 0, size, size))
	fill(canvas, color.RGBA{255, 255, 255, 255})
	total := 0.0
	for _, slice := range chart.Slices {
		if slice.Value > 0 {
			total += slice.Value
		}
	}
	if total <= 0 {
		total = 1
	}
	center := float64(size) / 2
	outer := center - 8
	inner := 0.0
	if donut {
		inner = outer * 0.55
	}
	start := -math.Pi / 2
	for _, slice := range chart.Slices {
		if slice.Value <= 0 {
			continue
		}
		sweep := 2 * math.Pi * slice.Value / total
		shade := parseHex(slice.Color)
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				dx := float64(x) - center
				dy := float64(y) - center
				radius := math.Hypot(dx, dy)
				if radius > outer || radius < inner {
					continue
				}
				angle := math.Atan2(dy, dx)
				if angle < start {
					angle += 2 * math.Pi
				}
				if angle >= start && angle < start+sweep {
					canvas.SetRGBA(x, y, shade)
				}
			}
		}
		start += sweep
	}
	png, err := encodePNG(canvas)
	return png, size, size, err
}

func barStrip(value, max float64, hex string) []byte {
	const width = 400
	const height = 16
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	fill(canvas, color.RGBA{236, 236, 236, 255})
	paintRect(canvas, 0, 0, barLength(value, max, width), height, parseHex(hex))
	png, err := encodePNG(canvas)
	if err != nil {
		return nil
	}
	return png
}

func chartMax(chart reportChart) float64 {
	maxValue := 0.0
	for _, slice := range chart.Slices {
		if slice.Value > maxValue {
			maxValue = slice.Value
		}
	}
	if maxValue <= 0 {
		return 1
	}
	return maxValue
}

func barLength(value, max float64, room int) int {
	length := int((value / max) * float64(room))
	if length < 4 && value > 0 {
		return 4
	}
	return length
}

func encodePNG(canvas *image.RGBA) ([]byte, error) {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func fill(canvas *image.RGBA, shade color.RGBA) {
	bounds := canvas.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			canvas.SetRGBA(x, y, shade)
		}
	}
}

func paintRect(canvas *image.RGBA, x, y, width, height int, shade color.RGBA) {
	bounds := canvas.Bounds()
	for py := y; py < y+height && py < bounds.Max.Y; py++ {
		for px := x; px < x+width && px < bounds.Max.X; px++ {
			if px >= 0 && py >= 0 {
				canvas.SetRGBA(px, py, shade)
			}
		}
	}
}

func parseHex(value string) color.RGBA {
	text := strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(text) != 6 {
		return color.RGBA{20, 20, 20, 255}
	}
	parsed, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return color.RGBA{20, 20, 20, 255}
	}
	return color.RGBA{uint8(parsed >> 16), uint8(parsed >> 8), uint8(parsed), 255}
}
