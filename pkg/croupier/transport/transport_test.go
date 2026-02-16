// Package transport provides tests for the NNG transport layer.
package transport

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/rep"
	"go.nanomsg.org/mangos/v3/protocol/req"
	_ "go.nanomsg.org/mangos/v3/transport/tcp"

	"github.com/cuihairu/croupier/sdks/go/pkg/croupier/protocol"
)

// ============================================
// 基础协议测试
// ============================================

func TestProtocol_HeaderSize(t *testing.T) {
	assert.Equal(t, 8, protocol.HeaderSize)
}

func TestProtocol_PutMsgID_GetMsgID(t *testing.T) {
	buf := make([]byte, 3)
	protocol.PutMsgID(buf, 0x030101)
	result := protocol.GetMsgID(buf)
	assert.Equal(t, uint32(0x030101), result)
}

func TestProtocol_PutMsgID_GetMsgID_Values(t *testing.T) {
	tests := []struct {
		name  string
		input uint32
	}{
		{"InvokeRequest", 0x030101},
		{"InvokeResponse", 0x030102},
		{"MaxValue", 0xFFFFFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, 3)
			protocol.PutMsgID(buf, tt.input)
			result := protocol.GetMsgID(buf)
			assert.Equal(t, tt.input, result)
		})
	}
}

func TestProtocol_ParseMessage_Empty(t *testing.T) {
	msg := mangos.NewMessage(0)
	msg.Header = []byte{}
	defer msg.Free()

	_, _, _, _, err := protocol.ParseMessage(msg)
	assert.Error(t, err)
}

func TestProtocol_ParseMessage_TooShort(t *testing.T) {
	msg := mangos.NewMessage(0)
	msg.Header = []byte{1, 2, 3, 4} // only 4 bytes
	defer msg.Free()

	_, _, _, _, err := protocol.ParseMessage(msg)
	assert.Error(t, err)
}

func TestProtocol_ParseMessage_Valid(t *testing.T) {
	msg := mangos.NewMessage(0)
	msg.Header = make([]byte, 8)
	msg.Header[0] = 0x01
	protocol.PutMsgID(msg.Header[1:4], 0x030101)
	msg.Header[4] = 0x00
	msg.Header[5] = 0x00
	msg.Header[6] = 0x00
	msg.Header[7] = 0x01
	msg.Body = []byte(`{"test": "data"}`)
	defer msg.Free()

	version, msgID, reqID, body, err := protocol.ParseMessage(msg)
	require.NoError(t, err)
	assert.Equal(t, uint8(0x01), version)
	assert.Equal(t, uint32(0x030101), msgID)
	assert.Equal(t, uint32(1), reqID)
	assert.Equal(t, []byte(`{"test": "data"}`), body)
}

// ============================================
// Handler 接口测试
// ============================================

type mockHandler struct {
	called bool
	msgID  uint32
	reqID  uint32
	body   []byte
}

func (h *mockHandler) Handle(ctx context.Context, msgID uint32, reqID uint32, body []byte) ([]byte, error) {
	h.called = true
	h.msgID = msgID
	h.reqID = reqID
	h.body = body
	return []byte(`{"result": "ok"}`), nil
}

func TestHandler_Interface(t *testing.T) {
	handler := &mockHandler{}
	// 实现 Handler 接口
	var _ Handler = handler
	_ = HandlerFunc(handler.Handle)
}

func TestHandler_Called(t *testing.T) {
	handler := &mockHandler{}
	ctx := context.Background()
	resp, err := handler.Handle(ctx, 0x030101, 123, []byte(`{"test": "data"}`))
	require.NoError(t, err)
	assert.True(t, handler.called)
	assert.Equal(t, uint32(0x030101), handler.msgID)
	assert.Equal(t, uint32(123), handler.reqID)
	assert.Equal(t, []byte(`{"test": "data"}`), handler.body)
	assert.Equal(t, []byte(`{"result": "ok"}`), resp)
}

// ============================================
// Config 测试
// ============================================

