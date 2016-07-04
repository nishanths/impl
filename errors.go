package main

import "fmt"

type wrappedErr struct {
	message string
	err     error
}

func (w wrappedErr) Error() string {
	return fmt.Sprintf("%s: %v", w.message, w.err)
}

func wrapErr(msg string, e error) error {
	switch e.(type) {
	case wrappedErr:
		msg = msg + ": " + e.(wrappedErr).message
	}
	return wrappedErr{
		message: msg,
		err:     e,
	}
}
