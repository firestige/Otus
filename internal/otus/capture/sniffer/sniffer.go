package sniffer

import (
	"context"
	"fmt"
	"sync"

	"firestige.xyz/otus/internal/otus/capture/codec"
)

type Sniffer struct {
	factory *CaptureHandleFactory
	options *Options
	handle  CaptureHandle
	decoder *codec.Decoder

	wg *sync.WaitGroup
}

func NewSniffer(decoder *codec.Decoder, options *Options, ctx context.Context) *Sniffer {
	return &Sniffer{
		decoder: decoder,
		options: options,
	}
}

func (s *Sniffer) Start(ctx context.Context) error {
	if !s.factory.IsTypeSupported(s.options.CaptureType) {
		return fmt.Errorf("capture type %s is not supported", s.options.CaptureType)
	}

	handle, err := s.factory.CreateHandle(s.options.CaptureType)
	if err != nil {
		return fmt.Errorf("failed to create capture handle: %v", err)
	}

	s.handle = handle
	err = s.handle.Open(s.options.InterfaceName, s.options.CaptureOptions)
	if err != nil {
		return fmt.Errorf("failed to open capture handle: %v", err)
	}

	go func() {
		defer func() {
			s.handle.Close()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				rawData, ci, err := s.handle.ReadPacket()
				if err != nil {
					return
				}

				err = s.decoder.Decode(rawData, &ci)
				if err != nil {
					continue // 处理解码错误
				}
			}
		}
	}()
	return nil
}

func (s *Sniffer) Stop() {
	s.wg.Wait()
	s.decoder.Close()
}
