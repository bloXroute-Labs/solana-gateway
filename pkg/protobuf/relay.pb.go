// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.6
// 	protoc        v6.30.2
// source: relay.proto

package protobuf

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type RegisterRequest struct {
	state      protoimpl.MessageState `protogen:"open.v1"`
	AuthHeader string                 `protobuf:"bytes,1,opt,name=auth_header,json=authHeader,proto3" json:"auth_header,omitempty"`
	// Deprecated: Marked as deprecated in relay.proto.
	Version string `protobuf:"bytes,2,opt,name=version,proto3" json:"version,omitempty"`
	// Deprecated: Marked as deprecated in relay.proto.
	ServerPort           int64  `protobuf:"varint,3,opt,name=server_port,json=serverPort,proto3" json:"server_port,omitempty"`
	GatewayConfiguration []byte `protobuf:"bytes,4,opt,name=gateway_configuration,json=gatewayConfiguration,proto3" json:"gateway_configuration,omitempty"`
	unknownFields        protoimpl.UnknownFields
	sizeCache            protoimpl.SizeCache
}

func (x *RegisterRequest) Reset() {
	*x = RegisterRequest{}
	mi := &file_relay_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *RegisterRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RegisterRequest) ProtoMessage() {}

func (x *RegisterRequest) ProtoReflect() protoreflect.Message {
	mi := &file_relay_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RegisterRequest.ProtoReflect.Descriptor instead.
func (*RegisterRequest) Descriptor() ([]byte, []int) {
	return file_relay_proto_rawDescGZIP(), []int{0}
}

func (x *RegisterRequest) GetAuthHeader() string {
	if x != nil {
		return x.AuthHeader
	}
	return ""
}

// Deprecated: Marked as deprecated in relay.proto.
func (x *RegisterRequest) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

// Deprecated: Marked as deprecated in relay.proto.
func (x *RegisterRequest) GetServerPort() int64 {
	if x != nil {
		return x.ServerPort
	}
	return 0
}

func (x *RegisterRequest) GetGatewayConfiguration() []byte {
	if x != nil {
		return x.GatewayConfiguration
	}
	return nil
}

type RegisterResponse struct {
	state               protoimpl.MessageState `protogen:"open.v1"`
	UdpAddress          string                 `protobuf:"bytes,1,opt,name=udp_address,json=udpAddress,proto3" json:"udp_address,omitempty"`
	JwtToken            string                 `protobuf:"bytes,2,opt,name=jwt_token,json=jwtToken,proto3" json:"jwt_token,omitempty"`
	TxPropagationConfig *TxPropagationConfig   `protobuf:"bytes,3,opt,name=tx_propagation_config,json=txPropagationConfig,proto3" json:"tx_propagation_config,omitempty"`
	unknownFields       protoimpl.UnknownFields
	sizeCache           protoimpl.SizeCache
}

func (x *RegisterResponse) Reset() {
	*x = RegisterResponse{}
	mi := &file_relay_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *RegisterResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RegisterResponse) ProtoMessage() {}

func (x *RegisterResponse) ProtoReflect() protoreflect.Message {
	mi := &file_relay_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RegisterResponse.ProtoReflect.Descriptor instead.
func (*RegisterResponse) Descriptor() ([]byte, []int) {
	return file_relay_proto_rawDescGZIP(), []int{1}
}

func (x *RegisterResponse) GetUdpAddress() string {
	if x != nil {
		return x.UdpAddress
	}
	return ""
}

func (x *RegisterResponse) GetJwtToken() string {
	if x != nil {
		return x.JwtToken
	}
	return ""
}

func (x *RegisterResponse) GetTxPropagationConfig() *TxPropagationConfig {
	if x != nil {
		return x.TxPropagationConfig
	}
	return nil
}

type RefreshTokenRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	AuthHeader    string                 `protobuf:"bytes,1,opt,name=auth_header,json=authHeader,proto3" json:"auth_header,omitempty"`
	JwtToken      string                 `protobuf:"bytes,2,opt,name=jwt_token,json=jwtToken,proto3" json:"jwt_token,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *RefreshTokenRequest) Reset() {
	*x = RefreshTokenRequest{}
	mi := &file_relay_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *RefreshTokenRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RefreshTokenRequest) ProtoMessage() {}

func (x *RefreshTokenRequest) ProtoReflect() protoreflect.Message {
	mi := &file_relay_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RefreshTokenRequest.ProtoReflect.Descriptor instead.
func (*RefreshTokenRequest) Descriptor() ([]byte, []int) {
	return file_relay_proto_rawDescGZIP(), []int{2}
}

func (x *RefreshTokenRequest) GetAuthHeader() string {
	if x != nil {
		return x.AuthHeader
	}
	return ""
}

func (x *RefreshTokenRequest) GetJwtToken() string {
	if x != nil {
		return x.JwtToken
	}
	return ""
}

type RefreshTokenResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	JwtToken      string                 `protobuf:"bytes,1,opt,name=jwt_token,json=jwtToken,proto3" json:"jwt_token,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *RefreshTokenResponse) Reset() {
	*x = RefreshTokenResponse{}
	mi := &file_relay_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *RefreshTokenResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RefreshTokenResponse) ProtoMessage() {}

func (x *RefreshTokenResponse) ProtoReflect() protoreflect.Message {
	mi := &file_relay_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RefreshTokenResponse.ProtoReflect.Descriptor instead.
func (*RefreshTokenResponse) Descriptor() ([]byte, []int) {
	return file_relay_proto_rawDescGZIP(), []int{3}
}

func (x *RefreshTokenResponse) GetJwtToken() string {
	if x != nil {
		return x.JwtToken
	}
	return ""
}

var File_relay_proto protoreflect.FileDescriptor

const file_relay_proto_rawDesc = "" +
	"\n" +
	"\vrelay.proto\x12\x05relay\x1a\vtypes.proto\"\xaa\x01\n" +
	"\x0fRegisterRequest\x12\x1f\n" +
	"\vauth_header\x18\x01 \x01(\tR\n" +
	"authHeader\x12\x1c\n" +
	"\aversion\x18\x02 \x01(\tB\x02\x18\x01R\aversion\x12#\n" +
	"\vserver_port\x18\x03 \x01(\x03B\x02\x18\x01R\n" +
	"serverPort\x123\n" +
	"\x15gateway_configuration\x18\x04 \x01(\fR\x14gatewayConfiguration\"\xa0\x01\n" +
	"\x10RegisterResponse\x12\x1f\n" +
	"\vudp_address\x18\x01 \x01(\tR\n" +
	"udpAddress\x12\x1b\n" +
	"\tjwt_token\x18\x02 \x01(\tR\bjwtToken\x12N\n" +
	"\x15tx_propagation_config\x18\x03 \x01(\v2\x1a.types.TxPropagationConfigR\x13txPropagationConfig\"S\n" +
	"\x13RefreshTokenRequest\x12\x1f\n" +
	"\vauth_header\x18\x01 \x01(\tR\n" +
	"authHeader\x12\x1b\n" +
	"\tjwt_token\x18\x02 \x01(\tR\bjwtToken\"3\n" +
	"\x14RefreshTokenResponse\x12\x1b\n" +
	"\tjwt_token\x18\x01 \x01(\tR\bjwtToken2\x8d\x01\n" +
	"\x05Relay\x12;\n" +
	"\bRegister\x12\x16.relay.RegisterRequest\x1a\x17.relay.RegisterResponse\x12G\n" +
	"\fRefreshToken\x12\x1a.relay.RefreshTokenRequest\x1a\x1b.relay.RefreshTokenResponseB7Z5github.com/bloXroute-Labs/solana-gateway/pkg/protobufb\x06proto3"

var (
	file_relay_proto_rawDescOnce sync.Once
	file_relay_proto_rawDescData []byte
)

func file_relay_proto_rawDescGZIP() []byte {
	file_relay_proto_rawDescOnce.Do(func() {
		file_relay_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_relay_proto_rawDesc), len(file_relay_proto_rawDesc)))
	})
	return file_relay_proto_rawDescData
}

