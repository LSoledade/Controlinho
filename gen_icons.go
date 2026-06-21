//go:build ignore

// Icon generator for the PC Remote PWA.
// Produces client/icon-192.png, client/icon-512.png and client/icon-maskable-512.png.
// Run with:  go run gen_icons.go
//
// Pure Go (no deps): draws a simple monitor glyph on a rounded dark background,
// so the PWA has real PNG icons that Android/iOS can use for "Add to Home Screen".
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

type canvas struct {
	w, h int
	px   []byte // RGBA
}

func newCanvas(w, h int) *canvas {
	return &canvas{w: w, h: h, px: make([]byte, w*h*4)}
}

func (c *canvas) set(x, y int, col color.RGBA) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	i := (y*c.w + x) * 4
	// source-over with whatever's there (simple blend on alpha)
	a := int(col.A)
	c.px[i+0] = byte((int(col.R)*a + int(c.px[i+0])*(255-a)) / 255)
	c.px[i+1] = byte((int(col.G)*a + int(c.px[i+1])*(255-a)) / 255)
	c.px[i+2] = byte((int(col.B)*a + int(c.px[i+2])*(255-a)) / 255)
	c.px[i+3] = byte(a + int(c.px[i+3])*(255-a)/255)
}

// fillRect draws a solid rectangle.
func (c *canvas) fillRect(x0, y0, x1, y1 int, col color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c.set(x, y, col)
		}
	}
}

// fillRoundRect draws a rectangle with rounded corners (radius r) and soft AA edge.
func (c *canvas) fillRoundRect(x0, y0, x1, y1, r int, col color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			// distance to nearest corner center for AA
			inside := true
			// check corners
			cx, cy := x, y
			if x < x0+r {
				cx = x0 + r
			} else if x > x1-r-1 {
				cx = x1 - r - 1
			} else {
				// not in a horizontal corner band; treat as fully inside horizontally
				cx = x0 + r + 1
			}
			if y < y0+r {
				cy = y0 + r
			} else if y > y1-r-1 {
				cy = y1 - r - 1
			} else {
				cy = y0 + r + 1
			}
			if (x < x0+r || x > x1-r-1) && (y < y0+r || y > y1-r-1) {
				d := math.Hypot(float64(x-cx), float64(y-cy))
				if d > float64(r) {
					inside = false
				} else if d > float64(r)-1 {
					// anti-alias edge
					alpha := uint8(255 * (float64(r) - d))
					tmp := col
					tmp.A = alpha
					c.set(x, y, tmp)
					continue
				}
			}
			if inside {
				c.set(x, y, col)
			}
		}
	}
}

// fillCircleAA draws an anti-aliased filled circle.
func (c *canvas) fillCircleAA(cx, cy, r float64, col color.RGBA) {
	x0 := int(cx - r - 1)
	x1 := int(cx + r + 1)
	y0 := int(cy - r - 1)
	y1 := int(cy + r + 1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			if d <= r-1 {
				c.set(x, y, col)
			} else if d < r {
				alpha := uint8(255 * (r - d))
				tmp := col
				tmp.A = alpha
				c.set(x, y, tmp)
			}
		}
	}
}

// lineThick draws a line of given thickness by stamping AA disks along it.
func (c *canvas) lineThick(x0, y0, x1, y1, thick float64, col color.RGBA) {
	// naive: sample a disk of radius thick/2 along the segment
	steps := int(math.Hypot(x1-x0, y1-y0)) * 2
	if steps < 1 {
		steps = 1
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x0 + t*(x1-x0)
		y := y0 + t*(y1-y0)
		c.fillCircleAA(x, y, thick/2, col)
	}
}

// pngBytes returns the canvas as 8-bit RGBA PNG bytes.
func (c *canvas) pngBytes() ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
	copy(img.Pix, c.px)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// encodePNG writes the canvas as an 8-bit RGBA PNG.
