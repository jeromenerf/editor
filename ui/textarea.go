package ui

import (
	"image"

	"github.com/BurntSushi/xgbutil/xcursor"
	"github.com/jmigpin/editor/drawutil2/hsdrawer"
	"github.com/jmigpin/editor/drawutil2/loopers"
	"github.com/jmigpin/editor/imageutil"
	"github.com/jmigpin/editor/ui/tautil"
	"github.com/jmigpin/editor/uiutil"
	"github.com/jmigpin/editor/xgbutil/evreg"
	"github.com/jmigpin/editor/xgbutil/xinput"

	"golang.org/x/image/math/fixed"
)

type TextArea struct {
	C  uiutil.Container
	ui *UI

	drawer *hsdrawer.HSDrawer

	EvReg   *evreg.Register
	evUnreg evreg.Unregister

	editHistory   *tautil.EditHistory
	edit          *tautil.EditHistoryEdit
	buttonPressed bool
	boundsChange  image.Rectangle

	str         string
	cursorIndex int
	offsetY     fixed.Int26_6
	selection   struct {
		on    bool
		index int // from index to cursorIndex
	}

	Colors                     *hsdrawer.Colors
	DisableHighlightCursorWord bool
	DisablePageUpDown          bool

	drawerWidth int
}

func NewTextArea(ui *UI) *TextArea {
	ta := &TextArea{ui: ui}
	ta.drawer = hsdrawer.NewHSDrawer(ui.FontFace())
	c := hsdrawer.DefaultColors
	ta.Colors = &c
	ta.C.PaintFunc = ta.paint
	ta.C.OnCalcFunc = ta.onContainerCalc
	ta.EvReg = evreg.NewRegister()
	ta.editHistory = tautil.NewEditHistory(40)

	r1 := ta.ui.EvReg.Add(xinput.KeyPressEventId,
		&evreg.Callback{ta.onKeyPress})
	r2 := ta.ui.EvReg.Add(xinput.ButtonPressEventId,
		&evreg.Callback{ta.onButtonPress})
	r3 := ta.ui.EvReg.Add(xinput.ButtonReleaseEventId,
		&evreg.Callback{ta.onButtonRelease})
	r4 := ta.ui.EvReg.Add(xinput.MotionNotifyEventId,
		&evreg.Callback{ta.onMotionNotify})
	r5 := ta.ui.EvReg.Add(xinput.DoubleClickEventId,
		&evreg.Callback{ta.onDoubleClick})
	r6 := ta.ui.EvReg.Add(xinput.TripleClickEventId,
		&evreg.Callback{ta.onTripleClick})
	ta.evUnreg.Add(r1, r2, r3, r4, r5, r6)

	return ta
}
func (ta *TextArea) Close() {
	ta.evUnreg.UnregisterAll()
}
func (ta *TextArea) Bounds() *image.Rectangle {
	return &ta.C.Bounds
}

func (ta *TextArea) drawerMeasure(width int) {
	if ta.str != ta.drawer.Str || ta.drawerWidth != width {
		ta.drawer.Str = ta.str
		ta.drawerWidth = width

		max := image.Point{width, 1000000}
		ta.drawer.Measure(&max)
	}
}

func (ta *TextArea) onContainerCalc() {
	ta.updateStringCacheWithBoundsChangedCheck()
}
func (ta *TextArea) updateStringCacheWithBoundsChangedCheck() {
	// check if bounds have changed to emit event
	changed := false
	offsetIndex := 0
	if !ta.C.Bounds.Eq(ta.boundsChange) {
		changed = true
		ta.boundsChange = ta.C.Bounds
		offsetIndex = ta.OffsetIndex()
	}

	ta.updateStringCache()

	if changed {
		// set offset to keep the same first line while resizing
		ta.SetOffsetIndex(offsetIndex)

		ev := &TextAreaBoundsChangeEvent{ta}
		ta.EvReg.RunCallbacks(TextAreaBoundsChangeEventId, ev)
	}
}
func (ta *TextArea) updateStringCache() {
	ta.drawerMeasure(ta.C.Bounds.Dx())
}
func (ta *TextArea) StrHeight() fixed.Int26_6 {
	h := ta.drawer.Height()
	min := ta.LineHeight()
	if h < min {
		h = min
	}
	return h
}

