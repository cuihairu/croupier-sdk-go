package protocol

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.nanomsg.org/mangos/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestPutMsgIDAndGetMsgID(t *testing.T) {
	tests := []struct {
		name  string
		msgID uint32
	}{
		{"Min value", 0x000000},
		{"Max value", 0xFFFFFF},
		{"RegisterRequest", MsgRegisterRequest},
		{"InvokeRequest", MsgInvokeRequest},
		{"InvokeResponse", MsgInvokeResponse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, 3)
			PutMsgID(buf, tt.msgID)
			got := GetMsgID(buf)
			assert.Equal(t, tt.msgID, got)
		})
	}
}

func TestNewRequestMessage(t *testing.T) {
	body := &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "test"}}

	msg, err := NewRequestMessage(MsgInvokeRequest, 12345, body)
	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.NotNil(t, msg.Header)
	assert.NotNil(t, msg.Body)
	assert.Equal(t, HeaderSize, len(msg.Header))

	// Verify header
	assert.Equal(t, byte(Version1), msg.Header[0])
	assert.Equal(t, uint32(MsgInvokeRequest), GetMsgID(msg.Header[1:4]))
	assert.Equal(t, uint32(12345), binary.BigEndian.Uint32(msg.Header[4:8]))

	msg.Free()
}

func TestNewResponseMessage(t *testing.T) {
	body := &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 42.0}}

	msg, err := NewResponseMessage(MsgInvokeResponse, 12345, body)
	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.NotNil(t, msg.Header)
	assert.NotNil(t, msg.Body)
	assert.Equal(t, HeaderSize, len(msg.Header))

	// Verify header
	assert.Equal(t, byte(Version1), msg.Header[0])
	assert.Equal(t, uint32(MsgInvokeResponse), GetMsgID(msg.Header[1:4]))
	assert.Equal(t, uint32(12345), binary.BigEndian.Uint32(msg.Header[4:8]))

	msg.Free()
}

func TestParseMessage(t *testing.T) {
	body := &structpb.Value{Kind: &structpb.Value_BoolValue{BoolValue: true}}

	msg, err := NewRequestMessage(MsgInvokeRequest, 99999, body)
	assert.NoError(t, err)
	defer msg.Free()

	version, msgID, reqID, bodyBytes, err := ParseMessage(msg)
	assert.NoError(t, err)
	assert.Equal(t, uint8(Version1), version)
	assert.Equal(t, uint32(MsgInvokeRequest), msgID)
	assert.Equal(t, uint32(99999), reqID)
	assert.NotNil(t, bodyBytes)
}

func TestParseMessageInvalid(t *testing.T) {
	msg := mangos.NewMessage(0)
	msg.Header = make([]byte, 4) // Too short

	_, _, _, _, err := ParseMessage(msg)
	assert.Error(t, err)
	msg.Free()
}

func TestIsRequest(t *testing.T) {
	tests := []struct {
		msgID uint32
		isReq bool
	}{
		{MsgRegisterRequest, true},
		{MsgInvokeRequest, true},
		{MsgInvokeResponse, false},
		{MsgJobEvent, false}, // Stream event, not a request
	}

	for _, tt := range tests {
		t.Run(MsgIDString(tt.msgID), func(t *testing.T) {
			assert.Equal(t, tt.isReq, IsRequest(tt.msgID))
		})
	}
}

func TestIsResponse(t *testing.T) {
	tests := []struct {
		msgID  uint32
		isResp bool
	}{
		{MsgRegisterRequest, false},
		{MsgRegisterResponse, true},
		{MsgInvokeResponse, true},
		{MsgJobEvent, false}, // Stream event, not a response
	}

	for _, tt := range tests {
		t.Run(MsgIDString(tt.msgID), func(t *testing.T) {
			assert.Equal(t, tt.isResp, IsResponse(tt.msgID))
		})
	}
}

