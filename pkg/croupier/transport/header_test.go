// Package transport provides tests for the NNG transport layer.
package transport

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/rep"
	_ "go.nanomsg.org/mangos/v3/transport/tcp"
	"go.nanomsg.org/mangos/v3/protocol/req"
)

// TestMangos_RepProtocol_HeaderPreservation 测试 REP 协议是否保留 Header
func TestMangos_RepProtocol_HeaderPreservation(t *testing.T) {
	addr := "tcp://127.0.0.1:19007"

	// 服务器
	serverSock, err := rep.NewSocket()
	require.NoError(t, err)
	defer serverSock.Close()

	err = serverSock.Listen(addr)
	require.NoError(t, err)

	receivedHeader := make([]byte, 0, 8)
	serverReady := make(chan struct{})
	go func() {
		close(serverReady)
		msg, err := serverSock.RecvMsg()
		require.NoError(t, err)

		// 记录接收到的 Header
		t.Logf("Server received Header length: %d", len(msg.Header))
		t.Logf("Server received Header: %v", msg.Header)
		t.Logf("Server received Body: %s", string(msg.Body))

		if len(msg.Header) > 0 {
			receivedHeader = append(receivedHeader, msg.Header...)
		}

		// 回显
		resp := mangos.NewMessage(0)
		resp.Body = []byte("ok")
		serverSock.SendMsg(resp)
		msg.Free()
	}()

	<-serverReady
	time.Sleep(100 * time.Millisecond)

	// 客户端
	clientSock, err := req.NewSocket()
	require.NoError(t, err)
	defer clientSock.Close()

	err = clientSock.Dial(addr)
	require.NoError(t, err)

	// 发送带 8 字节 Header 的消息
	msg := mangos.NewMessage(0)
	msg.Header = make([]byte, 8)
	msg.Header[0] = 0x01
	msg.Header[1] = 0x02
	msg.Header[2] = 0x03
	msg.Header[3] = 0x04
	msg.Header[4] = 0x05
	msg.Header[5] = 0x06
	msg.Header[6] = 0x07
	msg.Header[7] = 0x08
	msg.Body = []byte("test")

	err = clientSock.SendMsg(msg)
	require.NoError(t, err)

	// 等待服务器处理
	time.Sleep(100 * time.Millisecond)

	// 接收响应
	resp, err := clientSock.RecvMsg()
	require.NoError(t, err)
	defer resp.Free()

	// 验证
	t.Logf("Received header length: %d", len(receivedHeader))

	if len(receivedHeader) == 0 {
		t.Log("WARNING: REP protocol does NOT preserve custom Header!")
	}
}