// Used externally for dynamic textarea height.
func (ta *TextArea) CalcStringHeight(width int) int {
	ta.drawerMeasure(width)
	return ta.StrHeight().Round()
}

func (ta *TextArea) paint() {
	// fill background
	imageutil.FillRectangle(ta.ui.Image(), &ta.C.Bounds, ta.Colors.Normal.Bg)

	d := ta.drawer
	d.CursorIndex = ta.cursorIndex
	d.HWordIndex = ta.cursorIndex
	d.OffsetY = ta.offsetY
	d.Colors = ta.Colors
	d.Selection = ta.getDrawSelection()
	d.Draw(ta.ui.Image(), &ta.C.Bounds)
}
func (ta *TextArea) getDrawSelection() *loopers.SelectionIndexes {
	if ta.SelectionOn() {
		return &loopers.SelectionIndexes{
			Start: ta.SelectionIndex(),
			End:   ta.CursorIndex(),
		}
	}
	return nil
}

func (ta *TextArea) Str() string {
	if ta.edit != nil {
		// return edit str while editing
		return ta.edit.Str()
	}
	return ta.str
}

// No events, clears, or undos.
func (ta *TextArea) SetRawStr(s string) {
	if s == ta.str {
		return
	}

	ta.str = s

	// ensure valid indexes
	ta.SetCursorIndex(ta.CursorIndex())
	ta.SetSelectionIndex(ta.SelectionIndex())

	ta.updateStringCache()
	ta.C.NeedPaint()
}

func (ta *TextArea) setStr(s string) {
	if s == ta.str {
		return
	}

	oldBounds := ta.C.Bounds

	ta.SetRawStr(s)

	ev := &TextAreaSetStrEvent{ta, oldBounds}
	ta.EvReg.RunCallbacks(TextAreaSetStrEventId, ev)
}

// TODO: have a set str, and a clear func
func (ta *TextArea) SetStrClear(str string, clearPosition, clearUndoQ bool) {
	ta.SetSelectionOff()
	if clearPosition {
		ta.SetCursorIndex(0)
		ta.SetOffsetY(0)
	}
	if clearUndoQ {
		ta.editHistory.ClearQ()
		ta.setStr(str)
	} else {
		// replace string with edit to allow undo
		ta.EditOpen()
		ta.EditDelete(0, len(ta.Str()))
		ta.EditInsert(0, str)
		ta.EditClose()
	}
}

func (ta *TextArea) EditOpen() {
	if ta.edit != nil {
		panic("edit already exists")
	}
	ta.edit = tautil.NewEditHistoryEdit(ta.Str())
}
func (ta *TextArea) EditInsert(index int, str string) {
	ta.edit.Insert(index, str)
}
func (ta *TextArea) EditDelete(index, index2 int) {
	ta.edit.Delete(index, index2)
}
func (ta *TextArea) EditClose() {
	str, strEdit, ok := ta.edit.Close()
	ta.edit = nil
	if !ok {
		return
	}
	ta.editHistory.PushEdit(strEdit)
	ta.setStr(str)
}

func (ta *TextArea) popUndo() {
	s, i, ok := ta.editHistory.PopUndo(ta.Str())
	if !ok {
		return
	}
	ta.setStr(s)
	ta.SetCursorIndex(i)
	ta.SetSelectionOff()
}
func (ta *TextArea) unpopRedo() {
	s, i, ok := ta.editHistory.UnpopRedo(ta.Str())
	if !ok {
		return
	}
	ta.setStr(s)
	ta.SetCursorIndex(i)
	ta.SetSelectionOff()
}

