package utils

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	"golang.org/x/net/bpf"
)

// CompileBpf 编译 tcpdump 风格的过滤字符串为 BPF 指令
// 只解析 IP 层条件，避免传输层条件导致的 IP 分片问题
func CompileBpf(filter string) ([]bpf.RawInstruction, error) {
	if filter == "" {
		// 默认返回 IPv4 过滤器
		return compileIPv4Filter()
	}

	// 清理输入字符串
	filter = strings.TrimSpace(strings.ToLower(filter))

	// 解析 IP 条件
	ipConditions, err := parseIPConditions(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to parse IP conditions: %v", err)
	}

	// 如果没有找到任何 IP 条件，返回默认 IPv4 过滤器
	if len(ipConditions) == 0 {
		return compileIPv4Filter()
	}

	// 编译 BPF 指令
	return compileBPFInstructions(ipConditions)
}

// IPCondition 表示一个 IP 条件
type IPCondition struct {
	Protocol string // "ipv4" 或 "ipv6"
	SrcIP    net.IP
	DstIP    net.IP
	HostIP   net.IP // host 条件（既作为源也作为目的）
	Network  *net.IPNet
}

// parseIPConditions 解析过滤字符串中的 IP 条件
func parseIPConditions(filter string) ([]IPCondition, error) {
	var conditions []IPCondition

	// 匹配 IPv4/IPv6 协议
	if matched, _ := regexp.MatchString(`\b(ip6?|ipv6)\b`, filter); matched {
		if strings.Contains(filter, "ip6") || strings.Contains(filter, "ipv6") {
			conditions = append(conditions, IPCondition{Protocol: "ipv6"})
		} else {
			conditions = append(conditions, IPCondition{Protocol: "ipv4"})
		}
	}

	// 匹配源 IP 地址: src 192.168.1.1
	srcIPRegex := regexp.MustCompile(`\bsrc\s+([0-9a-fA-F.:]+)`)
	if matches := srcIPRegex.FindStringSubmatch(filter); len(matches) > 1 {
		ip := net.ParseIP(matches[1])
		if ip != nil {
			condition := IPCondition{SrcIP: ip}
			if ip.To4() != nil {
				condition.Protocol = "ipv4"
			} else {
				condition.Protocol = "ipv6"
			}
			conditions = append(conditions, condition)
		}
	}

	// 匹配目的 IP 地址: dst 192.168.1.1
	dstIPRegex := regexp.MustCompile(`\bdst\s+([0-9a-fA-F.:]+)`)
	if matches := dstIPRegex.FindStringSubmatch(filter); len(matches) > 1 {
		ip := net.ParseIP(matches[1])
		if ip != nil {
			condition := IPCondition{DstIP: ip}
			if ip.To4() != nil {
				condition.Protocol = "ipv4"
			} else {
				condition.Protocol = "ipv6"
			}
			conditions = append(conditions, condition)
		}
	}

	// 匹配主机 IP 地址: host 192.168.1.1
	hostIPRegex := regexp.MustCompile(`\bhost\s+([0-9a-fA-F.:]+)`)
	if matches := hostIPRegex.FindStringSubmatch(filter); len(matches) > 1 {
		ip := net.ParseIP(matches[1])
		if ip != nil {
			condition := IPCondition{HostIP: ip}
			if ip.To4() != nil {
				condition.Protocol = "ipv4"
			} else {
				condition.Protocol = "ipv6"
			}
			conditions = append(conditions, condition)
		}
	}

	// 匹配网络段: net 192.168.1.0/24
	netRegex := regexp.MustCompile(`\bnet\s+([0-9a-fA-F.:]+/\d+)`)
	if matches := netRegex.FindStringSubmatch(filter); len(matches) > 1 {
		_, network, err := net.ParseCIDR(matches[1])
		if err == nil {
			condition := IPCondition{Network: network}
			if network.IP.To4() != nil {
				condition.Protocol = "ipv4"
			} else {
				condition.Protocol = "ipv6"
			}
			conditions = append(conditions, condition)
		}
	}

	return conditions, nil
}

// compileIPv4Filter 返回默认的 IPv4 过滤器
func compileIPv4Filter() ([]bpf.RawInstruction, error) {
	instructions := []bpf.Instruction{
		// 加载以太网类型字段 (偏移 12，2 字节)
		&bpf.LoadAbsolute{Off: 12, Size: 2},
		// 如果是 IPv4 (0x0800)，跳过下一条指令
		&bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x0800, SkipFalse: 1},
		// 返回整个数据包 (65535 字节)
		&bpf.RetConstant{Val: 65535},
		// 丢弃数据包
		&bpf.RetConstant{Val: 0},
	}

	return bpf.Assemble(instructions)
}

