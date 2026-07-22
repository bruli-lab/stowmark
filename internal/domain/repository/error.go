package repository

import "fmt"

type NotFoundError struct {
	msg string
}

func (n NotFoundError) Error() string {
	return fmt.Sprintf("not found: %s", n.msg)
}

func NewNotFoundError(msg string) *NotFoundError {
	return &NotFoundError{msg: msg}
}