var file_relay_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_relay_proto_goTypes = []any{
	(*RegisterRequest)(nil),      // 0: relay.RegisterRequest
	(*RegisterResponse)(nil),     // 1: relay.RegisterResponse
	(*RefreshTokenRequest)(nil),  // 2: relay.RefreshTokenRequest
	(*RefreshTokenResponse)(nil), // 3: relay.RefreshTokenResponse
	(*TxPropagationConfig)(nil),  // 4: types.TxPropagationConfig
}
var file_relay_proto_depIdxs = []int32{
	4, // 0: relay.RegisterResponse.tx_propagation_config:type_name -> types.TxPropagationConfig
	0, // 1: relay.Relay.Register:input_type -> relay.RegisterRequest
	2, // 2: relay.Relay.RefreshToken:input_type -> relay.RefreshTokenRequest
	1, // 3: relay.Relay.Register:output_type -> relay.RegisterResponse
	3, // 4: relay.Relay.RefreshToken:output_type -> relay.RefreshTokenResponse
	3, // [3:5] is the sub-list for method output_type
	1, // [1:3] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_relay_proto_init() }
func file_relay_proto_init() {
	if File_relay_proto != nil {
		return
	}
	file_types_proto_init()
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_relay_proto_rawDesc), len(file_relay_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_relay_proto_goTypes,
		DependencyIndexes: file_relay_proto_depIdxs,
		MessageInfos:      file_relay_proto_msgTypes,
	}.Build()
	File_relay_proto = out.File
	file_relay_proto_goTypes = nil
	file_relay_proto_depIdxs = nil
}