func (ta *TextArea) CursorIndex() int {
	return ta.cursorIndex
}
func (ta *TextArea) SetCursorIndex(v int) {
	v = ta.validIndex(v)
	if v != ta.cursorIndex {
		ta.cursorIndex = v
		ta.validateSelection()
		ta.makeIndexVisible(v)
		ta.C.NeedPaint()
	}
}
func (ta *TextArea) SelectionIndex() int {
	return ta.selection.index
}
func (ta *TextArea) SetSelectionIndex(v int) {
	v = ta.validIndex(v)
	if v != ta.selection.index {
		ta.selection.index = v
		ta.validateSelection()
		ta.C.NeedPaint()
	}
}
func (ta *TextArea) SetSelection(si, ci int) {
	ta.SetSelectionIndex(si)
	ta.SetCursorIndex(ci)
	ta.setSelectionOn(ta.somethingSelected())
}

func (ta *TextArea) SelectionOn() bool {
	return ta.selection.on && ta.somethingSelected()
}
func (ta *TextArea) SetSelectionOff() {
	ta.setSelectionOn(false)
}
func (ta *TextArea) setSelectionOn(v bool) {
	if v != ta.selection.on {
		ta.selection.on = v
		ta.C.NeedPaint()
	}
}

func (ta *TextArea) validIndex(v int) int {
	if v < 0 {
		v = 0
	} else if v > len(ta.Str()) {
		v = len(ta.Str())
	}
	return v
}
func (ta *TextArea) validateSelection() {
	if !ta.somethingSelected() {
		ta.SetSelectionOff()
	}
}
func (ta *TextArea) somethingSelected() bool {
	si := ta.SelectionIndex()
	ci := ta.CursorIndex()
	return si != ci
}

func (ta *TextArea) OffsetY() fixed.Int26_6 {
	return ta.offsetY
}
func (ta *TextArea) SetOffsetY(v fixed.Int26_6) {
	if v < 0 {
		v = 0
	}
	if v > ta.StrHeight() {
		v = ta.StrHeight()
	}
	if v != ta.offsetY {
		ta.offsetY = v
		ta.C.NeedPaint()

		ev := &TextAreaSetOffsetYEvent{ta}
		ta.EvReg.RunCallbacks(TextAreaSetOffsetYEventId, ev)
	}
}

func (ta *TextArea) OffsetIndex() int {
	p := fixed.Point26_6{X: 0, Y: ta.offsetY}
	return ta.drawer.GetIndex(&p)
}
func (ta *TextArea) SetOffsetIndex(i int) {
	p := ta.drawer.GetPoint(i)
	ta.SetOffsetY(p.Y)
}
func (ta *TextArea) makeIndexVisible(index int) {
	y0 := ta.OffsetY()
	y1 := y0 + fixed.I(ta.C.Bounds.Dy())

	// is all visible
	a0 := ta.drawer.GetPoint(index).Y
	a1 := a0 + ta.LineHeight()
	if a0 >= y0 && a1 <= y1 {
		return
	}

	// is partially visible
	if y0 >= a0 && y0 <= a1 {
		// partially visible at top
		ta.SetOffsetY(a0)
		return
	}
	if y1 >= a0 && y1 <= a1 {
		// partially visible at bottom
		sy := fixed.I(ta.C.Bounds.Dy())
		ta.SetOffsetY(a0 - sy + ta.LineHeight())
		return
	}

	// set at half bounds
	half := fixed.I(ta.C.Bounds.Dy() / 2)
	ta.SetOffsetY(a0 - half)
}

