package proto

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
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

	legacyFile, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Syntax: proto.String("proto3"), Name: proto.String("legacy_desktop.proto"), Package: proto.String("legacy"),
		Dependency: []string{"desktop/pkg/proto/desktop.proto"},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("GetResourcesResponse"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: proto.String("ssh"), Number: proto.Int32(1),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				TypeName: proto.String(".awecloud.signaling.SSHResource"),
			}},
		}},
	}, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatal(err)
	}
	response := dynamicpb.NewMessage(legacyFile.Messages().ByName(protoreflect.Name("GetResourcesResponse")))
	if err := proto.Unmarshal(payload, response); err != nil {
		t.Fatal(err)
	}
	sshField := response.Descriptor().Fields().ByName("ssh")
	sshResources := response.Get(sshField).List()
	if sshResources.Len() != 1 {
		t.Fatalf("expected one legacy SSH resource, got %d", sshResources.Len())
	}
	sshResource := sshResources.Get(0).Message()
	agentID := sshResource.Get(sshResource.Descriptor().Fields().ByName("agent_id")).Uint()
	if agentID != 7 {
		t.Fatalf("unexpected legacy Agent ID: %d", agentID)
	}
}

func TestCurrentGetResourcesResponseReadsContainerSSHField(t *testing.T) {
	payload, err := proto.Marshal(&GetResourcesResponse{ContainerSsh: []*ContainerSSHResource{{
		ResourceId: "resource-a", Domain: "resource-a.container.beagle", AgentIp: "100.64.0.22", ListenPort: 50200,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	var response GetResourcesResponse
	if err := proto.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.GetContainerSsh()) != 1 || response.GetContainerSsh()[0].GetListenPort() != 50200 {
		t.Fatalf("unexpected ContainerSSH projection: %#v", response.GetContainerSsh())
	}
}

func TestGetResourcesRequestNegotiatesV2Additively(t *testing.T) {
	request := &GetResourcesRequest{DesktopId: 7, ResourceProtocol: "resource_session_v2", TenantId: "tenant-a"}
	payload, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	legacyFile, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Syntax: proto.String("proto3"), Name: proto.String("legacy_desktop_request.proto"), Package: proto.String("legacyrequest"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("GetResourcesRequest"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: proto.String("desktop_id"), Number: proto.Int32(1),
				Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT64.Enum(),
			}},
		}},
	}, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatal(err)
	}
	legacyRequest := dynamicpb.NewMessage(legacyFile.Messages().ByName("GetResourcesRequest"))
	if err := proto.Unmarshal(payload, legacyRequest); err != nil {
		t.Fatal(err)
	}
	desktopID := legacyRequest.Descriptor().Fields().ByName("desktop_id")
	if got := legacyRequest.Get(desktopID).Uint(); got != 7 {
		t.Fatalf("legacy request lost desktop_id: %d", got)
	}
	fields := request.ProtoReflect().Descriptor().Fields()
	if fields.ByName("resource_protocol").Number() != 2 || fields.ByName("tenant_id").Number() != 3 {
		t.Fatalf("unexpected additive request field numbers")
	}
}

func TestCurrentGetResourcesResponseReadsContainerServiceV2Field(t *testing.T) {
	payload, err := proto.Marshal(&GetResourcesResponse{ContainerService: []*ContainerServiceResource{{
		ResourceId: "resource-service-a", SessionId: "session-a", ServiceUid: "service-a",
		PortNumber: 443, Protocol: "TCP", AuthorizationRevision: 7,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	var response GetResourcesResponse
	if err := proto.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.GetContainerService()) != 1 || response.GetContainerService()[0].GetSessionId() != "session-a" || response.GetContainerService()[0].GetPortNumber() != 443 {
		t.Fatalf("unexpected ContainerService projection: %#v", response.GetContainerService())
	}
}

func TestDesktopSVCProxyV2FieldsKeepAgentWireNumbers(t *testing.T) {
	message := (&SVCProxyData{}).ProtoReflect().Descriptor().Fields()
	for name, number := range map[string]protoreflect.FieldNumber{
		"session_id": 8, "resource_id": 9, "source_id": 10, "target_revision_id": 11,
		"service_uid": 12, "port_name": 13, "protocol": 14, "authorization_revision": 15,
	} {
		field := message.ByName(protoreflect.Name(name))
		if field == nil || field.Number() != number {
			t.Fatalf("unexpected SVCProxyData field %s: %#v", name, field)
		}
	}
}
