// Package popup shows small windows that stay above the game without
// taking focus: the quick-sort prompt and notifications.
package popup

// Quick is the quick-sort prompt.
type Quick struct {
	Title   string    // replay summary
	Target  string    // folder chosen by the rule
	Options [4]string // labels for Ctrl+Alt+1–4, "" if unused
	Seconds int
}

// Toast is a notification with up to two actions.
type Toast struct {
	Title   string
	Body    string
	Actions []Action
	Disable func() // "don't show again", nil hides the link
}

type Action struct {
	Label string
	Run   func()
}
