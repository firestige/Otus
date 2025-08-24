package handle

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

const (
	TypeMock CaptureType = "mock_handle"
)

type MockHandle struct {
	isOpen      bool
	packetIndex uint64
	options     *Options

	// 预构建的SIP报文数据
	sipPackets [][]byte
}

// 构造完整的以太网+IP+UDP+SIP报文
func (m *MockHandle) buildSIPPackets() error {
	// SIP INVITE 请求报文
	sipInvite := "INVITE sip:alice@192.168.1.100 SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP 192.168.1.50:5060;branch=z9hG4bK-123456\r\n" +
		"From: Bob <sip:bob@192.168.1.50>;tag=12345\r\n" +
		"To: Alice <sip:alice@192.168.1.100>\r\n" +
		"Call-ID: 123456789@192.168.1.50\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"Contact: <sip:bob@192.168.1.50:5060>\r\n" +
		"Content-Type: application/sdp\r\n" +
		"Content-Length: 142\r\n" +
		"\r\n" +
		"v=0\r\n" +
		"o=bob 123456 654321 IN IP4 192.168.1.50\r\n" +
		"s=Session\r\n" +
		"c=IN IP4 192.168.1.50\r\n" +
		"t=0 0\r\n" +
		"m=audio 8000 RTP/AVP 0\r\n" +
		"a=rtpmap:0 PCMU/8000\r\n"

	// SIP 200 OK 响应报文
	sipOK := "SIP/2.0 200 OK\r\n" +
		"Via: SIP/2.0/UDP 192.168.1.50:5060;branch=z9hG4bK-123456\r\n" +
		"From: Bob <sip:bob@192.168.1.50>;tag=12345\r\n" +
		"To: Alice <sip:alice@192.168.1.100>;tag=67890\r\n" +
		"Call-ID: 123456789@192.168.1.50\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"Contact: <sip:alice@192.168.1.100:5060>\r\n" +
		"Content-Type: application/sdp\r\n" +
		"Content-Length: 147\r\n" +
		"\r\n" +
		"v=0\r\n" +
		"o=alice 654321 987654 IN IP4 192.168.1.100\r\n" +
		"s=Session\r\n" +
		"c=IN IP4 192.168.1.100\r\n" +
		"t=0 0\r\n" +
		"m=audio 8001 RTP/AVP 0\r\n" +
		"a=rtpmap:0 PCMU/8000\r\n"

	// SIP ACK 请求报文
	sipACK := "ACK sip:alice@192.168.1.100 SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP 192.168.1.50:5060;branch=z9hG4bK-789012\r\n" +
		"From: Bob <sip:bob@192.168.1.50>;tag=12345\r\n" +
		"To: Alice <sip:alice@192.168.1.100>;tag=67890\r\n" +
		"Call-ID: 123456789@192.168.1.50\r\n" +
		"CSeq: 1 ACK\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n"

	// SIP BYE 请求报文
	sipBYE := "BYE sip:alice@192.168.1.100 SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP 192.168.1.50:5060;branch=z9hG4bK-345678\r\n" +
		"From: Bob <sip:bob@192.168.1.50>;tag=12345\r\n" +
		"To: Alice <sip:alice@192.168.1.100>;tag=67890\r\n" +
		"Call-ID: 123456789@192.168.1.50\r\n" +
		"CSeq: 2 BYE\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n"

	sipMessages := []string{sipInvite, sipOK, sipACK, sipBYE}

	m.sipPackets = make([][]byte, len(sipMessages))

	for i, sipMsg := range sipMessages {
		var srcIP, dstIP net.IP
		var srcPort, dstPort uint16

		// 根据消息类型设置源和目的地址
		if i == 1 { // 200 OK 响应，从Alice到Bob
			srcIP = net.ParseIP("192.168.1.100")
			dstIP = net.ParseIP("192.168.1.50")
			srcPort = 5060
			dstPort = 5060
		} else { // 请求消息，从Bob到Alice
			srcIP = net.ParseIP("192.168.1.50")
			dstIP = net.ParseIP("192.168.1.100")
			srcPort = 5060
			dstPort = 5060
		}

		packet, err := m.buildEthernetPacket(srcIP, dstIP, srcPort, dstPort, []byte(sipMsg))
		if err != nil {
			return fmt.Errorf("failed to build packet %d: %v", i, err)
		}
		m.sipPackets[i] = packet
	}

	return nil
}

// 构建完整的以太网帧 (Ethernet + IP + UDP + SIP)
func (m *MockHandle) buildEthernetPacket(srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte) ([]byte, error) {
	// 构建以太网层
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		DstMAC:       net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee},
		EthernetType: layers.EthernetTypeIPv4,
	}

	// 构建IP层
	ip := &layers.IPv4{
		Version:    4,
		IHL:        5,
		TOS:        0,
		Length:     0, // 会被自动计算
		Id:         uint16(atomic.AddUint64(&m.packetIndex, 1)),
		Flags:      layers.IPv4DontFragment,
		FragOffset: 0,
		TTL:        64,
		Protocol:   layers.IPProtocolUDP,
		SrcIP:      srcIP,
		DstIP:      dstIP,
	}

	// 构建UDP层
	udp := &layers.UDP{
		SrcPort: layers.UDPPort(srcPort),
		DstPort: layers.UDPPort(dstPort),
	}

	// 设置UDP校验和计算所需的网络层
	err := udp.SetNetworkLayerForChecksum(ip)
	if err != nil {
		return nil, fmt.Errorf("failed to set network layer for checksum: %v", err)
	}

	// 序列化所有层
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	err = gopacket.SerializeLayers(buf, opts, eth, ip, udp, gopacket.Payload(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to serialize packet: %v", err)
	}

	return buf.Bytes(), nil
}

func (m *MockHandle) Open(options *Options) error {
	if m.isOpen {
		return fmt.Errorf("mock handle already opened")
	}

	m.options = options
	m.packetIndex = 0

	// 构建SIP报文数据
	if err := m.buildSIPPackets(); err != nil {
		return fmt.Errorf("failed to build SIP packets: %v", err)
	}

	m.isOpen = true
	return nil
}

func (m *MockHandle) ReadPacket() ([]byte, gopacket.CaptureInfo, error) {
	if !m.isOpen {
		return nil, gopacket.CaptureInfo{}, fmt.Errorf("mock handle not opened")
	}

	// 循环返回预构建的SIP报文
	packetIdx := atomic.LoadUint64(&m.packetIndex) % uint64(len(m.sipPackets))
	atomic.AddUint64(&m.packetIndex, 1)

	packet := m.sipPackets[packetIdx]

	// 构建CaptureInfo
	captureInfo := gopacket.CaptureInfo{
		Timestamp:      time.Now(),
		CaptureLength:  len(packet),
		Length:         len(packet),
		InterfaceIndex: 0,
	}

	// 模拟网络延迟
	time.Sleep(10 * time.Millisecond)

	return packet, captureInfo, nil
}

func (m *MockHandle) Close() error {
	if !m.isOpen {
		return fmt.Errorf("mock handle not opened")
	}

	m.isOpen = false
	m.sipPackets = nil
	m.options = nil
	return nil
}

func (m *MockHandle) GetType() CaptureType {
	return TypeMock
}

// IsOpen 检查句柄是否已打开（用于测试）
func (m *MockHandle) IsOpen() bool {
	return m.isOpen
}

// GetPacketCount 获取当前数据包索引（用于测试）
func (m *MockHandle) GetPacketCount() uint64 {
	return atomic.LoadUint64(&m.packetIndex)
}
