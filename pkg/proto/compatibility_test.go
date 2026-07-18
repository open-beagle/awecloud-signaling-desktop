package proto

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

// Old Desktop builds use this schema, which predates field 4 on
// GetResourcesResponse. Protobuf must ignore the new ContainerSSH projection
// while preserving the legacy resource lists.
func TestLegacyGetResourcesResponseIgnoresContainerSSHField(t *testing.T) {
	legacySSH, err := proto.Marshal(&SSHResource{AgentId: 7, AgentName: "legacy-agent", Domain: "agent.beagle"})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte{0x0a, byte(len(legacySSH))}
	payload = append(payload, legacySSH...)
	// Field 4 is the new repeated ContainerSSHResource. The old schema does
	// not know it and must continue decoding fields 1-3 normally.
	payload = append(payload, 0x22, 0x03, 0x0a, 0x01, 0x78)

	var response GetResourcesResponse
	if err := proto.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.GetSsh()) != 1 {
		t.Fatalf("expected one legacy SSH resource, got %d", len(response.GetSsh()))
	}
	if response.GetSsh()[0].GetAgentId() != 7 {
		t.Fatalf("unexpected legacy Agent ID: %d", response.GetSsh()[0].GetAgentId())
	}
}
