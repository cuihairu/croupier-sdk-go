// Package protocol provides debugging utilities for Croupier protocol.
package protocol

import (
	"encoding/binary"
	"fmt"

	"go.nanomsg.org/mangos/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// DebugString returns a human-readable string representation of the message.
// It attempts to parse the body as protobuf and format it as JSON if possible.
func DebugString(msg *mangos.Message) string {
	if msg == nil {
		return "<nil>"
	}

	version, msgID, reqID, body, err := ParseMessage(msg)
	if err != nil {
		return fmt.Sprintf("Message[ParseError: %v]", err)
	}

	return fmt.Sprintf("Message{Ver=%d, MsgID=%s(0x%06X), ReqID=%d, BodyLen=%d}",
		version, MsgIDString(msgID), msgID, reqID, len(body))
}

// DebugStringWithBody returns a detailed string representation including the body.
// The body is formatted as JSON using protojson.
func DebugStringWithBody(msg *mangos.Message, bodyMsg proto.Message) string {
	if msg == nil {
		return "<nil>"
	}

	version, msgID, reqID, body, err := ParseMessage(msg)
	if err != nil {
		return fmt.Sprintf("Message[ParseError: %v]", err)
	}

	bodyStr := "<body>"
	if bodyMsg != nil {
		if jsonBytes, err := protojson.Marshal(bodyMsg); err == nil {
			bodyStr = string(jsonBytes)
		} else {
			bodyStr = fmt.Sprintf("<json error: %v>", err)
		}
	} else if len(body) > 0 {
		// Try to show hex dump for unknown body types
		if len(body) <= 32 {
			bodyStr = fmt.Sprintf("%x", body)
		} else {
			bodyStr = fmt.Sprintf("%x... (%d bytes)", body[:32], len(body))
		}
	}

	return fmt.Sprintf("Message{Ver=%d, MsgID=%s(0x%06X), ReqID=%d, Body=%s}",
		version, MsgIDString(msgID), msgID, reqID, bodyStr)
}

// FormatHeader returns a formatted string representation of just the header.
func FormatHeader(header []byte) string {
	if len(header) < HeaderSize {
		return fmt.Sprintf("<invalid header: %d bytes>", len(header))
	}

	version := header[0]
	msgID := GetMsgID(header[1:4])
	reqID := binary.BigEndian.Uint32(header[4:8])

	return fmt.Sprintf("Header{Ver=%d, MsgID=%s(0x%06X), ReqID=%d}",
		version, MsgIDString(msgID), msgID, reqID)
}

// MessageInfo holds parsed message information.
type MessageInfo struct {
	Version uint8
	MsgID   uint32
	ReqID   uint32
	BodyLen int
	IsReq   bool
	IsResp  bool
}

// ParseMessageInfo parses the message and returns structured information.
func ParseMessageInfo(msg *mangos.Message) (*MessageInfo, error) {
	info := &MessageInfo{}

	version, msgID, reqID, body, err := ParseMessage(msg)
	if err != nil {
		return nil, err
	}

	info.Version = version
	info.MsgID = msgID
	info.ReqID = reqID
	info.BodyLen = len(body)
	info.IsReq = IsRequest(msgID)
	info.IsResp = IsResponse(msgID)

	return info, nil
}

// String returns a string representation of MessageInfo.
func (i *MessageInfo) String() string {
	return fmt.Sprintf("MessageInfo{Ver=%d, MsgID=%s(0x%06X), ReqID=%d, BodyLen=%d, IsReq=%v, IsResp=%v}",
		i.Version, MsgIDString(i.MsgID), i.MsgID, i.ReqID, i.BodyLen, i.IsReq, i.IsResp)
}
