package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Aesthetic Color Palette (Cyberpunk / Modern Terminal Slate)
var (
	ColorPrimary   = lipgloss.Color("#00f5d4") // Neon Cyan
	ColorSecondary = lipgloss.Color("#f72585") // Neon Magenta / Pink
	ColorAccent    = lipgloss.Color("#7209b7") // Deep Purple
	ColorYellow    = lipgloss.Color("#fee440") // Electric Yellow
	ColorGreen     = lipgloss.Color("#06d6a0") // Emerald Mint
	ColorRed       = lipgloss.Color("#ef476f") // Coral Red
	ColorSlateBg   = lipgloss.Color("#0d1117") // Dark Github / Cyber Slate
	ColorCardBg    = lipgloss.Color("#161b22") // Card Surface
	ColorCardAlt   = lipgloss.Color("#21262d") // Card Alternate
	ColorBorder    = lipgloss.Color("#30363d") // Subtle Border
	ColorBorderLit = lipgloss.Color("#58a6ff") // Active Border
	ColorText      = lipgloss.Color("#f0f6fc") // Off-white
	ColorTextDim   = lipgloss.Color("#8b949e") // Muted gray
)

// UI Component Styles
var (
	// Root Container
	AppStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Background(ColorSlateBg)

	// Top Banner / Header
	HeaderTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Background(ColorCardBg).
			Padding(0, 1)

	HeaderBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSlateBg).
			Background(ColorSecondary).
			Padding(0, 1)

	HeaderRate = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSlateBg).
			Background(ColorGreen).
			Padding(0, 1)

	HeaderProfile = lipgloss.NewStyle().
			Foreground(ColorYellow).
			Background(ColorCardAlt).
			Padding(0, 1)

	// Status Badges
	StatusConnected = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSlateBg).
			Background(ColorGreen).
			Padding(0, 1)

	StatusDisconnected = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSlateBg).
			Background(ColorRed).
			Padding(0, 1)

	// Status Pills
	PillPassThrough = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSlateBg).
			Background(ColorYellow).
			Padding(0, 1)

	PillGrabbed = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSlateBg).
			Background(ColorGreen).
			Padding(0, 1)

	// Navigation Tabs
	TabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSlateBg).
			Background(ColorPrimary).
			Padding(0, 2).
			MarginRight(1)

	TabInactive = lipgloss.NewStyle().
			Foreground(ColorTextDim).
			Background(ColorCardAlt).
			Padding(0, 2).
			MarginRight(1)

	// Content Cards
	CardBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Background(ColorCardBg).
		Padding(1, 2)

	CardBoxActive = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorPrimary).
			Background(ColorCardBg).
			Padding(1, 2)

	CardBoxSlim = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Background(ColorCardBg).
			Padding(0, 1)

	CardBoxSlimActive = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorGreen).
			Background(ColorCardBg).
			Padding(0, 1)

	CardBoxWarning = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSecondary).
			Background(ColorCardBg).
			Padding(1, 2)

	// Mouse Button HUD Badges
	BtnActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSlateBg).
			Background(ColorYellow).
			Padding(0, 1)

	BtnInactive = lipgloss.NewStyle().
			Foreground(ColorTextDim).
			Background(ColorCardAlt).
			Padding(0, 1)

	// Live Capture HUD
	HudBox = lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(ColorPrimary).
		Background(ColorCardAlt).
		Padding(1, 3).
		Align(lipgloss.Center)

	KeyPill = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSlateBg).
		Background(ColorYellow).
		Padding(0, 2).
		Margin(0, 1)

	KeyPillDim = lipgloss.NewStyle().
			Foreground(ColorTextDim).
			Background(ColorCardBg).
			Padding(0, 1).
			Margin(0, 1)

	// Macro Buttons
	MacroBtnActive = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Foreground(ColorText).
			Background(ColorCardAlt).
			Padding(0, 1).
			Width(22)

	MacroBtnNormal = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(ColorBorder).
			Foreground(ColorTextDim).
			Background(ColorCardBg).
			Padding(0, 1).
			Width(22)

	MacroKeyTag = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	MacroTitle = lipgloss.NewStyle().
			Foreground(ColorText)

	// Telemetry Labels
	StatLabel = lipgloss.NewStyle().
			Foreground(ColorTextDim).
			Width(15)

	StatValue = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	StatValueDim = lipgloss.NewStyle().
			Foreground(ColorTextDim)

	// Help / Footer Bar
	FooterKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	FooterDesc = lipgloss.NewStyle().
			Foreground(ColorTextDim).
			MarginRight(2)
)
