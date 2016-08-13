package api

import (
	"errors"
	"testing"
)

func checkAddr(t *testing.T, testAddr string, expProto string, expAddr string) {
	proto, addr, err := ParseListenAddress(testAddr)

	if err != nil {
		t.Errorf("Unexpected error: %s", err.Error())
		return
	}

	if proto != expProto {
		t.Errorf("Uxpected protocol for %s: expected %s, "+
			"got %s.", testAddr, expProto, proto)
	}

	if addr != expAddr {
		t.Errorf("Unexpected addr for %s: expected %s, got %s.",
			testAddr, expAddr, addr)
	}
}

func checkAddrError(t *testing.T, testAddr string, expErr error) {
	_, _, err := ParseListenAddress(testAddr)

	if err == nil {
		t.Errorf("Didn't get an error, expected %s", expErr.Error())
	}

	if expErr.Error() != err.Error() {
		t.Errorf("Unexpected error: expected %s, got %s",
			expErr.Error(), err.Error())
	}
}

func TestAddrParsing(t *testing.T) {
	t.Parallel()

	checkAddr(t, "tcp://8.8.8.8:9000", "tcp", "8.8.8.8:9000")
	checkAddr(t, "unix:///tmp/quilt.sock", "unix", "/tmp/quilt.sock")
	checkAddrError(t, "/tmp/quilt.sock",
		errors.New("malformed listen address: /tmp/quilt.sock"))
	checkAddrError(t, "8.8.8.8", errors.New("malformed listen address: 8.8.8.8"))
}

func TestRemoteAddress(t *testing.T) {
	t.Parallel()

	exp := "tcp://8.8.8.8:9000"
	if res := RemoteAddress("8.8.8.8"); res != exp {
		t.Errorf("Bad remote address result: expected %s, got %s", exp, res)
	}
}
