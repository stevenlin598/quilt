package ip

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"net"

	"github.com/NetSys/quilt/minion/network"

	log "github.com/Sirupsen/logrus"
)

var (
	// Rand32 stores rand.Uint32 in a variable so that it can be easily mocked out
	// for unit tests. Nondeterminism is hard to test.
	Rand32 = rand.Uint32

	// QuiltPrefix is the subnet under which quilt containers are given IP addresses
	QuiltPrefix = net.IPv4(10, 0, 0, 0)

	// QuiltMask is the subnet mask that corresponds with the Quilt subnet prefix
	QuiltMask = net.CIDRMask(8, 32)

	// SubMask is the subnet mask that minions can assign container IPs within. It
	// resprents a /20 subnet.
	SubMask = net.CIDRMask(20, 32)

	// LabelPrefix is the subnet that is reserved for label IPs. It represents
	// 10.0.0.0/20
	LabelPrefix = net.IPv4(10, 0, 0, 0) // Labels get their own /20

	minionMaskBits, _ = SubMask.Size()
	quiltMaskBits, _  = QuiltMask.Size()

	// MaxMinionCount is the largest number of minions that can exist, based
	// on the number of available subnets
	MaxMinionCount = int(math.Pow(2, float64(minionMaskBits-quiltMaskBits))+0.5) - 1
)

// Sync takes a map of IDs to IPs and creates an IP address for every entry that's
// missing one.
func Sync(ipMap map[string]string, prefixIP net.IP, mask net.IPMask) {
	var unassigned []string
	ipSet := map[string]struct{}{}
	for k, ipString := range ipMap {
		ip := Parse(ipString, prefixIP, mask)
		if !ip.Equal(net.IPv4zero) {
			ipSet[ip.String()] = struct{}{}
		} else {
			unassigned = append(unassigned, k)
		}
	}

	// Don't assign the IP of the default gateway
	ipSet[network.GatewayIP] = struct{}{}
	for _, k := range unassigned {
		ip := Random(ipSet, prefixIP, mask)
		if ip.Equal(net.IPv4zero) {
			log.Errorf("Failed to allocate IP for %s.", k)
			ipMap[k] = ""
			continue
		}

		ipMap[k] = ip.String()
		ipSet[ip.String()] = struct{}{}
	}
}

// Parse takes in an IP string, and parses it with respect to the given prefix and mask.
func Parse(ipStr string, prefix net.IP, mask net.IPMask) net.IP {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return net.IPv4zero
	}

	if !ip.Mask(mask).Equal(prefix) {
		return net.IPv4zero
	}

	return ip
}

// MaskToInt takes in a CIDR Mask and return the integer representation of it.
func MaskToInt(mask net.IPMask) uint32 {
	bits, _ := mask.Size()
	return 0xffffffff ^ uint32(0xffffffff>>uint(bits))
}

// FromInt converts the given integer into the equivalent IP address.
func FromInt(ip32 uint32) net.IP {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, ip32)
	return net.IP(b)
}

// Random selects a random IP address in the given prefix and mask that isn't already
// present in conflicts.
// Returns 0 on failure.
func Random(conflicts map[string]struct{}, pre net.IP, subnetMask net.IPMask) net.IP {
	prefix := ToInt(pre)
	mask := MaskToInt(subnetMask)

	randStart := Rand32() & ^mask
	randIP := randStart
	for {
		ip32 := randIP | (prefix & mask)
		ip := FromInt(ip32)
		if _, ok := conflicts[ip.String()]; !ok {
			return ip
		}

		randIP = (randIP + 1) & ^mask
		// Prevent infinite looping in the case that all IPs are taken
		if randIP == randStart {
			return net.IPv4zero
		}
	}
}

// ToMac converts the given IP address string into an internal MAC address.
func ToMac(ipStr string) string {
	parsedIP := net.ParseIP(ipStr)
	if parsedIP == nil {
		return ""
	}

	ip := parsedIP.To4()
	return fmt.Sprintf("02:00:%02x:%02x:%02x:%02x", ip[0], ip[1], ip[2], ip[3])
}

// ToInt converts the given IP address into the equivalent integer representation.
func ToInt(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip.To4())
}
