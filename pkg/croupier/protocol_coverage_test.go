// Copyright 2025 Croupier Authors
// Licensed under the Apache License, Version 2.0

//go:build integration
// +build integration

package croupier

import (
	"testing"

	"go.nanomsg.org/mangos/v3"
	"github.com/cuihairu/croupier/sdks/go/pkg/croupier/protocol"
)

// TestProtocol_messageOperations tests protocol message operations
func TestProtocol_messageOperations(t *testing.T) {
	t.Run("message ID operations", func(t *testing.T) {
		t.Run("PutMsgID and GetMsgID with request", func(t *testing.T) {
			msg, _ := protocol.NewRequestMessage(1, 2, nil)

			protocol.PutMsgID(msg.Header, 12345)
			msgID := protocol.GetMsgID(msg.Header)

			if msgID != 12345 {
				t.Errorf("GetMsgID = %d, want 12345", msgID)
			}
		})

		t.Run("PutMsgID with different values", func(t *testing.T) {
			testIDs := []uint32{0, 1, 100, 1000, 65535, 4294967295}

			for _, testID := range testIDs {
				msg, _ := protocol.NewRequestMessage(testID, testID+1, nil)
				protocol.PutMsgID(msg.Header, testID)

				retrievedID := protocol.GetMsgID(msg.Header)
				if retrievedID != testID {
					t.Errorf("PutMsgID(%d) -> GetMsgID = %d, want %d", testID, retrievedID, testID)
				}
			}
		})
	})

	t.Run("message type checking", func(t *testing.T) {
		t.Run("IsRequest with request message", func(t *testing.T) {
			msg, _ := protocol.NewRequestMessage(1, 2, nil)

			if !protocol.IsRequest(protocol.GetMsgID(msg.Header)) {
				t.Error("NewRequestMessage should be identified as request")
			}
		})

		t.Run("IsResponse with response message", func(t *testing.T) {
			msg, _ := protocol.NewResponseMessage(1, 2, nil)

			if !protocol.IsResponse(protocol.GetMsgID(msg.Header)) {
				t.Error("NewResponseMessage should be identified as response")
			}
		})

		t.Run("IsRequest with response message", func(t *testing.T) {
			msg, _ := protocol.NewResponseMessage(1, 2, nil)

			if protocol.IsRequest(protocol.GetMsgID(msg.Header)) {
				t.Error("NewResponseMessage should not be identified as request")
			}
		})

		t.Run("IsResponse with request message", func(t *testing.T) {
			msg, _ := protocol.NewRequestMessage(1, 2, nil)

			if protocol.IsResponse(protocol.GetMsgID(msg.Header)) {
				t.Error("NewRequestMessage should not be identified as response")
			}
		})
	})

	t.Run("response message ID", func(t *testing.T) {
		t.Run("GetResponseMsgID", func(t *testing.T) {
			requestID := uint32(9999)

			reqMsg, _ := protocol.NewRequestMessage(1, 2, nil)
			protocol.PutMsgID(reqMsg.Header, requestID)

			respMsg, _ := protocol.NewResponseMessage(1, 2, nil)
			protocol.PutMsgID(respMsg.Header, requestID)

			responseID := protocol.GetResponseMsgID(protocol.GetMsgID(respMsg.Header))
			if responseID != requestID {
				t.Errorf("GetResponseMsgID = %d, want %d", responseID, requestID)
			}
		})
	})

	t.Run("message ID string", func(t *testing.T) {
		t.Run("MsgIDString with valid message", func(t *testing.T) {
			msg, _ := protocol.NewRequestMessage(1, 2, nil)
			protocol.PutMsgID(msg.Header, 42)

			idStr := protocol.MsgIDString(protocol.GetMsgID(msg.Header))
			if idStr == "" {
				t.Error("MsgIDString should not be empty")
			}

			t.Logf("MsgIDString: %s", idStr)
		})

		t.Run("MsgIDString with different IDs", func(t *testing.T) {
			testIDs := []uint32{0, 1, 42, 100, 1000}

			for _, testID := range testIDs {
				msg, _ := protocol.NewRequestMessage(1, 2, nil)
				protocol.PutMsgID(msg.Header, testID)

				idStr := protocol.MsgIDString(protocol.GetMsgID(msg.Header))
				t.Logf("MsgIDString(%d) = %s", testID, idStr)
			}
		})
	})
}

