package main

import (
	"fmt"
	"net"
	"testing"
)

func TestParseSubnetRangeSubnetOnly(t *testing.T) {
	start := net.ParseIP("10.100.0.1")
	sub := fmt.Sprintf("%s/24", start.String())

	r, err := parseSubnetRange(sub)
	if err != nil {
		t.Fatal(err)
	}

	if !r.Start.Equal(start) {
		t.Fatalf("expected start ip of %s; received %s", start, r.Start)
	}

	if !r.End.Equal(net.ParseIP("10.100.0.254")) {
		t.Fatalf("expected end of 10.100.0.254; received %s", r.End)
	}
}

func TestParseSubnetRangeStartEnd(t *testing.T) {
	start := net.ParseIP("10.100.0.100")
	end := net.ParseIP("10.100.0.200")
	sub := fmt.Sprintf("%s-%s/24", start.String(), end.String())

	r, err := parseSubnetRange(sub)
	if err != nil {
		t.Fatal(err)
	}

	if !r.Start.Equal(start) {
		t.Fatalf("expected start ip of %s; received %s", start, r.Start)
	}

	if !r.End.Equal(end) {
		t.Fatalf("expected end of %s; received %s", end, r.End)
	}
}

func TestParseSubnetRangeInvalid(t *testing.T) {
	sub := "1.2.3.4-254/24"

	if _, err := parseSubnetRange(sub); err == nil {
		t.Fatal("expected err for invalid format")
	}
}
