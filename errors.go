package webchan

import "fmt"

type sendError struct {
	Err error
}

type recvError struct {
	Err error
}

func (e *sendError) Error() string {
	return fmt.Sprintf("send error: %s", e.Err)
}

func (e *recvError) Error() string {
	return fmt.Sprintf("recv error: %s", e.Err)
}