// TestProtocol_messageParsing tests protocol message parsing
func TestProtocol_messageParsing(t *testing.T) {
	t.Run("ParseMessage with invalid inputs", func(t *testing.T) {
		t.Run("ParseMessage with nil message", func(t *testing.T) {
			_, _, _, _, err := protocol.ParseMessage(nil)

			if err == nil {
				t.Error("ParseMessage with nil should return error")
			}

			t.Logf("ParseMessage(nil) error: %v", err)
		})

		t.Run("ParseMessage with message with empty header", func(t *testing.T) {
			msg := mangos.NewMessage(0)

			_, _, _, _, err := protocol.ParseMessage(msg)

			if err == nil {
				t.Error("ParseMessage with empty header should return error")
			}

			t.Logf("ParseMessage(empty header) error: %v", err)
		})

		t.Run("ParseMessage with message with short header", func(t *testing.T) {
			msg := mangos.NewMessage(0)
			msg.Header = []byte{0x01, 0x02}

			version, msgID, reqID, body, err := protocol.ParseMessage(msg)
			_ = version
			_ = msgID
			_ = reqID
			_ = body

			t.Logf("ParseMessage(short header) error: %v", err)
		})
	})

	t.Run("ParseMessageFromBody with invalid inputs", func(t *testing.T) {
		t.Run("ParseMessageFromBody with nil data", func(t *testing.T) {
			_, _, _, _, err := protocol.ParseMessageFromBody(nil)

			if err == nil {
				t.Error("ParseMessageFromBody with nil should return error")
			}

			t.Logf("ParseMessageFromBody(nil) error: %v", err)
		})

		t.Run("ParseMessageFromBody with empty data", func(t *testing.T) {
			_, _, _, _, err := protocol.ParseMessageFromBody([]byte{})

			if err == nil {
				t.Error("ParseMessageFromBody with empty should return error")
			}

			t.Logf("ParseMessageFromBody([]) error: %v", err)
		})
	})
}

// TestProtocol_debugFunctions tests protocol debug functions
func TestProtocol_debugFunctions(t *testing.T) {
	t.Run("DebugString with various messages", func(t *testing.T) {
		t.Run("DebugString with nil message", func(t *testing.T) {
			str := protocol.DebugString(nil)
			t.Logf("DebugString(nil): %s", str)
		})

		t.Run("DebugString with request message", func(t *testing.T) {
			msg, _ := protocol.NewRequestMessage(1, 2, nil)
			str := protocol.DebugString(msg)
			t.Logf("DebugString(request): %s", str)
		})

		t.Run("DebugString with response message", func(t *testing.T) {
			msg, _ := protocol.NewResponseMessage(1, 2, nil)
			str := protocol.DebugString(msg)
			t.Logf("DebugString(response): %s", str)
		})
	})

	t.Run("DebugStringWithBody with various messages", func(t *testing.T) {
		t.Run("DebugStringWithBody with nil message", func(t *testing.T) {
			str := protocol.DebugStringWithBody(nil, nil)
			t.Logf("DebugStringWithBody(nil, nil): %s", str)
		})

		t.Run("DebugStringWithBody with request message", func(t *testing.T) {
			msg, _ := protocol.NewRequestMessage(1, 2, nil)
			str := protocol.DebugStringWithBody(msg, nil)
			t.Logf("DebugStringWithBody(request, nil): %s", str)
		})

		t.Run("DebugStringWithBody with response and body", func(t *testing.T) {
			msg, _ := protocol.NewResponseMessage(1, 2, nil)
			str := protocol.DebugStringWithBody(msg, nil)
			t.Logf("DebugStringWithBody(response, nil): %s", str)
		})
	})

	t.Run("FormatHeader with various messages", func(t *testing.T) {
		t.Run("FormatHeader with nil message", func(t *testing.T) {
			str := protocol.FormatHeader(nil)
			t.Logf("FormatHeader(nil): %s", str)
		})

		t.Run("FormatHeader with valid message", func(t *testing.T) {
			msg, _ := protocol.NewRequestMessage(1, 2, nil)
			str := protocol.FormatHeader(msg.Header)
			t.Logf("FormatHeader(valid): %s", str)
		})
	})

	t.Run("ParseMessageInfo with various inputs", func(t *testing.T) {
		t.Run("ParseMessageInfo with nil message", func(t *testing.T) {
			info, err := protocol.ParseMessageInfo(nil)
			t.Logf("ParseMessageInfo(nil): %+v, error: %v", info, err)
		})

		t.Run("ParseMessageInfo with empty message", func(t *testing.T) {
			msg := mangos.NewMessage(0)
			info, err := protocol.ParseMessageInfo(msg)
			t.Logf("ParseMessageInfo(empty): %+v, error: %v", info, err)
		})

		t.Run("ParseMessageInfo with short header", func(t *testing.T) {
			msg := mangos.NewMessage(0)
			msg.Header = []byte{0x01, 0x02}
			info, err := protocol.ParseMessageInfo(msg)
			t.Logf("ParseMessageInfo(short): %+v, error: %v", info, err)
		})
	})

	t.Run("MessageInfo String method", func(t *testing.T) {
		info := &protocol.MessageInfo{}
		str := info.String()
		t.Logf("MessageInfo.String(): %s", str)
	})
}

