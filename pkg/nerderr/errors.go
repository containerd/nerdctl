package nerderr

import "errors"

var (
	// ErrSystemIsBroken should wrap all system-level errors (filesystem unexpected conditions, hosed files, misbehaving subsystems)
	ErrSystemIsBroken = errors.New("system error")

	// ErrInvalidArgument should wrap all cases where an argument does not match expected syntax, or prevents an operation from succeeding
	// because of its value
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrServerIsMisbehaving should wrap all server errors (eg: status code 50x)
	// but NOT dns, tcp, or tls errors
	ErrServerIsMisbehaving = errors.New("server error")
)
