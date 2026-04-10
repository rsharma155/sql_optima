package validation

import "regexp"

var instanceNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateInstanceName validates an instance identifier that is used in URLs and DB lookups.
// It intentionally rejects whitespace, path separators, and HTML-like content.
func ValidateInstanceName(name string) error {
	if name == "" {
		return &Error{Message: "missing instance name"}
	}
	if !instanceNameRe.MatchString(name) {
		return &Error{Message: "invalid instance name: contains invalid characters"}
	}
	if len(name) > 255 {
		return &Error{Message: "instance name too long"}
	}
	return nil
}

type Error struct {
	Message string
}

func (e *Error) Error() string { return e.Message }

