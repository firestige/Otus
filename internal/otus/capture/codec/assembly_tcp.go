package codec

import (
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/tcpassembly"
)

type streamFactory struct {
}

type tcpStream struct {
	net, transport gopacket.Flow
	readerStream   readerStream
}

func (s *streamFactory) NewStream(net, transport gopacket.Flow) tcpassembly.Stream {
	stream := &tcpStream{
		net:       net,
		transport: transport,
		readerStream: readerStream{
			ReassembledChan: make(chan tcpassembly.Reassembly),
		},
	}
	go stream.run()
	return stream.readerStream
}

type readerStream struct {
	tcpassembly.Stream
	ReassembledChan chan tcpassembly.Reassembly
	closed          bool
}

func newReaderStream() readerStream {
	return readerStream{
		ReassembledChan: make(chan tcpassembly.Reassembly),
		closed:          false,
	}
}

func (r *readerStream) Reassembled(readerStream []tcpassembly.Reassembly) {
	for _, reassembly := range readerStream {
		r.ReassembledChan <- reassembly
	}
}

func (r *readerStream) ReassemblyComplete() {
	close(r.ReassembledChan)
	r.closed = true
}

func (s *tcpStream) run() {
	tsUnset := time.Unix(0, 0)
	ts = tsUnset
	data := make([]byte, 0)
	var reassembly tcpassembly.Reassembly
	again := false
	ignore := false
	for {
		if again {
			again = false
		} else {
			reassembly, more := <-s.readerStream.ReassembledChan
			if !more {
				return
			}
			if reassembly.Skip != 0 {
				data = data[0:0]
				ts = tsUnset
			}
			if ignore {
				continue
			}
			if len(reassembly.Bytes) == 0 {
				continue
			}
			if ts == tsUnset {
				ts = reassembly.Seen
			}
			data = append(data, reassembly.Bytes...)
		}
		if len(data) == 0 {
			continue
		}
		// 判断是不是SIP消息
	}
}
