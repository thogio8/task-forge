package apperror

type NotFoundError struct {
	Message string
	Err     error
}

func NotFound(msg string, err error) *NotFoundError {
	return &NotFoundError{Message: msg, Err: err}
}

func (n *NotFoundError) Error() string {
	return n.Message
}

func (n *NotFoundError) Unwrap() error {
	return n.Err
}

type ValidationError struct {
	Message string
	Err     error
}

func Validation(msg string, err error) *ValidationError {
	return &ValidationError{Message: msg, Err: err}
}

func (v *ValidationError) Error() string {
	return v.Message
}

func (v *ValidationError) Unwrap() error {
	return v.Err
}

type InternalError struct {
	Message string
	Err     error
}

func Internal(msg string, err error) *InternalError {
	return &InternalError{Message: msg, Err: err}
}

func (i *InternalError) Error() string {
	return i.Message
}

func (i *InternalError) Unwrap() error {
	return i.Err
}