func TestConfig_Default(t *testing.T) {
	cfg := DefaultConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, "127.0.0.1:19090", cfg.Address)
	assert.True(t, cfg.Insecure)
	assert.Equal(t, 128, cfg.ReadQLen)
	assert.Equal(t, 64, cfg.WriteQLen)
}

func TestConfig_DialAddr_IP(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		insecure bool
		expected string
	}{
		{"IP address", "192.168.1.1:8080", true, "tcp://192.168.1.1:8080"},
		{"IP address TLS", "192.168.1.1:8080", false, "tls+tcp://192.168.1.1:8080"},
		{"With scheme", "tcp://192.168.1.1:8080", true, "tcp://192.168.1.1:8080"},
		{"With TLS scheme", "tls+tcp://192.168.1.1:8080", false, "tls+tcp://192.168.1.1:8080"},
		{"inproc", "inproc://test", true, "inproc://test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Address:  tt.address,
				Insecure: tt.insecure,
			}
			result := dialAddr(cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfig_ListenAddr_IP(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		insecure bool
		expected string
	}{
		{"IP address", "0.0.0.0:8080", true, "tcp://0.0.0.0:8080"},
		{"IP address TLS", "0.0.0.0:8080", false, "tls+tcp://0.0.0.0:8080"},
		{"With scheme", "tcp://0.0.0.0:8080", true, "tcp://0.0.0.0:8080"},
		{"inproc", "inproc://test", true, "inproc://test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Address:  tt.address,
				Insecure: tt.insecure,
			}
			result := listenAddrServer(cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================
// 原生 mangos API 测试（基线验证）
// ============================================

func TestMangos_ReqRep_TCP_Echo(t *testing.T) {
	addr := "tcp://127.0.0.1:19001"

	// 服务器
	serverSock, err := rep.NewSocket()
	require.NoError(t, err)
	defer serverSock.Close()

	err = serverSock.Listen(addr)
	require.NoError(t, err)

	serverReady := make(chan struct{})
	go func() {
		close(serverReady)
		msg, err := serverSock.RecvMsg()
		require.NoError(t, err)
		defer msg.Free()
		// 回显
		resp := mangos.NewMessage(0)
		resp.Body = []byte("echo: " + string(msg.Body))
		require.NoError(t, serverSock.SendMsg(resp))
	}()

	<-serverReady
	time.Sleep(100 * time.Millisecond)

	// 客户端
	clientSock, err := req.NewSocket()
	require.NoError(t, err)
	defer clientSock.Close()

	err = clientSock.Dial(addr)
	require.NoError(t, err)

	// 发送
	msg := mangos.NewMessage(0)
	msg.Body = []byte("hello")
	err = clientSock.SendMsg(msg)
	require.NoError(t, err)

	// 接收
	resp, err := clientSock.RecvMsg()
	require.NoError(t, err)
	defer resp.Free()

	assert.Equal(t, []byte("echo: hello"), resp.Body)
}

// TestMangos_ReqRep_TCP_BodyPrefixHeader 测试在 Body 中包含协议头的通信方式
// 这是正确的方式，因为 REP 协议不保留自定义 Header
func TestMangos_ReqRep_TCP_BodyPrefixHeader(t *testing.T) {
	addr := "tcp://127.0.0.1:19002"

	// 服务器
	serverSock, err := rep.NewSocket()
	require.NoError(t, err)
	defer serverSock.Close()

	err = serverSock.Listen(addr)
	require.NoError(t, err)

	serverReady := make(chan struct{})
	go func() {
		close(serverReady)
		msg, err := serverSock.RecvMsg()
		require.NoError(t, err)
		defer msg.Free()

		// 验证：mangos REP 协议不保留自定义 Header
		assert.Equal(t, 0, len(msg.Header), "REP protocol strips custom headers")

		// 协议头应该在 Body 前缀中
		// Body 格式: [8字节协议头][实际数据]
		version, msgID, reqID, data, err := protocol.ParseMessageFromBody(msg.Body)
		require.NoError(t, err)
		assert.EqualValues(t, protocol.Version1, version)
		assert.EqualValues(t, protocol.MsgInvokeRequest, msgID)
		assert.EqualValues(t, 1, reqID)
		assert.Equal(t, []byte(`{"test": "data"}`), data)

		// 回显响应（协议头在 Body 前缀中）
		respBody := []byte(`{"result": "ok"}`)
		respBodyWithHeader := protocol.NewMessageBody(protocol.MsgInvokeResponse, reqID, respBody)

		resp := mangos.NewMessage(0)
		resp.Body = respBodyWithHeader
		require.NoError(t, serverSock.SendMsg(resp))
	}()

	<-serverReady
	time.Sleep(100 * time.Millisecond)

	// 客户端
	clientSock, err := req.NewSocket()
	require.NoError(t, err)
	defer clientSock.Close()

	err = clientSock.Dial(addr)
	require.NoError(t, err)

	// 发送请求（协议头在 Body 前缀中）
	reqBody := []byte(`{"test": "data"}`)
	reqBodyWithHeader := protocol.NewMessageBody(protocol.MsgInvokeRequest, 1, reqBody)

	msg := mangos.NewMessage(0)
	msg.Body = reqBodyWithHeader

	err = clientSock.SendMsg(msg)
	require.NoError(t, err)

	// 接收响应
	resp, err := clientSock.RecvMsg()
	require.NoError(t, err)
	defer resp.Free()

	// 解析响应（协议头在 Body 前缀中）
	_, respMsgID, respReqID, respData, err := protocol.ParseMessageFromBody(resp.Body)
	require.NoError(t, err)

	assert.EqualValues(t, protocol.MsgInvokeResponse, respMsgID)
	assert.EqualValues(t, 1, respReqID)
	assert.Equal(t, []byte(`{"result": "ok"}`), respData)
}

// ============================================
// Server 基础测试
// ============================================

func TestServer_NewServer(t *testing.T) {
	handler := &mockHandler{}
	cfg := &Config{
		Address:  "127.0.0.1:19003",
		Insecure: true,
	}

	server, err := NewServer(cfg, handler)
	require.NoError(t, err)
	assert.NotNil(t, server)
	assert.False(t, server.IsClosed())

	err = server.Close()
	require.NoError(t, err)
	assert.True(t, server.IsClosed())

	// 双重关闭应该是幂等的
	err = server.Close()
	require.NoError(t, err)
}

func TestServer_NewServer_ListenError(t *testing.T) {
	handler := &mockHandler{}
	cfg := &Config{
		Address:  "invalid-address", // 无效地址
		Insecure: true,
	}

	_, err := NewServer(cfg, handler)
	assert.Error(t, err)
}

func TestServer_ReadyChannel(t *testing.T) {
	handler := &mockHandler{}
	cfg := &Config{
		Address:  "127.0.0.1:19004",
		Insecure: true,
	}

	server, err := NewServer(cfg, handler)
	require.NoError(t, err)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()

	// 等待 Ready 信号
	select {
	case <-server.Ready():
		// 正常
	case <-time.After(5 * time.Second):
		t.Fatal("server not ready")
	}
}

// ============================================
// Client 基础测试
// ============================================

func TestClient_NewClient_DialError(t *testing.T) {
	cfg := &Config{
		Address:  "invalid-host:9999",
		Insecure: true,
	}

	_, err := NewClient(cfg)
	assert.Error(t, err)
}

func TestClient_Close(t *testing.T) {
	// 需要先有一个服务器在监听
	handler := &mockHandler{}
	serverCfg := &Config{
		Address:  "127.0.0.1:19005",
		Insecure: true,
	}

	server, err := NewServer(serverCfg, handler)
	require.NoError(t, err)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = server.Serve(ctx)
	}()

	<-server.Ready()
	time.Sleep(100 * time.Millisecond)

	clientCfg := &Config{
		Address:  "127.0.0.1:19005",
		Insecure: true,
	}

	client, err := NewClient(clientCfg)
	require.NoError(t, err)

	assert.False(t, client.IsClosed())

	err = client.Close()
	require.NoError(t, err)
	assert.True(t, client.IsClosed())

	// 双重关闭
	err = client.Close()
	require.NoError(t, err)
}

// ============================================
// 集成测试（我们的 Client + 我们的 Server）
// ============================================

// TestIntegration_OurClient_OurServer 测试完整的请求-响应流程
// 客户端发送包含协议头的消息，服务器解析并响应
func TestIntegration_OurClient_OurServer(t *testing.T) {
	handler := &echoHandler{}

	serverCfg := &Config{
		Address:     "127.0.0.1:19006",
		Insecure:    true,
		RecvTimeout: 5 * time.Second,
	}

	server, err := NewServer(serverCfg, handler)
	require.NoError(t, err)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()

	<-server.Ready()
	time.Sleep(100 * time.Millisecond)

	// 使用我们的 Client
	clientCfg := &Config{
		Address:     "127.0.0.1:19006",
		Insecure:    true,
		SendTimeout: 5 * time.Second,
	}

	client, err := NewClient(clientCfg)
	require.NoError(t, err)
	defer client.Close()

	// 发送请求
	reqBody := []byte(`{"test": "data"}`)
	respMsgID, respBody, err := client.Call(context.Background(), protocol.MsgInvokeRequest, reqBody)
	require.NoError(t, err)

	// 验证响应
	assert.EqualValues(t, protocol.MsgInvokeResponse, respMsgID)
	assert.Contains(t, string(respBody), "echo")
	assert.Contains(t, string(respBody), `{"test": "data"}`)
}

// TestIntegration_MultipleRequests 测试多个连续请求
func TestIntegration_MultipleRequests(t *testing.T) {
	handler := &echoHandler{}

	serverCfg := &Config{
		Address:     "127.0.0.1:19007",
		Insecure:    true,
		RecvTimeout: 5 * time.Second,
	}

	server, err := NewServer(serverCfg, handler)
	require.NoError(t, err)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()

	<-server.Ready()
	time.Sleep(100 * time.Millisecond)

	clientCfg := &Config{
		Address:     "127.0.0.1:19007",
		Insecure:    true,
		SendTimeout: 5 * time.Second,
	}

	client, err := NewClient(clientCfg)
	require.NoError(t, err)
	defer client.Close()

	// 发送多个请求
	for i := 0; i < 5; i++ {
		reqBody := []byte(fmt.Sprintf(`{"request": %d}`, i))
		respMsgID, respBody, err := client.Call(context.Background(), protocol.MsgInvokeRequest, reqBody)
		require.NoError(t, err)
		assert.EqualValues(t, protocol.MsgInvokeResponse, respMsgID)
		assert.Contains(t, string(respBody), fmt.Sprintf(`{"request": %d}`, i))
	}
}

// TestIntegration_ErrorHandling 测试错误处理
func TestIntegration_ErrorHandling(t *testing.T) {
	handler := &errorHandler{}

	serverCfg := &Config{
		Address:     "127.0.0.1:19008",
		Insecure:    true,
		RecvTimeout: 5 * time.Second,
	}

	server, err := NewServer(serverCfg, handler)
	require.NoError(t, err)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()

	<-server.Ready()
	time.Sleep(100 * time.Millisecond)

	clientCfg := &Config{
		Address:     "127.0.0.1:19008",
		Insecure:    true,
		SendTimeout: 5 * time.Second,
	}

	client, err := NewClient(clientCfg)
	require.NoError(t, err)
	defer client.Close()

	// 发送请求，处理程序返回错误
	reqBody := []byte(`{"test": "data"}`)
	respMsgID, respBody, err := client.Call(context.Background(), protocol.MsgInvokeRequest, reqBody)
	require.NoError(t, err) // Call 本身成功（通信层）

	// 验证响应包含错误信息
	assert.EqualValues(t, protocol.MsgInvokeResponse, respMsgID)
	assert.Contains(t, string(respBody), "error")
}

// TestIntegration_ContextCancellation 测试上下文取消
func TestIntegration_ContextCancellation(t *testing.T) {
	handler := &slowHandler{}

	serverCfg := &Config{
		Address:     "127.0.0.1:19009",
		Insecure:    true,
		RecvTimeout: 5 * time.Second,
	}

	server, err := NewServer(serverCfg, handler)
	require.NoError(t, err)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()

	<-server.Ready()
	time.Sleep(100 * time.Millisecond)

	clientCfg := &Config{
		Address:     "127.0.0.1:19009",
		Insecure:    true,
		SendTimeout: 5 * time.Second,
	}

	client, err := NewClient(clientCfg)
	require.NoError(t, err)
	defer client.Close()

	// 创建一个会很快取消的上下文
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shortCancel()

	// 发送请求（服务器会慢处理，导致超时）
	reqBody := []byte(`{"test": "data"}`)
	_, _, err = client.Call(shortCtx, protocol.MsgInvokeRequest, reqBody)
	assert.Error(t, err)
}

// ============================================
// 原生 mangos 客户端测试（验证协议兼容性）
// ============================================

// TestIntegration_MangosClient_OurServer_RawBody 测试原生 mangos 客户端
// 这个测试验证：如果客户端直接在 Body 中放入协议头，服务器应该能正确解析
func TestIntegration_MangosClient_OurServer_RawBody(t *testing.T) {
	handler := &echoHandler{}

	serverCfg := &Config{
		Address:     "127.0.0.1:19010",
		Insecure:    true,
		RecvTimeout: 5 * time.Second,
	}

	server, err := NewServer(serverCfg, handler)
	require.NoError(t, err)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()

	<-server.Ready()
	time.Sleep(100 * time.Millisecond)

	// 使用原生 mangos 客户端
	clientSock, err := req.NewSocket()
	require.NoError(t, err)
	defer clientSock.Close()

	err = clientSock.Dial("tcp://127.0.0.1:19010")
	require.NoError(t, err)

	// 使用 protocol.NewMessageBody 创建带有协议头的请求体
	reqBody := []byte(`{"test": "data"}`)
	reqID := uint32(1)
	reqBodyWithHeader := protocol.NewMessageBody(protocol.MsgInvokeRequest, reqID, reqBody)

	// 发送请求（协议头在 Body 中）
	msg := mangos.NewMessage(0)
	msg.Body = reqBodyWithHeader

	err = clientSock.SendMsg(msg)
	require.NoError(t, err)

	// 接收响应
	resp, err := clientSock.RecvMsg()
	require.NoError(t, err)
	defer resp.Free()

	// 解析响应（协议头在 Body 中）
	_, respMsgID, respReqID, respData, err := protocol.ParseMessageFromBody(resp.Body)
	require.NoError(t, err)

	// 验证
	assert.EqualValues(t, protocol.MsgInvokeResponse, respMsgID)
	assert.EqualValues(t, reqID, respReqID)
	assert.Contains(t, string(respData), "echo")
}

// ============================================
// 辅助类型
// ============================================

type echoHandler struct{}

func (h *echoHandler) Handle(ctx context.Context, msgID uint32, reqID uint32, body []byte) ([]byte, error) {
	return []byte(`{"echo": "` + string(body) + `"}`), nil
}

type errorHandler struct{}

func (h *errorHandler) Handle(ctx context.Context, msgID uint32, reqID uint32, body []byte) ([]byte, error) {
	return nil, fmt.Errorf("test error: msgID=%d, reqID=%d", msgID, reqID)
}

type slowHandler struct{}

func (h *slowHandler) Handle(ctx context.Context, msgID uint32, reqID uint32, body []byte) ([]byte, error) {
	// 模拟慢处理
	time.Sleep(200 * time.Millisecond)
	return []byte(`{"result": "slow"}`), nil
}
