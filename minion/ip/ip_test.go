package ip

import (
	"math/rand"
	"net"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestSyncIPs(t *testing.T) {
	prefix := net.IPv4(10, 0, 0, 0)

	nextRand := uint32(0)
	Rand32 = func() uint32 {
		ret := nextRand
		nextRand++
		return ret
	}

	defer func() {
		Rand32 = rand.Uint32
	}()

	ipMap := map[string]string{
		"a": "",
		"b": "",
		"c": "",
	}

	mask := net.CIDRMask(20, 32)
	Sync(ipMap, prefix, mask)

	// 10.0.0.1 is reserved for the default gateway
	exp := sliceToSet([]string{"10.0.0.0", "10.0.0.2", "10.0.0.3"})
	ipSet := map[string]struct{}{}
	for _, ip := range ipMap {
		ipSet[ip] = struct{}{}
	}

	if !eq(ipSet, exp) {
		t.Error(spew.Sprintf("Unexpected IP allocations."+
			"\nFound %s\nExpected %s\nMap %s",
			ipSet, exp, ipMap))
	}

	ipMap["d"] = "junk"

	Sync(ipMap, prefix, mask)

	aIP := ipMap["d"]
	expected := "10.0.0.4"
	if aIP != expected {
		t.Error(spew.Sprintf("Unexpected IP allocations.\nFound %s\nExpected %s",
			aIP, expected))
	}

	// Force collisions
	Rand32 = func() uint32 {
		return 4
	}

	ipMap["a"] = "10.0.0.0"
	ipMap["b"] = "10.0.0.2"
	ipMap["c"] = "10.0.0.3"
	ipMap["d"] = "junk"

	Sync(ipMap, prefix, net.CIDRMask(30, 32)) // only 3 viable address in this mask

	if ip, _ := ipMap["d"]; ip != "" {
		t.Error(spew.Sprintf("Expected IP deletion, found %s", ip))
	}
}

func TestParseIP(t *testing.T) {
	expected := net.IPv4(0x01, 0, 0, 0)
	mask := net.CIDRMask(8, 32)
	res := Parse("1.0.0.0", expected, mask)
	if !res.Equal(expected) {
		t.Errorf("parseIP expected 0x%x, got 0x%x", 0x01000000, res)
	}

	res = Parse("2.0.0.1", expected, mask)
	if !res.Equal(net.IPv4zero) {
		t.Errorf("parseIP expected 0x%x, got 0x%x", 0, res)
	}

	res = Parse("a", expected, mask)
	if !res.Equal(net.IPv4zero) {
		t.Errorf("parseIP expected 0x%x, got 0x%x", 0, res)
	}
}

func TestMaskToInt(t *testing.T) {
	mask := net.CIDRMask(16, 32)
	if MaskToInt(mask) != 0xffff0000 {
		t.Fatalf("Wrong mask int, expected 0xffff0000, got %x", MaskToInt(mask))
	}

	mask = net.CIDRMask(19, 32)
	if MaskToInt(mask) != 0xffffe000 {
		t.Fatalf("Wrong mask int, expected 0xffffe000, got %x", MaskToInt(mask))
	}

	mask = net.CIDRMask(32, 32)
	if MaskToInt(mask) != 0xffffffff {
		t.Fatalf("Wrong mask int, expected 0xffffffff, got %x", MaskToInt(mask))
	}
}

func TestRandomIP(t *testing.T) {
	prefix := net.IPv4(0xaa, 0xbb, 0xcc, 0xdd)
	mask := net.CIDRMask(20, 32)
	conflicts := map[string]struct{}{}

	// Only 4k IPs, in 0xfff00000. Guaranteed a collision
	for i := 0; i < 5000; i++ {
		ip := Random(conflicts, prefix, mask)
		if ip.Equal(net.IPv4zero) {
			continue
		}

		if _, ok := conflicts[ip.String()]; ok {
			t.Fatalf("IP Double allocation: 0x%x", ip)
		}

		if !prefix.Mask(mask).Equal(ip.Mask(mask)) {
			t.Fatalf("Bad IP allocation: %v & %v != %v",
				ip, mask, prefix.Mask(mask))
		}

		conflicts[ip.String()] = struct{}{}
	}

	if len(conflicts) < 2500 || len(conflicts) > 4096 {
		// If the code's working, this is possible but *extremely* unlikely.
		// Probably a bug.
		t.Errorf("Too few conflicts: %d", len(conflicts))
	}
}

func sliceToSet(slice []string) map[string]struct{} {
	res := map[string]struct{}{}
	for _, s := range slice {
		res[s] = struct{}{}
	}
	return res
}

func eq(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}