func TestGetResponseMsgID(t *testing.T) {
	tests := []struct {
		reqMsgID  uint32
		respMsgID uint32
	}{
		{MsgRegisterRequest, MsgRegisterResponse},
		{MsgInvokeRequest, MsgInvokeResponse},
		{MsgStartJobRequest, MsgStartJobResponse},
	}

	for _, tt := range tests {
		t.Run(MsgIDString(tt.reqMsgID), func(t *testing.T) {
			assert.Equal(t, tt.respMsgID, GetResponseMsgID(tt.reqMsgID))
		})
	}
}

func TestMsgIDString(t *testing.T) {
	tests := []struct {
		msgID     uint32
		wantPanic bool
	}{
		{MsgRegisterRequest, false},
		{MsgInvokeRequest, false},
		{MsgJobEvent, false},
		{0xFFFFFF, false}, // Unknown
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := MsgIDString(tt.msgID)
			assert.NotEmpty(t, got)
		})
	}
}

func TestDebugString(t *testing.T) {
	body := &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "test"}}

	msg, err := NewRequestMessage(MsgInvokeRequest, 12345, body)
	assert.NoError(t, err)
	defer msg.Free()

	str := DebugString(msg)
	assert.Contains(t, str, "Ver=1")
	assert.Contains(t, str, "InvokeRequest")
	assert.Contains(t, str, "ReqID=12345")
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()

	factory := func() proto.Message {
		return &structpb.Value{}
	}

	reg.Register(MsgInvokeRequest, factory)

	// Test Create
	msg, err := reg.Create(MsgInvokeRequest)
	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.IsType(t, &structpb.Value{}, msg)

	// Test Create unknown
	_, err = reg.Create(0x999999)
	assert.Error(t, err)

	// Test Unmarshal
	bodyValue := &structpb.Value{Kind: &structpb.Value_BoolValue{BoolValue: true}}
	bodyBytes, _ := proto.Marshal(bodyValue)

	parsed, err := reg.Unmarshal(MsgInvokeRequest, bodyBytes)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)

	// Test Unmarshal unknown
	_, err = reg.Unmarshal(0x999999, bodyBytes)
	assert.Error(t, err)
}

func TestRegistryMustRegister(t *testing.T) {
	reg := NewRegistry()

	factory := func() proto.Message {
		return &structpb.Value{}
	}

	assert.NotPanics(t, func() {
		reg.MustRegister(MsgInvokeRequest, factory)
	})

	// Duplicate registration should panic
	assert.Panics(t, func() {
		reg.MustRegister(MsgInvokeRequest, factory)
	})
}

func TestDebugStringWithBody(t *testing.T) {
	t.Run("with nil message", func(t *testing.T) {
		str := DebugStringWithBody(nil, nil)
		assert.Equal(t, "<nil>", str)
	})

	t.Run("with valid message and body", func(t *testing.T) {
		body := &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "test"}}
		msg, err := NewRequestMessage(MsgInvokeRequest, 12345, body)
		assert.NoError(t, err)
		defer msg.Free()

		str := DebugStringWithBody(msg, body)
		assert.Contains(t, str, "Ver=1")
		assert.Contains(t, str, "InvokeRequest")
	})

	t.Run("with valid message but nil body", func(t *testing.T) {
		body := &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "test"}}
		msg, err := NewRequestMessage(MsgInvokeRequest, 12345, body)
		assert.NoError(t, err)
		defer msg.Free()

		str := DebugStringWithBody(msg, nil)
		assert.Contains(t, str, "Ver=1")
		// Should show hex dump of body
	})
}

func TestFormatHeader(t *testing.T) {
	t.Run("with valid header", func(t *testing.T) {
		body := &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "test"}}
		msg, err := NewRequestMessage(MsgInvokeRequest, 12345, body)
		assert.NoError(t, err)
		defer msg.Free()

		str := FormatHeader(msg.Header)
		assert.Contains(t, str, "Ver=1")
		assert.Contains(t, str, "InvokeRequest")
		assert.Contains(t, str, "ReqID=12345")
	})

	t.Run("with short header", func(t *testing.T) {
		shortHeader := make([]byte, 4)
		str := FormatHeader(shortHeader)
		assert.Contains(t, str, "invalid header")
	})
}

