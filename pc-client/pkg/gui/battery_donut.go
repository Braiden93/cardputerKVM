package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// BatteryDonut is a circular radial gauge widget for the Cardputer receiver battery.
// Uses a vector-drawn circle ring with centered percentage text, fully visible in both light & dark themes.
type BatteryDonut struct {
	widget.BaseWidget
	trackCircle *canvas.Circle
	text        *canvas.Text
	size        int
	textVal     string
	percent     float64
	isOnline    bool
	isDark      bool
}

func (b *BatteryDonut) ringColor() color.NRGBA {
	if b.isDark {
		if b.isOnline {
			return ColorCyan // Neon Cyan in dark mode
		}
		return ColorPurple // Purple in dark mode
	}
	// Light mode: high-contrast colors on white card
	if b.isOnline {
		return color.NRGBA{R: 0x00, G: 0x89, B: 0x7B, A: 0xFF} // Vibrant Teal
	}
	return color.NRGBA{R: 0x64, G: 0x74, B: 0x8B, A: 0xFF} // Slate-500
}

func (b *BatteryDonut) textColor() color.NRGBA {
	if b.isDark {
		return color.NRGBA{R: 0xF8, G: 0xFA, B: 0xFC, A: 0xFF} // Crisp white
	}
	return color.NRGBA{R: 0x0F, G: 0x17, B: 0x2A, A: 0xFF} // Deep slate/black
}

// NewBatteryDonut creates a new BatteryDonut gauge of given pixel diameter.
func NewBatteryDonut(size int, textVal string, isOnline bool, isDark bool) *BatteryDonut {
	b := &BatteryDonut{
		size:     size,
		textVal:  textVal,
		isOnline: isOnline,
		isDark:   isDark,
	}

	b.trackCircle = canvas.NewCircle(color.Transparent)
	b.trackCircle.StrokeColor = b.ringColor()
	b.trackCircle.StrokeWidth = 3.5

	fontSize := float32(size) * 0.36
	if fontSize < 11 {
		fontSize = 11
	}

	b.text = canvas.NewText(textVal, b.textColor())
	b.text.TextSize = fontSize
	b.text.TextStyle = fyne.TextStyle{Bold: true}

	b.ExtendBaseWidget(b)
	return b
}

// SetValue updates the displayed text and progress ratio (0.0 to 1.0).
func (b *BatteryDonut) SetValue(textVal string, percent float64) {
	b.textVal = textVal
	b.percent = percent
	b.text.Text = textVal
	b.Refresh()
}

// SetOnline updates the donut color state based on connection status.
func (b *BatteryDonut) SetOnline(online bool) {
	b.isOnline = online
	b.trackCircle.StrokeColor = b.ringColor()
	b.text.Color = b.textColor()
	b.Refresh()
}

// SetTheme updates the theme mode (Dark vs Light).
func (b *BatteryDonut) SetTheme(isDark bool) {
	b.isDark = isDark
	b.trackCircle.StrokeColor = b.ringColor()
	b.text.Color = b.textColor()
	b.Refresh()
}

func (b *BatteryDonut) CreateRenderer() fyne.WidgetRenderer {
	return &donutRenderer{
		donut:   b,
		objects: []fyne.CanvasObject{b.trackCircle, b.text},
	}
}

type donutRenderer struct {
	donut   *BatteryDonut
	objects []fyne.CanvasObject
}

func (r *donutRenderer) Layout(size fyne.Size) {
	centerX := size.Width / 2
	centerY := size.Height / 2
	radius := float32(r.donut.size) / 2.0

	r.donut.trackCircle.Move(fyne.NewPos(centerX-radius, centerY-radius))
	r.donut.trackCircle.Resize(fyne.NewSize(radius*2, radius*2))

	txtMin := r.donut.text.MinSize()
	r.donut.text.Move(fyne.NewPos(centerX-txtMin.Width/2, centerY-txtMin.Height/2))
	r.donut.text.Resize(txtMin)
}

func (r *donutRenderer) MinSize() fyne.Size {
	return fyne.NewSize(float32(r.donut.size), float32(r.donut.size))
}

func (r *donutRenderer) Refresh() {
	r.donut.trackCircle.StrokeColor = r.donut.ringColor()
	r.donut.text.Color = r.donut.textColor()
	canvas.Refresh(r.donut.trackCircle)
	canvas.Refresh(r.donut.text)
}

func (r *donutRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *donutRenderer) Destroy() {}
