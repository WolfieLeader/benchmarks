package consts

const (
	ErrInvalidJSON                  = "invalid JSON body"
	ErrInvalidForm                  = "invalid form data"
	ErrExpectedFormContentType      = "expected content-type: application/x-www-form-urlencoded or multipart/form-data"
	ErrInvalidMultipart             = "invalid multipart form data"
	ErrExpectedMultipartContentType = "expected content-type: multipart/form-data"
	ErrFileNotFound                 = "file not found in form data"
	ErrFileSizeExceeded             = "file size exceeds limit"
	ErrInvalidFileType              = "only text/plain files are allowed"
	ErrNotPlainText                 = "file does not look like plain text"
	ErrNotFound                     = "not found"
	ErrInternal                     = "internal error"
)
