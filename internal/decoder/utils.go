package decoder

import "github.com/google/gopacket/layers"

func isFragmented(ip4 *layers.IPv4) bool {
	return ip4.Flags&layers.IPv4DontFragment != 0 || (ip4.Flags&layers.IPv4MoreFragments == 0 && ip4.FragOffset == 0)
}
