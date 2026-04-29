package mail

import "errors"

var (
	ErrInvalidRecipient = errors.New("invalid recipient")
	ErrInactiveDomain   = errors.New("inactive or unknown domain")
)
