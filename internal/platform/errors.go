package platform

// Error is a stable application error suitable for mapping to the API envelope.
type Error struct {
	Code       string
	HTTPStatus int
	Fields     map[string]string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Code
}
