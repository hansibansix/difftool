package tui

// prompt is a question waiting in the status line: the next key answers it
// and the prompt is gone either way. answer gets the key and the action it
// is bound to and reports whether it accepted; otherwise cancel is shown.
type prompt struct {
	answer func(key, act string) bool
	cancel string
}

// yesNo accepts y / Y.
func yesNo(yes func()) func(key, act string) bool {
	return func(key, _ string) bool {
		if key == "y" || key == "Y" {
			yes()
			return true
		}
		return false
	}
}

// direction accepts the ▶ / ◀ keys of the given actions.
func direction(right, left string, do func(toRight bool)) func(key, act string) bool {
	return func(_, act string) bool {
		switch act {
		case right:
			do(true)
		case left:
			do(false)
		default:
			return false
		}
		return true
	}
}
