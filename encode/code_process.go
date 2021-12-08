package encode

import "io"

type Header struct {
	Seq           uint64
	ServiceMethod string
	Error         string
}

type CodeProcess interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

type NewCodeProcess func(io.ReadWriteCloser) CodeProcess

type Type string

const (
	GobType Type = "application/gob"
)

var NewCodeProcessMap map[Type]NewCodeProcess

func init() {
	NewCodeProcessMap = make(map[Type]NewCodeProcess)
	NewCodeProcessMap[GobType] = NewGobCodeProcess
}
