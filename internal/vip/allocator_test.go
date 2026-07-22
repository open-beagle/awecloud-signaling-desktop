package vip

import (
	"testing"
)

func TestReleaseRemovesBothVIPMappings(t *testing.T) {
	allocator := NewAllocator()
	address, err := allocator.Allocate("resource.container.beagle")
	if err != nil {
		t.Fatal(err)
	}
	allocator.Release("resource.container.beagle")
	_, ok := allocator.GetVIP("resource.container.beagle")
	if ok {
		t.Fatal("expected domain mapping to be released")
	}
	_, ok = allocator.Resolve(address)
	if ok {
		t.Fatal("expected reverse VIP mapping to be released")
	}
}