func (ta *TextArea) MakeIndexVisibleAtCenter(index int) {
	// set at half bounds
	p0 := ta.drawer.GetPoint(index).Y
	half := fixed.I(ta.C.Bounds.Dy() / 2)
	offsetY := p0 - half
	ta.SetOffsetY(offsetY)
}
func (ta *TextArea) WarpPointerToIndexIfVisible(index int) {
	p := ta.drawer.GetPoint(index)
	p.Y -= ta.OffsetY()
	p2 := &image.Point{p.X.Round(), p.Y.Round()}
	p3 := p2.Add(ta.C.Bounds.Min)

	// padding
	p3.Y += ta.LineHeight().Round() - 1
	p3.X += 5

	if !p3.In(ta.C.Bounds) {
		return
	}
	ta.ui.WarpPointer(&p3)
}

func (ta *TextArea) RequestPrimaryPaste() (string, error) {
	return ta.ui.RequestPrimaryPaste()
}
func (ta *TextArea) RequestClipboardPaste() (string, error) {
	return ta.ui.RequestClipboardPaste()
}

func (ta *TextArea) SetClipboardCopy(v string) {
	ta.ui.SetClipboardCopy(v)
}
func (ta *TextArea) SetPrimaryCopy(v string) {
	ta.ui.SetPrimaryCopy(v)
}

func (ta *TextArea) LineHeight() fixed.Int26_6 {
	return ta.drawer.LineHeight()
}
func (ta *TextArea) IndexPoint(i int) *fixed.Point26_6 {
	return ta.drawer.GetPoint(i)
}
func (ta *TextArea) PointIndex(p *fixed.Point26_6) int {
	return ta.drawer.GetIndex(p)
}

func (ta *TextArea) PageUp() {
	if ta.DisablePageUpDown {
		return
	}
	tautil.PageUp(ta)
}
func (ta *TextArea) PageDown() {
	if ta.DisablePageUpDown {
		return
	}
	tautil.PageDown(ta)
}

func (ta *TextArea) onButtonPress(ev0 interface{}) {
	ev := ev0.(*xinput.ButtonPressEvent)
	if !ev.Point.In(ta.C.Bounds) {
		return
	}

	ta.buttonPressed = true
	switch {
	case ev.Button.Button(1):
		switch {
		case ev.Button.Mods.IsShift():
			tautil.MoveCursorToPoint(ta, ev.Point, true)
		default:
			tautil.MoveCursorToPoint(ta, ev.Point, false)
		}
	case ev.Button.Button(3) && ev.Button.Mods.IsNone():
		ta.ui.CursorMan.SetCursor(xcursor.Hand2)
	case ev.Button.Button(4):
		canScroll := !ta.DisablePageUpDown
		if canScroll {
			tautil.ScrollUp(ta)
		}
	case ev.Button.Button(5):
		canScroll := !ta.DisablePageUpDown
		if canScroll {
			tautil.ScrollDown(ta)
		}
	}
}
func (ta *TextArea) onMotionNotify(ev0 interface{}) {
	if !ta.buttonPressed {
		return
	}
	ev := ev0.(*xinput.MotionNotifyEvent)
	if ev.Mods.IsButton(1) {
		tautil.MoveCursorToPoint(ta, ev.Point, true)
	}
}
func (ta *TextArea) onButtonRelease(ev0 interface{}) {
	if !ta.buttonPressed {
		return
	}
	ta.buttonPressed = false

	ta.ui.CursorMan.UnsetCursor()

	ev := ev0.(*xinput.ButtonReleaseEvent)

	// release must be in the area
	if !ev.Point.In(ta.C.Bounds) {
		return
	}

	switch {
	case ev.Button.Mods.IsButton(1):
		// Commented: on press the cursor is moved and the
		// text position might be ajusted to have the cursor be visible
		// if on release the cursor is moved as well then it can cause an
		// undesired selected area from the visible cursor to the pointer
		//tautil.MoveCursorToPoint(ta, ev.Point, true)

	case ev.Button.Mods.IsButton(2):
		tautil.MoveCursorToPoint(ta, ev.Point, false)
		tautil.PastePrimary(ta)
	case ev.Button.Mods.IsButton(3):
		tautil.MoveCursorToPoint(ta, ev.Point, false)
		ev2 := &TextAreaCmdEvent{ta}
		ta.EvReg.RunCallbacks(TextAreaCmdEventId, ev2)
	}
}

