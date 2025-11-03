package mock_test

import (
	"context"
	"fmt"
	"io"

	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/config"
	"firestige.xyz/otus/internal/otus/factory"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

const Name = "file"

type FileCfg struct {
	config.SourceConfig `mapstructure:",squash"`
	FilePath            string `mapstructure:"file_path"`
}

type FileSource struct {
	path   string
	handle *pcap.Handle
}

func Init() {
	fn := func(cfg interface{}) interface{} {
		fileCfg, ok := cfg.(*FileCfg)
		if !ok {
			return nil
		}
		s, err := NewSource(fileCfg)
		if err != nil {
			return nil
		}
		return s
	}
	factory.Register(otus.ComponentTypeSource, Name, fn)
}

func NewSource(cfg *FileCfg) (s otus.Source, err error) {
	if cfg.FilePath == "" {
		return nil, fmt.Errorf("file_path is required")
	}
	return &FileSource{
		path: cfg.FilePath,
	}, nil
}

func (fs *FileSource) Start(ctx context.Context) error {
	// Open the pcap file
	handle, err := pcap.OpenOffline(fs.path)
	if err != nil {
		return fmt.Errorf("failed to open pcap file %s: %w", fs.path, err)
	}
	fs.handle = handle
	return nil
}

func (fs *FileSource) ReadPacket() ([]byte, gopacket.CaptureInfo, error) {
	if fs.handle == nil {
		return nil, gopacket.CaptureInfo{}, fmt.Errorf("file source not started")
	}

	// Read the next packet from the file
	data, ci, err := fs.handle.ReadPacketData()
	if err != nil {
		// Return EOF or other errors
		if err == io.EOF {
			return nil, gopacket.CaptureInfo{}, io.EOF
		}
		return nil, gopacket.CaptureInfo{}, fmt.Errorf("failed to read packet: %w", err)
	}

	return data, ci, nil
}

func (fs *FileSource) LinkType() layers.LinkType {
	if fs.handle == nil {
		return layers.LinkTypeEthernet // default
	}
	return fs.handle.LinkType()
}

func (fs *FileSource) Stop() error {
	if fs.handle != nil {
		fs.handle.Close()
		fs.handle = nil
	}
	return nil
}
