package app

// Palette indices for the PICO-8 editor-inspired interface. Keep UI colors
// here so the chrome, status bar, and selection styles stay in sync.
const (
	// Chrome is the red top and bottom bar family from PICO-8's editor.
	chromeBackground       = 8  // red
	chromeForeground       = 15 // peach
	chromeActiveBackground = 14 // pink
	chromeActiveForeground = 2  // dark purple

	// Panels are the dark navy popups used for help, dialogs, and the palette.
	panelBorder = 6  // light gray
	panelTitle  = 15 // peach

	// Editor highlights.
	currentLineBackground   = 0 // black; PICO-8 does not brighten the cursor line
	textSelectionBackground = 2 // dark purple
	searchBackground        = 3 // dark green
	searchCurrentBackground = 9 // orange
	searchCurrentForeground = 0 // black
	selectionBackground     = 14
	selectionForeground     = 2

	// Status bar.
	statusBackground        = 8  // red
	statusForeground        = 15 // peach
	statusSecondary         = 7  // white
	statusErrorForeground   = 0  // black
	statusWarningForeground = 10 // yellow
)