func (ta *TextArea) PointIndexInsideSelection(p *image.Point) bool {
	p2 := p.Sub(ta.Bounds().Min)
	p3 := fixed.P(p2.X, p2.Y)
	p3.Y += ta.OffsetY()
	i := ta.PointIndex(&p3)
	s, e := ta.SelectionIndex(), ta.CursorIndex()
	if s > e {
		s, e = e, s
	}
	return i >= s && i < e
}

func (ta *TextArea) onDoubleClick(ev0 interface{}) {
	ev := ev0.(*xinput.DoubleClickEvent)
	if !ev.Point.In(ta.C.Bounds) {
		return
	}
	switch {
	case ev.Button.Button(1):
		tautil.MoveCursorToPoint(ta, ev.Point, false)
		tautil.SelectWord(ta)
	case ev.Button.Button(3) && ev.Button.Mods.IsNone():
		tautil.MoveCursorToPoint(ta, ev.Point, false)
		ev2 := &TextAreaCmdEvent{ta}
		ta.EvReg.RunCallbacks(TextAreaCmdEventId, ev2)
	}
}
func (ta *TextArea) onTripleClick(ev0 interface{}) {
	ev := ev0.(*xinput.TripleClickEvent)
	if !ev.Point.In(ta.C.Bounds) {
		return
	}
	switch {
	case ev.Button.Button(1):
		tautil.MoveCursorToPoint(ta, ev.Point, false)
		tautil.SelectLine(ta)
	case ev.Button.Button(3) && ev.Button.Mods.IsNone():
		tautil.MoveCursorToPoint(ta, ev.Point, false)
		ev2 := &TextAreaCmdEvent{ta}
		ta.EvReg.RunCallbacks(TextAreaCmdEventId, ev2)
	}
}

