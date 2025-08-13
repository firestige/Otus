package log

import "io"

type MultiWriter struct {
	writers []io.Writer
}

func (m *MultiWriter) Write(p []byte) (n int, err error) {
	for _, w := range m.writers {
		_, e := w.Write(p)
		if e != nil {
			err = e
		}
	}
	return len(p), err
}

func (m *MultiWriter) Add(writer io.Writer) *MultiWriter {
	m.writers = append(m.writers, writer)
	return m
}

func NewMultiWriter() *MultiWriter {
	return &MultiWriter{writers: make([]io.Writer, 0)}
}
