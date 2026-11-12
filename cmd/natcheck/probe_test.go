package main

import (
	"encoding/binary"
	"hash/crc32"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTailscaleSTUNPacketAndXorDecode(t *testing.T) {
	txID := [12]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	req := BuildTailscaleSTUNRequest(txID)
	require.GreaterOrEqual(t, len(req), 28)

	// 验证末尾 8 字节为 RFC 5389 FINGERPRINT 属性
	fpAttrType := binary.BigEndian.Uint16(req[len(req)-8 : len(req)-6])
	require.Equal(t, uint16(stunAttrFingerprint), fpAttrType)
	expectedCRC := crc32.ChecksumIEEE(req[:len(req)-8]) ^ stunFingerprintXOR
	gotCRC := binary.BigEndian.Uint32(req[len(req)-4:])
	require.Equal(t, expectedCRC, gotCRC)

	// 构造标准 XOR-MAPPED-ADDRESS 响应 (203.0.113.45:51820，RFC 5737 文档示例地址)
	resp := make([]byte, 32)
	binary.BigEndian.PutUint16(resp[0:2], stunBindingResponse)
	binary.BigEndian.PutUint16(resp[2:4], 12)
	binary.BigEndian.PutUint32(resp[4:8], stunMagicCookie)
	copy(resp[8:20], txID[:])
	binary.BigEndian.PutUint16(resp[20:22], stunAttrXorMapped)
	binary.BigEndian.PutUint16(resp[22:24], 8)
	resp[24] = 0x00
	resp[25] = 0x01 // IPv4
	binary.BigEndian.PutUint16(resp[26:28], uint16(51820)^uint16(stunMagicCookie>>16))
	var cookieBytes [4]byte
	binary.BigEndian.PutUint32(cookieBytes[:], stunMagicCookie)
	sampleIP := net.IPv4(203, 0, 113, 45).To4()
	for i := 0; i < 4; i++ {
		resp[28+i] = sampleIP[i] ^ cookieBytes[i]
	}

	gotIP, gotPort, err := ParseSTUNBindingResponse(resp, txID)
	require.NoError(t, err)
	require.Equal(t, "203.0.113.45", gotIP.String())
	require.Equal(t, 51820, gotPort)
	require.Equal(t, "public-v4", ClassifyIPScope(gotIP))
	require.Equal(t, "cgnat-v4", ClassifyIPScope(net.IPv4(100, 100, 12, 34)))
}

func TestAnalyzeNATMappingsAndPrivacyRedaction(t *testing.T) {
	coneType, spread, preserve := AnalyzeNATMappings(54000, []observedMapping{
		{ServerLabel: "s1", IPKey: "ip#1", IPScope: "public-v4", Port: 54000},
		{ServerLabel: "s2", IPKey: "ip#1", IPScope: "public-v4", Port: 54000},
	})
	require.Equal(t, "cone", coneType)
	require.Equal(t, 0, spread)
	require.True(t, preserve)

	coneWithHairpin, hpSpread, hpPreserve := AnalyzeNATMappings(61022, []observedMapping{
		{ServerLabel: "control-plane-stun:3479", IPKey: "ip#1", IPScope: "private-v4", Port: 65322},
		{ServerLabel: "public-stun-1", IPKey: "ip#2", IPScope: "public-v4", Port: 61022},
		{ServerLabel: "public-stun-3", IPKey: "ip#2", IPScope: "public-v4", Port: 61022},
	})
	require.Equal(t, "cone", coneWithHairpin)
	require.Equal(t, 0, hpSpread)
	require.True(t, hpPreserve)

	symType, symSpread, symPreserve := AnalyzeNATMappings(54000, []observedMapping{
		{ServerLabel: "s1", IPKey: "ip#1", IPScope: "public-v4", Port: 61001},
		{ServerLabel: "s2", IPKey: "ip#1", IPScope: "public-v4", Port: 61018},
	})
	require.Equal(t, "symmetric_port", symType)
	require.Equal(t, 17, symSpread)
	require.False(t, symPreserve)

	report := NATReport{
		GeneratedAtUTC:       "2026-11-12T12:00:00Z",
		LocalUDPPort:         54000,
		ControlSTUNTested:    true,
		ControlSTUNReachable: true,
		Observations: []STUNObservation{
			{ServerLabel: "control-plane-stun:3479", Reachable: true, RTTMs: 250, IPScope: "public-v4", IPBucket: "ip#1", MappedPort: 54000},
			{ServerLabel: "public-stun-1", Reachable: true, RTTMs: 25, IPScope: "public-v4", IPBucket: "ip#1", MappedPort: 54000},
		},
		RespondedCount:      2,
		NATMappingType:      "cone",
		Firewall:            FirewallStatus{Verdict: "missing_rule_public", ActiveProfiles: []string{"Public"}},
	}
	SynthesizeVerdict(&report)
	require.Equal(t, "conditional", report.SupportsUDPPunching)

	md := RenderMarkdownReport(report)
	require.Contains(t, md, "cone")
	require.Contains(t, md, "missing_rule_public")
	require.False(t, strings.Contains(md, "203.0.113"), "报告不得包含任何真实公网 IP")
}