func (ta *TextArea) onKeyPress(ev0 interface{}) {
	ev := ev0.(*xinput.KeyPressEvent)
	if !ev.Point.In(ta.C.Bounds) {
		return
	}

	k := ev.Key
	firstKeysym := k.FirstKeysym()
	mods := k.Mods.ClearButtons()

	switch firstKeysym {
	case xinput.XKAltL,
		xinput.XKIsoLevel3Shift,
		xinput.XKShiftL,
		xinput.XKShiftR,
		xinput.XKControlL,
		xinput.XKControlR,
		xinput.XKCapsLock,
		xinput.XKNumLock,
		xinput.XKSuperL,
		xinput.XKInsert:
		// ignore these
	case xinput.XKRight:
		switch {
		case mods.IsControlShift():
			tautil.MoveCursorJumpRight(ta, true)
		case mods.IsControl():
			tautil.MoveCursorJumpRight(ta, false)
		case mods.IsShift():
			tautil.MoveCursorRight(ta, true)
		case mods.IsNone():
			tautil.MoveCursorRight(ta, false)
		}
	case xinput.XKLeft:
		switch {
		case mods.IsControlShift():
			tautil.MoveCursorJumpLeft(ta, true)
		case mods.IsControl():
			tautil.MoveCursorJumpLeft(ta, false)
		case mods.IsShift():
			tautil.MoveCursorLeft(ta, true)
		case mods.IsNone():
			tautil.MoveCursorLeft(ta, false)
		}
	case xinput.XKUp:
		switch {
		case mods.IsControlMod1():
			tautil.MoveLineUp(ta)
		case mods.IsShift():
			tautil.MoveCursorUp(ta, true)
		case mods.IsNone():
			tautil.MoveCursorUp(ta, false)
		}
	case xinput.XKDown:
		switch {
		case mods.IsControlShiftMod1():
			tautil.DuplicateLines(ta)
		case mods.IsControlMod1():
			tautil.MoveLineDown(ta)
		case mods.IsShift():
			tautil.MoveCursorDown(ta, true)
		case mods.IsNone():
			tautil.MoveCursorDown(ta, false)
		}
	case xinput.XKHome:
		switch {
		case mods.IsControlShift():
			tautil.StartOfString(ta, true)
		case mods.IsControl():
			tautil.StartOfString(ta, false)
		case mods.IsShift():
			tautil.StartOfLine(ta, true)
		case mods.IsNone():
			tautil.StartOfLine(ta, false)
		}
	case xinput.XKEnd:
		switch {
		case mods.IsControlShift():
			tautil.EndOfString(ta, true)
		case mods.IsControl():
			tautil.EndOfString(ta, false)
		case mods.IsShift():
			tautil.EndOfLine(ta, true)
		case mods.IsNone():
			tautil.EndOfLine(ta, false)
		}
	case xinput.XKBackspace:
		tautil.Backspace(ta)
	case xinput.XKDelete:
		switch {
		case mods.IsNone():
			tautil.Delete(ta)
		}
	case xinput.XKPageUp:
		switch {
		case mods.IsNone():
			ta.PageUp()
		}
	case xinput.XKPageDown:
		switch {
		case mods.IsNone():
			ta.PageDown()
		}
	case xinput.XKTab:
		switch {
		case mods.IsNone():
			tautil.TabRight(ta)
		case mods.IsShift():
			tautil.TabLeft(ta)
		}
	case xinput.XKReturn:
		switch {
		case mods.IsNone():
			tautil.AutoIndent(ta)
		}
	case xinput.XKSpace:
		tautil.InsertString(ta, " ")
	default:
		// shortcuts with printable runes
		switch {
		case mods.IsControlShift():
			switch firstKeysym {
			case 'd':
				tautil.Uncomment(ta)
			case 'z':
				ta.unpopRedo()
			}
		case mods.IsControl():
			switch firstKeysym {
			case 'd':
				tautil.Comment(ta)
			case 'c':
				tautil.Copy(ta)
			case 'x':
				tautil.Cut(ta)
			case 'v':
				tautil.PasteClipboard(ta)
			case 'k':
				tautil.RemoveLines(ta)
			case 'a':
				tautil.SelectAll(ta)
			case 'z':
				ta.popUndo()
			}
		default: // all other modifier combos
			ta.insertKeyRune(k)
		}
	}
}
func (ta *TextArea) insertKeyRune(k *xinput.Key) {
	// print rune from keysym table (takes into consideration the modifiers)
	ks := k.Keysym()
	switch ks {
	case xinput.XKAsciiTilde:
		tautil.InsertString(ta, "~")
	case xinput.XKAsciiCircum:
		tautil.InsertString(ta, "^")
	case xinput.XKAcute:
		tautil.InsertString(ta, "´")
	case xinput.XKGrave:
		tautil.InsertString(ta, "`")
	default:
		tautil.InsertString(ta, string(rune(ks)))
	}
}
func (ta *TextArea) InsertStringAsync(str string) {
	ta.ui.TextAreaInsertStringAsync(ta, str)
}

const (
	TextAreaCmdEventId = iota
	TextAreaSetStrEventId
	TextAreaSetOffsetYEventId
	TextAreaBoundsChangeEventId
	TextAreaSetCursorIndexEventId
)

type TextAreaCmdEvent struct {
	TextArea *TextArea
}
type TextAreaSetStrEvent struct {
	TextArea  *TextArea
	OldBounds image.Rectangle // TODO: should not be here
}
type TextAreaSetOffsetYEvent struct {
	TextArea *TextArea
}
type TextAreaBoundsChangeEvent struct {
	TextArea *TextArea
}
