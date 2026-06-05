package handlers

import (
	"io"
	"net/http"
)

// flushWriter implements http.ResponseWriter + http.Flusher for tests.
type flushWriter struct {
	pw     io.Writer
	header http.Header
	code   int
}

func (f *flushWriter) Header() http.Header        { return f.header }
func (f *flushWriter) WriteHeader(code int)        { f.code = code }
func (f *flushWriter) Write(b []byte) (int, error) { return f.pw.Write(b) }
func (f *flushWriter) Flush()                      {}

func chanPipe() (io.Reader, io.Writer) {
	pr, pw := io.Pipe()
	return pr, pw
}
