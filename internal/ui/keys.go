package ui

import "github.com/charmbracelet/bubbletea"

type keyMsg struct {
	name string
}

// Key message types
var (
	KeyTab      = keyMsg{"tab"}
	KeyUp       = keyMsg{"up"}
	KeyDown     = keyMsg{"down"}
	KeyEnter    = keyMsg{"enter"}
	KeySpace    = keyMsg{"space"}
	KeyCopy     = keyMsg{"c"}
	KeyAddHost  = keyMsg{"n"}
	KeyQuit     = keyMsg{"q"}
	KeyBackspace = keyMsg{"backspace"}
)

func (k keyMsg) String() string {
	return k.name
}

func handleKey(msg tea.KeyMsg) tea.Msg {
	switch msg.String() {
	case "tab":
		return KeyTab
	case "up":
		return KeyUp
	case "down":
		return KeyDown
	case "enter":
		return KeyEnter
	case " ":
		return KeySpace
	case "c":
		return KeyCopy
	case "n":
		return KeyAddHost
	case "q":
		return KeyQuit
	case "backspace":
		return KeyBackspace
	}
	return nil
}
