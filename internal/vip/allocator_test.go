package vip

import (
	"net"
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

func TestDeterministicVIPOrderIndependentAndCollisionSafe(t *testing.T) {
	a1 := NewAllocator()
	vHome1, err := a1.Allocate("home.alice.acme")
	if err != nil {
		t.Fatal(err)
	}
	vWork1, err := a1.Allocate("work.alice.acme")
	if err != nil {
		t.Fatal(err)
	}

	// 交换分配顺序的新实例必须得到完全一致的确定性 VIP
	a2 := NewAllocator()
	vWork2, err := a2.Allocate("WORK.ALICE.ACME.")
	if err != nil {
		t.Fatal(err)
	}
	vHome2, err := a2.Allocate("home.alice.acme")
	if err != nil {
		t.Fatal(err)
	}

	if vHome1 != vHome2 || vHome1 != DeterministicVIP("home.alice.acme") {
		t.Fatalf("expected deterministic home VIP %s, got %s", DeterministicVIP("home.alice.acme"), vHome2)
	}
	if vWork1 != vWork2 || vWork1 != DeterministicVIP("work.alice.acme") {
		t.Fatalf("expected deterministic work VIP %s, got %s", DeterministicVIP("work.alice.acme"), vWork2)
	}

	for _, addr := range []string{vHome1, vWork1} {
		ip := net.ParseIP(addr).To4()
		if ip == nil || ip[0] != 127 || ip[1] != 1 || ip[3] == 0 || ip[3] == 255 {
			t.Fatalf("unexpected VIP out of 127.1.<0..255>.<1..254>: %s", addr)
		}
	}

	// 人为制造哈希槽冲突，验证线性探测不会覆盖既有域名
	slot := deterministicVIPSlot("home.alice.acme")
	occupiedVIP := vipFromSlot(slot)
	a3 := NewAllocator()
	a3.domainToVIP["preoccupied.alice.acme"] = occupiedVIP
	a3.vipToDomain[occupiedVIP] = "preoccupied.alice.acme"

	probed, err := a3.Allocate("home.alice.acme")
	if err != nil {
		t.Fatal(err)
	}
	if probed == occupiedVIP {
		t.Fatalf("expected linear probing to skip occupied VIP %s", occupiedVIP)
	}
	if probed != vipFromSlot(slot+1) {
		t.Fatalf("expected next slot VIP %s, got %s", vipFromSlot(slot+1), probed)
	}
}
