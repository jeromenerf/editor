package drawutil

import (
	"image"
	"image/color"
	"image/draw"
	"sync"

	"github.com/jmigpin/editor/imageutil"

	"golang.org/x/image/math/fixed"
)

type StringDraw struct {
	liner       *StringLiner
	img         draw.Image
	bounds      *image.Rectangle
	cursorIndex int // set externally, use <0 to not draw the cursor
}

func NewStringDraw(img draw.Image, bounds *image.Rectangle, face *Face, str string) *StringDraw {
	max0 := bounds.Max.Sub(bounds.Min)
	max := PointToPoint266(&max0)
	liner := NewStringLiner(face, str, max)
	return &StringDraw{liner: liner, img: img, bounds: bounds}
}
func (sd *StringDraw) Loop(fn func() (fg, bg color.Color, ok bool)) {
	var wg sync.WaitGroup
	sd.liner.Loop(func() bool {
		fg, bg, ok := fn()
		if !ok {
			return false
		}

		// rune background
		if bg != nil {
			pb := Rect266ToRect(sd.liner.iter.PenBounds())
			dr := pb.Add(sd.bounds.Min).Intersect(*sd.bounds)
			imageutil.FillRectangle(sd.img, &dr, bg)
		}

		// cursor
		if !sd.liner.isWrapLineRune {
			if sd.liner.iter.ri == sd.cursorIndex {
				drawCursor(sd.img, sd.bounds, sd.liner)
			}
		}

		// rune foreground (glyph)
		wg.Add(1)
		go func(ru rune, pen fixed.Point26_6, fg color.Color) {
			defer wg.Done()
			penPoint := Point266ToPoint(&pen)
			dr, mask, maskp, _, ok := sd.liner.iter.face.Glyph(ru)
			if ok {
				dr = dr.Add(sd.bounds.Min).Add(*penPoint).Intersect(*sd.bounds)
				fgi := image.NewUniform(fg)
				draw.DrawMask(sd.img, dr, fgi, image.Point{}, mask, maskp, draw.Over)
			}
		}(sd.liner.iter.ru, sd.liner.iter.pen, fg)

		return true
	})
	wg.Wait()
}
func drawCursor(img draw.Image, bounds *image.Rectangle, liner *StringLiner) {
	pb := Rect266ToRect(liner.iter.PenBounds())
	dr := pb.Add(bounds.Min)

	r1 := dr
	r1.Min.X -= 1
	r1.Max.X = r1.Min.X + 3
	r1.Max.Y = r1.Min.Y + 3
	r1 = r1.Intersect(*bounds)
	imageutil.FillRectangle(img, &r1, &color.Black)

	r2 := dr
	r2.Min.X -= 1
	r2.Max.X = r2.Min.X + 3
	r2.Min.Y = r2.Max.Y - 3
	r2 = r2.Intersect(*bounds)
	imageutil.FillRectangle(img, &r2, &color.Black)

	r3 := dr
	r3.Max.X = r3.Min.X + 1
	r3 = r3.Intersect(*bounds)
	imageutil.FillRectangle(img, &r3, &color.Black)
}