// TestProtocol_messageBodyOperations tests message body operations
func TestProtocol_messageBodyOperations(t *testing.T) {
	t.Run("NewMessageBody with various inputs", func(t *testing.T) {
		t.Run("NewMessageBody with valid params", func(t *testing.T) {
			body := protocol.NewMessageBody(1, 2, []byte("test"))
			if body == nil {
				t.Error("NewMessageBody should not return nil")
			}
			t.Logf("NewMessageBody: %+v", body)
		})

		t.Run("NewMessageBody with nil data", func(t *testing.T) {
			body := protocol.NewMessageBody(1, 2, nil)
			if body == nil {
				t.Error("NewMessageBody with nil data should not return nil")
			}
			t.Logf("NewMessageBody(nil data): %+v", body)
		})

		t.Run("NewMessageBody with empty data", func(t *testing.T) {
			body := protocol.NewMessageBody(1, 2, []byte{})
			if body == nil {
				t.Error("NewMessageBody with empty data should not return nil")
			}
			t.Logf("NewMessageBody(empty data): %+v", body)
		})
	})
}

// TestProtocol_streamMessage tests stream message operations
func TestProtocol_streamMessage(t *testing.T) {
	t.Run("NewStreamMessage", func(t *testing.T) {
		msg, _ := protocol.NewStreamMessage(1, 2, nil)
		if msg == nil {
			t.Error("NewStreamMessage should not return nil")
		}
		t.Logf("NewStreamMessage: %+v", msg)
	})
}

// TestProtocol_messageCombinations tests message combinations
func TestProtocol_messageCombinations(t *testing.T) {
	t.Run("multiple request messages with same ID", func(t *testing.T) {
		msgID := uint32(42)

		for i := 0; i < 5; i++ {
			msg, _ := protocol.NewRequestMessage(uint32(i), uint32(i+1), nil)
			protocol.PutMsgID(msg.Header, msgID)

			retrievedID := protocol.GetMsgID(msg.Header)
			if retrievedID != msgID {
				t.Errorf("Iteration %d: GetMsgID = %d, want %d", i, retrievedID, msgID)
			}

			if !protocol.IsRequest(protocol.GetMsgID(msg.Header)) {
				t.Errorf("Iteration %d: message should be identified as request", i)
			}
		}
	})

	t.Run("request-response message pairs", func(t *testing.T) {
		pairs := []struct{ reqID, respID uint32 }{
			{1, 1},
			{100, 100},
			{1000, 1000},
			{65535, 65535},
		}

		for i, pair := range pairs {
			reqMsg, _ := protocol.NewRequestMessage(1, 2, nil)
			protocol.PutMsgID(reqMsg.Header, pair.reqID)

			respMsg, _ := protocol.NewResponseMessage(1, 2, nil)
			protocol.PutMsgID(respMsg.Header, pair.respID)

			if !protocol.IsRequest(protocol.GetMsgID(reqMsg.Header)) {
				t.Errorf("Pair %d: request not identified", i)
			}

			if !protocol.IsResponse(protocol.GetMsgID(respMsg.Header)) {
				t.Errorf("Pair %d: response not identified", i)
			}

			responseID := protocol.GetResponseMsgID(protocol.GetMsgID(respMsg.Header))
			if responseID != pair.respID {
				t.Errorf("Pair %d: GetResponseMsgID = %d, want %d", i, responseID, pair.respID)
			}
		}
	})
}