func TestParseMessageInfo(t *testing.T) {
	t.Run("with valid message", func(t *testing.T) {
		body := &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "test"}}
		msg, err := NewRequestMessage(MsgInvokeRequest, 12345, body)
		assert.NoError(t, err)
		defer msg.Free()

		info, err := ParseMessageInfo(msg)
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, uint8(Version1), info.Version)
		assert.Equal(t, uint32(MsgInvokeRequest), info.MsgID)
		assert.Equal(t, uint32(12345), info.ReqID)
		assert.True(t, info.IsReq)
		assert.False(t, info.IsResp)
	})

	t.Run("with invalid message", func(t *testing.T) {
		msg := mangos.NewMessage(0)
		msg.Header = make([]byte, 4) // Too short

		_, err := ParseMessageInfo(msg)
		assert.Error(t, err)
		msg.Free()
	})
}

func TestMessageInfoString(t *testing.T) {
	info := &MessageInfo{
		Version: 1,
		MsgID:   MsgInvokeRequest,
		ReqID:   12345,
		BodyLen: 100,
		IsReq:   true,
		IsResp:  false,
	}

	str := info.String()
	assert.Contains(t, str, "Ver=1")
	assert.Contains(t, str, "InvokeRequest")
	assert.Contains(t, str, "ReqID=12345")
	assert.Contains(t, str, "BodyLen=100")
	assert.Contains(t, str, "IsReq=true")
	assert.Contains(t, str, "IsResp=false")
}

func TestRegistryRegisterBatch(t *testing.T) {
	reg := NewRegistry()

	factories := map[uint32]func() proto.Message{
		MsgInvokeRequest: func() proto.Message { return &structpb.Value{} },
		MsgInvokeResponse: func() proto.Message { return &structpb.Value{} },
	}

	reg.RegisterBatch(factories)

	// Verify registration
	_, err := reg.Create(MsgInvokeRequest)
	assert.NoError(t, err)

	_, err = reg.Create(MsgInvokeResponse)
	assert.NoError(t, err)
}

func TestParseMessageFromBody(t *testing.T) {
	t.Run("with valid body", func(t *testing.T) {
		// Create a complete message body (header + data)
		data := []byte("test data")
		fullBody := NewMessageBody(MsgInvokeRequest, 12345, data)

		version, msgID, reqID, bodyBytes, err := ParseMessageFromBody(fullBody)
		assert.NoError(t, err)
		assert.Equal(t, uint8(Version1), version)
		assert.Equal(t, uint32(MsgInvokeRequest), msgID)
		assert.Equal(t, uint32(12345), reqID)
		assert.NotNil(t, bodyBytes)
	})

	t.Run("with empty body", func(t *testing.T) {
		_, _, _, _, err := ParseMessageFromBody([]byte{})
		assert.Error(t, err)
	})

	t.Run("with short body", func(t *testing.T) {
		_, _, _, _, err := ParseMessageFromBody([]byte{1, 2, 3})
		assert.Error(t, err)
	})
}

func TestNewStreamMessage(t *testing.T) {
	body := &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "event"}}

	msg, err := NewStreamMessage(MsgJobEvent, 12345, body)
	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.NotNil(t, msg.Header)
	assert.NotNil(t, msg.Body)
	assert.Equal(t, HeaderSize, len(msg.Header))

	// Verify header
	assert.Equal(t, byte(Version1), msg.Header[0])
	assert.Equal(t, uint32(MsgJobEvent), GetMsgID(msg.Header[1:4]))
	assert.Equal(t, uint32(12345), binary.BigEndian.Uint32(msg.Header[4:8]))

	msg.Free()
}

func TestNewMessageBody(t *testing.T) {
	t.Run("with data", func(t *testing.T) {
		data := []byte("test data")
		bodyBytes := NewMessageBody(MsgInvokeRequest, 12345, data)
		assert.NotNil(t, bodyBytes)
		assert.True(t, len(bodyBytes) >= HeaderSize)
		assert.Equal(t, uint8(Version1), bodyBytes[0])
	})

	t.Run("with empty data", func(t *testing.T) {
		bodyBytes := NewMessageBody(MsgInvokeRequest, 12345, []byte{})
		assert.NotNil(t, bodyBytes)
		assert.Equal(t, HeaderSize, len(bodyBytes))
	})
}
