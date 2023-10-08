package gatesentryproxy

type ContentScannerInput struct {
	Content     []byte
	ContentType string
	Url         string
}
