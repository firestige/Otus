package sniffer

import (
	"context"
	"fmt"

	"firestige.xyz/otus/internal/otus/module/capture/codec"
)

type Sniffer struct {
	options *Options

	handle  CaptureHandle
	decoder *codec.Decoder
}

func (s *Sniffer) Start(ctx context.Context) error {
	factory := HandleFactory()
	if !factory.IsTypeSupported(s.options.CaptureType) {
		return fmt.Errorf("capture type %s is not supported", s.options.CaptureType)
	}

	handle, err := factory.CreateHandle(ctx, s.options.CaptureType)
	if err != nil {
		return fmt.Errorf("failed to create capture handle: %v", err)
	}

	s.handle = handle
	err = s.handle.Open(s.options)
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
}