// compileIPv6Filter 返回 IPv6 过滤器
func compileIPv6Filter() ([]bpf.RawInstruction, error) {
	instructions := []bpf.Instruction{
		// 加载以太网类型字段 (偏移 12，2 字节)
		&bpf.LoadAbsolute{Off: 12, Size: 2},
		// 如果是 IPv6 (0x86DD)，跳过下一条指令
		&bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x86DD, SkipFalse: 1},
		// 返回整个数据包
		&bpf.RetConstant{Val: 65535},
		// 丢弃数据包
		&bpf.RetConstant{Val: 0},
	}

	return bpf.Assemble(instructions)
}

// compileBPFInstructions 根据 IP 条件编译 BPF 指令
func compileBPFInstructions(conditions []IPCondition) ([]bpf.RawInstruction, error) {
	// 如果只有协议条件
	if len(conditions) == 1 && conditions[0].SrcIP == nil && conditions[0].DstIP == nil &&
		conditions[0].HostIP == nil && conditions[0].Network == nil {
		if conditions[0].Protocol == "ipv6" {
			return compileIPv6Filter()
		}
		return compileIPv4Filter()
	}

	// 处理复杂条件 - 这里简化处理，主要处理单个 IP 地址条件
	for _, condition := range conditions {
		if condition.SrcIP != nil {
			if condition.Protocol == "ipv4" || condition.SrcIP.To4() != nil {
				return compileSrcIPv4Filter(condition.SrcIP.To4())
			}
		} else if condition.DstIP != nil {
			if condition.Protocol == "ipv4" || condition.DstIP.To4() != nil {
				return compileDstIPv4Filter(condition.DstIP.To4())
			}
		} else if condition.HostIP != nil {
			if condition.Protocol == "ipv4" || condition.HostIP.To4() != nil {
				return compileHostIPv4Filter(condition.HostIP.To4())
			}
		}
	}

	// 如果没有生成任何指令，返回默认 IPv4 过滤器
	return compileIPv4Filter()
}

// compileSrcIPv4Filter 编译源 IPv4 地址过滤器
func compileSrcIPv4Filter(ip net.IP) ([]bpf.RawInstruction, error) {
	if ip == nil {
		return compileIPv4Filter()
	}

	ipAddr := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])

	instructions := []bpf.Instruction{
		// 检查以太网类型是否为 IPv4
		&bpf.LoadAbsolute{Off: 12, Size: 2},
		&bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: 0x0800, SkipTrue: 4},
		// 加载源 IP 地址 (以太网头 14 字节 + IP 头源地址偏移 12 字节)
		&bpf.LoadAbsolute{Off: 26, Size: 4},
		&bpf.JumpIf{Cond: bpf.JumpEqual, Val: ipAddr, SkipFalse: 1},
		&bpf.RetConstant{Val: 65535},
		&bpf.RetConstant{Val: 0},
	}

	return bpf.Assemble(instructions)
}

// compileDstIPv4Filter 编译目的 IPv4 地址过滤器
func compileDstIPv4Filter(ip net.IP) ([]bpf.RawInstruction, error) {
	if ip == nil {
		return compileIPv4Filter()
	}

	ipAddr := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])

	instructions := []bpf.Instruction{
		// 检查以太网类型是否为 IPv4
		&bpf.LoadAbsolute{Off: 12, Size: 2},
		&bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: 0x0800, SkipTrue: 4},
		// 加载目的 IP 地址 (以太网头 14 字节 + IP 头目的地址偏移 16 字节)
		&bpf.LoadAbsolute{Off: 30, Size: 4},
		&bpf.JumpIf{Cond: bpf.JumpEqual, Val: ipAddr, SkipFalse: 1},
		&bpf.RetConstant{Val: 65535},
		&bpf.RetConstant{Val: 0},
	}

	return bpf.Assemble(instructions)
}

// compileHostIPv4Filter 编译主机 IPv4 地址过滤器 (源或目的)
func compileHostIPv4Filter(ip net.IP) ([]bpf.RawInstruction, error) {
	if ip == nil {
		return compileIPv4Filter()
	}

	ipAddr := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])

	instructions := []bpf.Instruction{
		// 检查以太网类型是否为 IPv4
		&bpf.LoadAbsolute{Off: 12, Size: 2},
		&bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: 0x0800, SkipTrue: 6},
		// 加载源 IP 地址
		&bpf.LoadAbsolute{Off: 26, Size: 4},
		&bpf.JumpIf{Cond: bpf.JumpEqual, Val: ipAddr, SkipTrue: 2}, // 如果源 IP 匹配，跳到返回
		// 加载目的 IP 地址
		&bpf.LoadAbsolute{Off: 30, Size: 4},
		&bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: ipAddr, SkipTrue: 1}, // 如果目的 IP 不匹配，跳到丢弃
		&bpf.RetConstant{Val: 65535},                                  // 返回整个数据包
		&bpf.RetConstant{Val: 0},                                      // 丢弃数据包
	}

	return bpf.Assemble(instructions)
}

// ValidateFilter 验证过滤字符串是否有效
func ValidateFilter(filter string) error {
	_, err := CompileBpf(filter)
	return err
}