func (c *canvas) encodePNG(path string) error {
	b, err := c.pngBytes()
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// writeICO assembles a Windows .ico holding PNG-compressed images at the given
// sizes (Windows Vista+ reads PNG inside ICO). Used for the system-tray icon,
// which is embedded into the binary and handed to systray.SetIcon.
func writeICO(path string, sizes []int) error {
	type entry struct {
		size int
		png  []byte
	}
	var entries []entry
	for _, s := range sizes {
		b, err := drawIcon(s, false).pngBytes()
		if err != nil {
			return err
		}
		entries = append(entries, entry{s, b})
	}

	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // type: icon
	binary.Write(&buf, binary.LittleEndian, uint16(len(entries)))
	offset := 6 + 16*len(entries) // dir header + one entry each
	for _, e := range entries {
		dim := byte(e.size)
		if e.size >= 256 {
			dim = 0 // 0 means 256 in the ICONDIRENTRY
		}
		buf.WriteByte(dim)                                  // width
		buf.WriteByte(dim)                                  // height
		buf.WriteByte(0)                                    // palette size (0 = no palette)
		buf.WriteByte(0)                                    // reserved
		binary.Write(&buf, binary.LittleEndian, uint16(1))  // color planes
		binary.Write(&buf, binary.LittleEndian, uint16(32)) // bits per pixel
		binary.Write(&buf, binary.LittleEndian, uint32(len(e.png)))
		binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(e.png)
	}
	for _, e := range entries {
		buf.Write(e.png)
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

// drawIcon renders the PC Remote glyph into a size×size canvas.
// maskable=true uses a background that fills the whole canvas (safe zone for maskable).
func drawIcon(size int, maskable bool) *canvas {
	c := newCanvas(size, size)

	// Background: dark navy. For maskable we fill edge-to-edge; for regular we
	// leave it transparent and let the platform composite.
	bg := color.RGBA{0x0f, 0x11, 0x15, 0xff}
	if maskable {
		c.fillRect(0, 0, size, size, bg)
	} else {
		// rounded app tile
		r := size * 22 / 100
		c.fillRoundRect(0, 0, size, size, r, bg)
	}

	// Geometry scaled to size.
	s := float64(size)
	accent := color.RGBA{0x4f, 0x9c, 0xf9, 0xff} // #4f9cf9
	screen := color.RGBA{0x1f, 0x23, 0x2c, 0xff}
	stand := color.RGBA{0x9a, 0xa3, 0xb2, 0xff}

	// Monitor screen (rounded rect)
	marginX := s * 0.22
	top := s * 0.24
	bot := s * 0.66
	radius := s * 0.06
	c.fillRoundRect(int(marginX), int(top), int(s-marginX), int(bot), int(radius), screen)

	// Screen bezel highlight (top)
	c.fillRoundRect(int(marginX), int(top), int(s-marginX), int(top+s*0.04), int(radius), accent)

	// Stand
	standW := s * 0.06
	c.fillRect(int(s/2-standW/2), int(bot), int(s/2+standW/2), int(bot+s*0.12), stand)
	// base
	c.fillRect(int(s*0.34), int(bot+s*0.12), int(s*0.66), int(bot+s*0.16), stand)

	// "signal" arcs / play glyph inside screen: draw a small play triangle
	triCx := s * 0.5
	triCy := (top+bot)/2 + s*0.01
	triSize := s * 0.10
	// triangle pointing right
	p1 := [2]float64{triCx - triSize*0.5, triCy - triSize}
	p2 := [2]float64{triCx - triSize*0.5, triCy + triSize}
	p3 := [2]float64{triCx + triSize, triCy}
	c.lineThick(p1[0], p1[1], p3[0], p3[1], s*0.045, accent)
	c.lineThick(p3[0], p3[1], p2[0], p2[1], s*0.045, accent)
	c.lineThick(p2[0], p2[1], p1[0], p1[1], s*0.045, accent)

	return c
}

func main() {
	sizes := []struct {
		size     int
		path     string
		maskable bool
	}{
		{192, "client/icon-192.png", false},
		{512, "client/icon-512.png", false},
		{512, "client/icon-maskable-512.png", true},
	}
	for _, g := range sizes {
		c := drawIcon(g.size, g.maskable)
		if err := c.encodePNG(g.path); err != nil {
			panic(err)
		}
		// sanity: report file size
		fi, _ := os.Stat(g.path)
		println("wrote", g.path, int64ToString(fi.Size()), "bytes")
	}

	// Multi-size .ico for the Windows system-tray icon (embedded via client/).
	if err := writeICO("client/icon.ico", []int{16, 24, 32, 48, 64, 128, 256}); err != nil {
		panic(err)
	}
	fi, _ := os.Stat("client/icon.ico")
	println("wrote client/icon.ico", int64ToString(fi.Size()), "bytes")

	// MSIX (Microsoft Store) tile assets. BackgroundColor is "transparent" in
	// Package.appxmanifest, so each PNG carries its own (edge-to-edge) background —
	// that's the maskable=true variant. These are the logos the manifest references.
	if err := os.MkdirAll("packaging/Assets", 0755); err != nil {
		panic(err)
	}
	storeAssets := []struct {
		size int
		path string
	}{
		{50, "packaging/Assets/StoreLogo.png"},
		{44, "packaging/Assets/Square44x44Logo.png"},
		{150, "packaging/Assets/Square150x150Logo.png"},
	}
	for _, a := range storeAssets {
		if err := drawIcon(a.size, true).encodePNG(a.path); err != nil {
			panic(err)
		}
		fi, _ := os.Stat(a.path)
		println("wrote", a.path, int64ToString(fi.Size()), "bytes")
	}

	// Optional wide tile (310×150): the square glyph centered on a dark bar.
	if err := drawWide(310, 150).encodePNG("packaging/Assets/Wide310x150Logo.png"); err != nil {
		panic(err)
	}
	fi, _ = os.Stat("packaging/Assets/Wide310x150Logo.png")
	println("wrote packaging/Assets/Wide310x150Logo.png", int64ToString(fi.Size()), "bytes")
}

// drawWide renders the square glyph centered on a w×h dark background, for the
// optional MSIX wide tile.
func drawWide(w, h int) *canvas {
	out := newCanvas(w, h)
	out.fillRect(0, 0, w, h, color.RGBA{0x0f, 0x11, 0x15, 0xff})
	glyph := drawIcon(h, true)
	offX := (w - h) / 2
	for y := 0; y < h; y++ {
		for x := 0; x < h; x++ {
			i := (y*glyph.w + x) * 4
			out.set(offX+x, y, color.RGBA{glyph.px[i], glyph.px[i+1], glyph.px[i+2], glyph.px[i+3]})
		}
	}
	return out
}

func int64ToString(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
