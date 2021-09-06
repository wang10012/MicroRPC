package encode

import "io"

type NewCodeProcess func(io.ReadWriteCloser) CodeProcess

type Type string

const (
	GobType Type = "application/gob"
)

var NewCodeProcessMap map[Type]NewCodeProcess

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

func init() {
	NewCodeProcessMap = make(map[Type]NewCodeProcess)
	NewCodeProcessMap[GobType] = NewGodCodeProcess
}
