package foundation

import "github.com/google/uuid"

type ID string

func NewID() ID {
	return ID(uuid.NewString())
}

func (id ID) String() string {
	return string(id)
}
