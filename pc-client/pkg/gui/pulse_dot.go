package gui

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// PulseDot is a custom widget displaying an inner status dot with an animated outer ring
// that pulses like a beating heart whenever an RF heartbeat packet is transmitted.
type PulseDot struct {
	widget.BaseWidget
	innerCircle *canvas.Circle
	outerCircle *canvas.Circle
	active      bool
	isDark      bool
	scale       float32
	anim        *fyne.Animation
}

// heartbeatCurve simulates a biological "lub-dub" heartbeat expansion and ripple.
func heartbeatCurve(t float32) (scale float32, alpha float32) {
	if t < 0.25 {
		// First pump (lub): quick expansion to 0.70
		p := t / 0.25
		scale = 0.70 * p
		alpha = 0.90 * p
	} else if t < 0.40 {
		// Small recoil
		p := (t - 0.25) / 0.15
		scale = 0.70 - 0.20*p
		alpha = 0.90 - 0.30*p
	} else if t < 0.70 {
		// Second pump (dub): expansion to full 1.00
		p := (t - 0.40) / 0.30
		scale = 0.50 + 0.50*p
		alpha = 0.60 + 0.40*(1.0-p*0.5)
	} else {
		// Smooth fade-out to rest
		p := (t - 0.70) / 0.30
		scale = 1.00
		alpha = 0.80 * (1.0 - p)
	}
	return
}

func (p *PulseDot) primaryColor() color.NRGBA {
	if p.isDark {
		return ColorCyan
	}
	return color.NRGBA{R: 0x00, G: 0x89, B: 0x7B, A: 0xFF} // High contrast teal for light mode
}

func (p *PulseDot) offlineColor() color.NRGBA {
	if p.isDark {
		return ColorPurple
	}
	return color.NRGBA{R: 0x7C, G: 0x3A, B: 0xED, A: 0xFF}
}

// NewPulseDot creates a new PulseDot widget with an initial connected state and theme.
func NewPulseDot(initConnected bool, isDark bool) *PulseDot {
	p := &PulseDot{
		active: initConnected,
		isDark: isDark,
		scale:  0.0,
	}

	dotColor := p.offlineColor()
	if initConnected {
		dotColor = p.primaryColor()
	}

	p.innerCircle = canvas.NewCircle(dotColor)
	p.outerCircle = canvas.NewCircle(color.Transparent)
	p.outerCircle.StrokeColor = color.Transparent
	p.outerCircle.StrokeWidth = 1.5
	p.ExtendBaseWidget(p)

	p.anim = fyne.NewAnimation(480*time.Millisecond, func(progress float32) {
		s, a := heartbeatCurve(progress)
		p.scale = s
		alpha := uint8(a * 235)
		c := p.primaryColor()
		p.outerCircle.StrokeColor = color.NRGBA{R: c.R, G: c.G, B: c.B, A: alpha}
		p.Refresh()
	})

	return p
}

// Pulse triggers a single heartbeat cycle on the outer ring.
func (p *PulseDot) Pulse() {
	if p.active && p.anim != nil {
		p.anim.Stop()
		p.anim.Start()
	}
}

// SetConnected sets whether the status dot is active (online) or offline.
func (p *PulseDot) SetConnected(connected bool) {
	if p.active == connected {
		return
	}
	p.active = connected
	if connected {
		p.innerCircle.FillColor = p.primaryColor()
	} else {
		p.innerCircle.FillColor = p.offlineColor()
		if p.anim != nil {
			p.anim.Stop()
		}
		p.outerCircle.StrokeColor = color.Transparent
	}
	p.Refresh()
}

// SetTheme adapts dot and ring colors to Dark or Light mode.
func (p *PulseDot) SetTheme(isDark bool) {
	p.isDark = isDark
	if p.active {
		p.innerCircle.FillColor = p.primaryColor()
	} else {
		p.innerCircle.FillColor = p.offlineColor()
	}
	p.Refresh()
}

func (p *PulseDot) CreateRenderer() fyne.WidgetRenderer {
	return &pulseDotRenderer{
		dot:     p,
		objects: []fyne.CanvasObject{p.outerCircle, p.innerCircle},
	}
}

type pulseDotRenderer struct {
	dot     *PulseDot
	objects []fyne.CanvasObject
}

func (r *pulseDotRenderer) Layout(size fyne.Size) {
	centerX := size.Width / 2
	centerY := size.Height / 2

	// Inner dot: ~7px diameter
	innerRadius := float32(3.5)
	r.dot.innerCircle.Move(fyne.NewPos(centerX-innerRadius, centerY-innerRadius))
	r.dot.innerCircle.Resize(fyne.NewSize(innerRadius*2, innerRadius*2))

	// Outer ring: expands from 3.5px up to 8.0px
	maxRadius := float32(8.0)
	curRadius := innerRadius + (maxRadius-innerRadius)*r.dot.scale
	r.dot.outerCircle.Move(fyne.NewPos(centerX-curRadius, centerY-curRadius))
	r.dot.outerCircle.Resize(fyne.NewSize(curRadius*2, curRadius*2))
}

func (r *pulseDotRenderer) MinSize() fyne.Size {
	return fyne.NewSize(18, 18)
}

func (r *pulseDotRenderer) Refresh() {
	r.Layout(r.MinSize())
	canvas.Refresh(r.dot.outerCircle)
	canvas.Refresh(r.dot.innerCircle)
}

func (r *pulseDotRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *pulseDotRenderer) Destroy() {}
