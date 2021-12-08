package encode

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

type GobCodeProcess struct {
	connect io.ReadWriteCloser
	encoder *gob.Encoder
	decoder *gob.Decoder
	buffer  *bufio.Writer
}

func (c *GobCodeProcess) ReadHeader(h *Header) error {
	return c.decoder.Decode(h)
}

func (c *GobCodeProcess) ReadBody(body interface{}) error {
	return c.decoder.Decode(body)
}

func (c *GobCodeProcess) Close() error {
	return c.connect.Close()
}

func (c *GobCodeProcess) Write(header *Header, body interface{}) (err error) {
	defer func() {
		_ = c.buffer.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()
	if err := c.encoder.Encode(header); err != nil {
		log.Println("rpc encoding: gob error encoding header:", err)
		return err
	}
	if err := c.encoder.Encode(body); err != nil {
		log.Println("rpc encoding: gob error encoding body:", err)
		return err
	}
	return nil
}

var _ CodeProcess = (*GobCodeProcess)(nil)

func NewGobCodeProcess(connect io.ReadWriteCloser) CodeProcess {
	buffer := bufio.NewWriter(connect)
	return &GobCodeProcess{
		connect: connect,
		encoder: gob.NewEncoder(buffer),
		decoder: gob.NewDecoder(connect),
		buffer:  buffer,
	}
}
