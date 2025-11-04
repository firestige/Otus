package decoder

import (
	"sync/atomic"
	"time"

	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/config"
	"firestige.xyz/otus/internal/otus/factory"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

const Name = "default_decoder"

type Decoder struct {
	parser *gopacket.DecodingLayerParser

	eth     layers.Ethernet
	ip4     layers.IPv4
	ip6     layers.IPv6
	ip6Frag layers.IPv6Fragment
	tcp     layers.TCP
	udp     layers.UDP
	payload gopacket.Payload

	decoded []gopacket.LayerType

	reassembler     *ipv4Reassembler // IPv4 分片重组器
	ipv6Reassembler *ipv6Reassembler // IPv6 分片重组器

	statistics
}

type statistics struct {
	ipv4Count uint64
	ipv6Count uint64
	tcpCount  uint64
	udpCount  uint64
}

type DecoderCfg struct {
	config.DecoderConfig
	FragmentTimeout time.Duration `mapstructure:"fragment_timeout"` // 分片超时时间，默认 30 秒
}

func NewDecoder(cfg *DecoderCfg) (*Decoder, error) {
	// 设置默认分片超时时间
	timeout := cfg.FragmentTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	d := &Decoder{
		reassembler:     newIPv4Reassembler(timeout),
		ipv6Reassembler: newIPv6Reassembler(timeout),
	}
	d.parser = gopacket.NewDecodingLayerParser(
		layers.LayerTypeEthernet,
		&d.eth,
		&d.ip4,
		&d.ip6,
		&d.tcp,
		&d.udp,
		&d.payload,
	)
	d.parser.IgnoreUnsupported = true
	return d, nil
}

func init() {
	// 注册 Decoder 组件到工厂
	fn := func(cfg interface{}) interface{} {
		decoderCfg, ok := cfg.(*DecoderCfg)
		if !ok {
			return nil
		}
		decoder, err := NewDecoder(decoderCfg)
		if err != nil {
			return nil
		}
		return decoder
	}

	factory.Register(otus.ComponentTypeDecoder, Name, fn)
}

func (d *Decoder) Decode(data []byte, ci gopacket.CaptureInfo) (*otus.NetPacket, error) {
	d.decoded = d.decoded[:0] // 重置切片以重用

	err := d.parser.DecodeLayers(data, &d.decoded)
	if err != nil {
		return nil, err
	}

	packet := &otus.NetPacket{
		Timestamp: ci.Timestamp.UnixMilli(),
		Raw:       data,
	}

	for _, layerType := range d.decoded {
		// 由于 ip 层存在分片可能，所以这里不能直接处理UDP和TCP层，仅处理 ip 侧面还
		switch layerType {
		case layers.LayerTypeIPv4:
			atomic.AddUint64(&d.ipv4Count, 1)
			packet.FiveTuple = &otus.FiveTuple{
				SrcIP:    d.ip4.SrcIP,
				DstIP:    d.ip4.DstIP,
				Protocol: d.ip4.Protocol,
				SrcPort:  0,
				DstPort:  0,
			}

			// 检查是否需要分片重组
			if isFragmented(&d.ip4) {
				// 尝试重组分片
				reassembled, err := d.reassembleIPv4(d.ip4, ci.Timestamp)
				if err != nil {
					// 分片还未完成或出错，跳过此包
					continue
				}
				// 使用重组后的 IP 包
				d.ip4 = *reassembled
			}

			// 处理传输层（无分片或已重组）
			d.processTransport(&d.decoded, &d.udp, &d.tcp, packet)

		case layers.LayerTypeIPv6:
			atomic.AddUint64(&d.ipv6Count, 1)
			packet.FiveTuple = &otus.FiveTuple{
				SrcIP:    d.ip6.SrcIP,
				DstIP:    d.ip6.DstIP,
				Protocol: d.ip6.NextHeader,
				SrcPort:  0,
				DstPort:  0,
			}

			// IPv6 分片处理
			// 检查是否有 IPv6 分片扩展头
			hasFragment := false
			for _, layerType := range d.decoded {
				if layerType == layers.LayerTypeIPv6Fragment {
					hasFragment = true
					break
				}
			}

			if hasFragment {
				// 解析 IPv6 分片层
				pkt := gopacket.NewPacket(data, layers.LayerTypeIPv6, gopacket.Default)
				if fragLayer := pkt.Layer(layers.LayerTypeIPv6Fragment); fragLayer != nil {
					frag := fragLayer.(*layers.IPv6Fragment)
					// 尝试重组分片
					reassembled, err := d.reassembleIPv6(d.ip6, *frag, ci.Timestamp)
					if err != nil || reassembled == nil {
						// 分片还未完成或出错，跳过此包
						continue
					}
					// 使用重组后的 IP 包
					d.ip6 = *reassembled
				} else {
					// 无法解析分片层，跳过
					continue
				}
			}

			// 处理传输层（无分片或已重组）
			d.processTransport(&d.decoded, &d.udp, &d.tcp, packet)

		}
	}
	return packet, nil
}

func (d *Decoder) processTransport(decoded *[]gopacket.LayerType, udp *layers.UDP, tcp *layers.TCP, packet *otus.NetPacket) {
	for _, layerType := range *decoded {
		switch layerType {
		case layers.LayerTypeUDP:
			atomic.AddUint64(&d.udpCount, 1)
			if packet.FiveTuple != nil {
				packet.FiveTuple.SrcPort = uint16(udp.SrcPort)
				packet.FiveTuple.DstPort = uint16(udp.DstPort)
				packet.FiveTuple.Protocol = layers.IPProtocolUDP
			}
			// 结合端口和报文内容识别 UDP 应用层协议
			packet.Protocol = d.identifyUDPProtocol(udp, d.payload)

		case layers.LayerTypeTCP:
			atomic.AddUint64(&d.tcpCount, 1)
			if packet.FiveTuple != nil {
				packet.FiveTuple.SrcPort = uint16(tcp.SrcPort)
				packet.FiveTuple.DstPort = uint16(tcp.DstPort)
				packet.FiveTuple.Protocol = layers.IPProtocolTCP
			}
			// 结合端口和报文内容识别 TCP 应用层协议
			packet.Protocol = d.identifyTCPProtocol(tcp, d.payload)

		case layers.LayerTypeSCTP:
			// SCTP 协议处理（较少见）
			packet.Protocol = "SCTP"

		case layers.LayerTypeDNS:
			// DNS 协议（可能是 UDP 或 TCP）
			packet.Protocol = "DNS"
		}
	}
}

// identifyUDPProtocol 基于端口和载荷内容识别 UDP 应用层协议
func (d *Decoder) identifyUDPProtocol(udp *layers.UDP, payload gopacket.Payload) string {
	srcPort := uint16(udp.SrcPort)
	dstPort := uint16(udp.DstPort)
	data := []byte(payload)

	// 先通过报文内容进行深度检测
	if protocol := d.detectProtocolByContent(data); protocol != "" {
		return protocol
	}

	// 如果内容检测失败，回退到端口号判断（作为提示）
	switch {
	case srcPort == 53 || dstPort == 53:
		return "DNS"
	case srcPort == 67 || srcPort == 68 || dstPort == 67 || dstPort == 68:
		return "DHCP"
	case srcPort == 123 || dstPort == 123:
		return "NTP"
	case srcPort == 161 || srcPort == 162 || dstPort == 161 || dstPort == 162:
		return "SNMP"
	case srcPort == 5060 || dstPort == 5060 || srcPort == 5061 || dstPort == 5061:
		return "SIP" // 可能是 SIP，但需要内容验证
	case srcPort >= 16384 && srcPort <= 32767:
		return "RTP" // RTP 常用动态端口范围（可能性）
	case dstPort >= 16384 && dstPort <= 32767:
		return "RTP"
	case srcPort == 514 || dstPort == 514:
		return "Syslog"
	default:
		return "UDP"
	}
}

// identifyTCPProtocol 基于端口和载荷内容识别 TCP 应用层协议
func (d *Decoder) identifyTCPProtocol(tcp *layers.TCP, payload gopacket.Payload) string {
	srcPort := uint16(tcp.SrcPort)
	dstPort := uint16(tcp.DstPort)
	data := []byte(payload)

	// 先通过报文内容进行深度检测
	if protocol := d.detectProtocolByContent(data); protocol != "" {
		return protocol
	}

	// 如果内容检测失败，回退到端口号判断（作为提示）
	switch {
	case srcPort == 80 || dstPort == 80:
		return "HTTP"
	case srcPort == 443 || dstPort == 443:
		return "HTTPS"
	case srcPort == 21 || dstPort == 21:
		return "FTP"
	case srcPort == 22 || dstPort == 22:
		return "SSH"
	case srcPort == 23 || dstPort == 23:
		return "Telnet"
	case srcPort == 25 || dstPort == 25:
		return "SMTP"
	case srcPort == 110 || dstPort == 110:
		return "POP3"
	case srcPort == 143 || dstPort == 143:
		return "IMAP"
	case srcPort == 3306 || dstPort == 3306:
		return "MySQL"
	case srcPort == 5432 || dstPort == 5432:
		return "PostgreSQL"
	case srcPort == 6379 || dstPort == 6379:
		return "Redis"
	case srcPort == 27017 || dstPort == 27017:
		return "MongoDB"
	case srcPort == 5060 || dstPort == 5060:
		return "SIP"
	case srcPort == 8080 || dstPort == 8080:
		return "HTTP-Alt"
	case srcPort == 3389 || dstPort == 3389:
		return "RDP"
	default:
		return "TCP"
	}
}

// detectProtocolByContent 通过报文内容特征检测协议（深度包检测 DPI）
func (d *Decoder) detectProtocolByContent(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	// HTTP 检测：检查请求方法或响应状态码
	if d.isHTTP(data) {
		return "HTTP"
	}

	// SIP 检测：检查 SIP 消息特征
	if d.isSIP(data) {
		return "SIP"
	}

	// RTP 检测：检查 RTP 头部特征
	if d.isRTP(data) {
		return "RTP"
	}

	// DNS 检测：检查 DNS 头部
	if d.isDNS(data) {
		return "DNS"
	}

	// TLS/SSL 检测：检查握手消息
	if d.isTLS(data) {
		return "TLS"
	}

	// SSH 检测：检查协议标识
	if d.isSSH(data) {
		return "SSH"
	}

	// MySQL 检测：检查握手包
	if d.isMySQL(data) {
		return "MySQL"
	}

	// Redis 检测：检查 RESP 协议
	if d.isRedis(data) {
		return "Redis"
	}

	return ""
}

// isHTTP 检测是否为 HTTP 协议
func (d *Decoder) isHTTP(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	// 检查 HTTP 请求方法
	methods := []string{"GET ", "POST", "PUT ", "DELE", "HEAD", "OPTI", "PATC", "TRAC", "CONN"}
	for _, method := range methods {
		if len(data) >= len(method) && string(data[:len(method)]) == method {
			return true
		}
	}

	// 检查 HTTP 响应状态行
	if len(data) >= 5 && string(data[:5]) == "HTTP/" {
		return true
	}

	return false
}

// isSIP 检测是否为 SIP 协议
func (d *Decoder) isSIP(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	// SIP 请求方法
	methods := []string{"INVI", "REGI", "BYE ", "ACK ", "CANC", "OPTI", "INFO", "SUBS", "NOTI", "MESS", "REFE", "UPDA"}
	for _, method := range methods {
		if len(data) >= len(method) && string(data[:len(method)]) == method {
			return true
		}
	}

	// SIP 响应状态行
	if len(data) >= 7 && string(data[:7]) == "SIP/2.0" {
		return true
	}

	return false
}

// isRTP 检测是否为 RTP 协议
func (d *Decoder) isRTP(data []byte) bool {
	if len(data) < 12 {
		return false
	}

	// RTP 头部格式检查
	// Byte 0: V(2) P(1) X(1) CC(4)
	version := (data[0] >> 6) & 0x03
	if version != 2 {
		return false
	}

	// Byte 1: M(1) PT(7) - Payload Type
	payloadType := data[1] & 0x7F
	// 常见的 RTP Payload Types: 0-34 (音频), 96-127 (动态)
	if payloadType > 127 {
		return false
	}

	return true
}

// isDNS 检测是否为 DNS 协议
func (d *Decoder) isDNS(data []byte) bool {
	if len(data) < 12 {
		return false
	}

	// DNS 头部：
	// Flags 字节检查 (QR, Opcode, AA, TC, RD, RA, Z, RCODE)
	flags := uint16(data[2])<<8 | uint16(data[3])

	// Opcode 应该是 0-2 (标准查询、反向查询、状态请求)
	opcode := (flags >> 11) & 0x0F
	if opcode > 2 {
		return false
	}

	// RCODE 应该是 0-5
	rcode := flags & 0x0F
	return rcode <= 5
}

// isTLS 检测是否为 TLS/SSL 协议
func (d *Decoder) isTLS(data []byte) bool {
	if len(data) < 5 {
		return false
	}

	// TLS Record Layer 头部
	// Byte 0: Content Type (20-23)
	contentType := data[0]
	if contentType < 20 || contentType > 23 {
		return false
	}

	// Byte 1-2: Version (0x0301=TLS1.0, 0x0302=TLS1.1, 0x0303=TLS1.2, 0x0304=TLS1.3)
	version := uint16(data[1])<<8 | uint16(data[2])
	if version < 0x0301 || version > 0x0304 {
		return false
	}

	return true
}

// isSSH 检测是否为 SSH 协议
func (d *Decoder) isSSH(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	// SSH 协议标识：SSH-2.0-xxx 或 SSH-1.x-xxx
	return len(data) >= 4 && string(data[:4]) == "SSH-"
}

// isMySQL 检测是否为 MySQL 协议
func (d *Decoder) isMySQL(data []byte) bool {
	if len(data) < 5 {
		return false
	}

	// MySQL 握手包特征
	// Byte 4: 协议版本号 (通常是 10)
	if data[4] == 10 {
		// 进一步检查是否包含 MySQL 服务器版本字符串
		if len(data) > 10 {
			// 版本字符串通常以数字开头 (如 "5.7.x", "8.0.x")
			for i := 5; i < len(data)-1; i++ {
				if data[i] == 0 { // NULL 终止符
					versionStr := string(data[5:i])
					if len(versionStr) > 0 && versionStr[0] >= '0' && versionStr[0] <= '9' {
						return true
					}
					break
				}
			}
		}
	}

	return false
}

// isRedis 检测是否为 Redis RESP 协议
func (d *Decoder) isRedis(data []byte) bool {
	if len(data) < 1 {
		return false
	}

	// Redis RESP 协议特征
	// 第一个字节是类型标识符：
	// + 简单字符串, - 错误, : 整数, $ 批量字符串, * 数组
	firstByte := data[0]
	return firstByte == '+' || firstByte == '-' || firstByte == ':' || firstByte == '$' || firstByte == '*'
}
