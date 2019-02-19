package main

import "fmt"

// ChildNotFound is returned when the requested child object is not found.
type ChildNotFound struct {
	// Name is the name of the requested child object.
	Name string

	// ID is the ID of the requested object.
	ID int
}

// MakeChildNotFound creates a ChildNotFound error.
func MakeChildNotFound(name string, id int) ChildNotFound {
	return ChildNotFound{Name: name, ID: id}
}

// Error implements the Go error interface.
func (c ChildNotFound) Error() string {
	var key interface{}
	if c.Name == "" {
		key = c.ID
	} else {
		key = c.Name
	}
	return fmt.Sprintf("child not found: %v", key)
}
