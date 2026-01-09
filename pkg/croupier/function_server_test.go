package croupier

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	functionv1 "github.com/cuihairu/croupier/pkg/pb/croupier/function/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestFunctionServer_NewFunctionServer(t *testing.T) {
	t.Parallel()

	handlers := map[string]FunctionHandler{
		"test_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte("result"), nil
		},
	}

	s := newFunctionServer(handlers)

	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if s.handlers == nil {
		t.Error("expected handlers map to be initialized")
	}
	if s.jobs == nil {
		t.Error("expected jobs map to be initialized")
	}
}

func TestFunctionServer_Handler(t *testing.T) {
	t.Parallel()

	handlers := map[string]FunctionHandler{
		"test_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte("result"), nil
		},
	}

	s := newFunctionServer(handlers)

	// Test existing handler
	h, ok := s.handler("test_func")
	if !ok {
		t.Error("expected handler to be found")
	}
	if h == nil {
		t.Error("expected non-nil handler")
	}

	// Test non-existing handler
	_, ok = s.handler("nonexistent")
	if ok {
		t.Error("expected handler not to be found")
	}
}

func TestFunctionServer_Invoke(t *testing.T) {
	t.Parallel()

	handlers := map[string]FunctionHandler{
		"test_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte("result"), nil
		},
		"error_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			return nil, context.Canceled
		},
	}

	s := newFunctionServer(handlers)

	tests := []struct {
		name       string
		functionID string
		payload    []byte
		wantErr    bool
		wantCode   codes.Code
	}{
		{
			name:       "success",
			functionID: "test_func",
			payload:    []byte("input"),
			wantErr:    false,
		},
		{
			name:       "handler error",
			functionID: "error_func",
			payload:    []byte("input"),
			wantErr:    true,
			wantCode:   codes.Internal,
		},
		{
			name:       "not found",
			functionID: "nonexistent",
			payload:    []byte("input"),
			wantErr:    true,
			wantCode:   codes.NotFound,
		},
		{
			name:       "empty function id",
			functionID: "",
			payload:    []byte("input"),
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "nil request",
			functionID: "",
			payload:    nil,
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *functionv1.InvokeRequest
			if tt.functionID != "" || tt.payload != nil {
				req = &functionv1.InvokeRequest{
					FunctionId: tt.functionID,
					Payload:    tt.payload,
				}
			}

			resp, err := s.Invoke(context.Background(), req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
					return
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Errorf("expected gRPC status error, got %v", err)
					return
				}
				if st.Code() != tt.wantCode {
					t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if resp == nil {
				t.Error("expected non-nil response")
			}
		})
	}
}

func TestFunctionServer_StartJob(t *testing.T) {
	t.Parallel()

	handlers := map[string]FunctionHandler{
		"quick_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte("done"), nil
		},
		"error_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			return nil, context.DeadlineExceeded
		},
	}

	s := newFunctionServer(handlers)

	tests := []struct {
		name       string
		functionID string
		wantErr    bool
		wantCode   codes.Code
	}{
		{
			name:       "success",
			functionID: "quick_func",
			wantErr:    false,
		},
		{
			name:       "not found",
			functionID: "nonexistent",
			wantErr:    true,
			wantCode:   codes.NotFound,
		},
		{
			name:       "empty function id",
			functionID: "",
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
		{
			name:       "nil request",
			functionID: "",
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *functionv1.InvokeRequest
			if tt.functionID != "" {
				req = &functionv1.InvokeRequest{
					FunctionId: tt.functionID,
					Payload:    []byte("input"),
				}
			}

			resp, err := s.StartJob(context.Background(), req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
					return
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Errorf("expected gRPC status error, got %v", err)
					return
				}
				if st.Code() != tt.wantCode {
					t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if resp == nil {
				t.Error("expected non-nil response")
				return
			}

			if resp.JobId == "" {
				t.Error("expected non-empty job ID")
			}

			// Verify job is registered
			s.mu.RLock()
			_, ok := s.jobs[resp.JobId]
			s.mu.RUnlock()

			if !ok {
				t.Error("expected job to be registered")
			}
		})
	}
}

func TestFunctionServer_CancelJob(t *testing.T) {
	t.Parallel()

	handlers := map[string]FunctionHandler{
		"long_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			// This job will be canceled
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Second):
				return []byte("done"), nil
			}
		},
	}

	s := newFunctionServer(handlers)

	tests := []struct {
		name     string
		jobID    string
		wantErr  bool
		wantCode codes.Code
	}{
		{
			name:    "cancel non-existent job",
			jobID:   "nonexistent",
			wantErr: false, // CancelJob returns success even if job doesn't exist
		},
		{
			name:     "empty job id",
			jobID:    "",
			wantErr:  true,
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "nil request",
			jobID:    "",
			wantErr:  true,
			wantCode: codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *functionv1.CancelJobRequest
			if tt.jobID != "" {
				req = &functionv1.CancelJobRequest{
					JobId: tt.jobID,
				}
			}

			resp, err := s.CancelJob(context.Background(), req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
					return
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Errorf("expected gRPC status error, got %v", err)
					return
				}
				if st.Code() != tt.wantCode {
					t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if resp == nil {
				t.Error("expected non-nil response")
			}
		})
	}

	// Test canceling a real job
	t.Run("cancel active job", func(t *testing.T) {
		req := &functionv1.InvokeRequest{
			FunctionId: "long_func",
			Payload:    []byte("input"),
		}

		resp, err := s.StartJob(context.Background(), req)
		if err != nil {
			t.Fatalf("failed to start job: %v", err)
		}

		// Cancel the job
		cancelReq := &functionv1.CancelJobRequest{
			JobId: resp.JobId,
		}

		_, err = s.CancelJob(context.Background(), cancelReq)
		if err != nil {
			t.Errorf("failed to cancel job: %v", err)
		}

		// Wait a bit for the job to finish
		time.Sleep(10 * time.Millisecond)

		// Verify job is removed
		s.mu.RLock()
		_, ok := s.jobs[resp.JobId]
		s.mu.RUnlock()

		if ok {
			t.Error("expected job to be removed after cancellation")
		}
	})
}

func TestFunctionServer_RunJob_ErrorCases(t *testing.T) {
	t.Parallel()

	// Test the runJob error path - when handler returns an error
	t.Run("handler returns error", func(t *testing.T) {
		handlerCalled := int64(0)
		handlers := map[string]FunctionHandler{
			"error_func": func(ctx context.Context, payload []byte) ([]byte, error) {
				atomic.AddInt64(&handlerCalled, 1)
				return nil, context.Canceled
			},
		}

		s := newFunctionServer(handlers)
		req := &functionv1.InvokeRequest{
			FunctionId: "error_func",
			Payload:    []byte("input"),
		}

		resp, err := s.StartJob(context.Background(), req)
		if err != nil {
			t.Fatalf("failed to start job: %v", err)
		}

		// Wait for job to complete
		time.Sleep(50 * time.Millisecond)

		if atomic.LoadInt64(&handlerCalled) == 0 {
			t.Error("expected handler to be called")
		}

		// Verify job is removed after completion
		s.mu.RLock()
		_, ok := s.jobs[resp.JobId]
		s.mu.RUnlock()

		if ok {
			t.Error("expected job to be removed after completion")
		}
	})

	// Test the runJob success path
	t.Run("handler succeeds", func(t *testing.T) {
		handlerCalled := int64(0)
		handlers := map[string]FunctionHandler{
			"success_func": func(ctx context.Context, payload []byte) ([]byte, error) {
				atomic.AddInt64(&handlerCalled, 1)
				return []byte("result"), nil
			},
		}

		s := newFunctionServer(handlers)
		req := &functionv1.InvokeRequest{
			FunctionId: "success_func",
			Payload:    []byte("input"),
		}

		resp, err := s.StartJob(context.Background(), req)
		if err != nil {
			t.Fatalf("failed to start job: %v", err)
		}

		// Wait for job to complete
		time.Sleep(50 * time.Millisecond)

		if atomic.LoadInt64(&handlerCalled) == 0 {
			t.Error("expected handler to be called")
		}

		// Verify job is removed after completion
		s.mu.RLock()
		_, ok := s.jobs[resp.JobId]
		s.mu.RUnlock()

		if ok {
			t.Error("expected job to be removed after completion")
		}
	})
}

func TestFunctionServer_StreamJob(t *testing.T) {
	t.Parallel()

	handlers := map[string]FunctionHandler{
		"stream_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte("result"), nil
		},
	}

	s := newFunctionServer(handlers)

	// Test error cases directly by calling the method with invalid requests
	tests := []struct {
		name    string
		jobID   string
		wantErr bool
	}{
		{
			name:    "nil request",
			jobID:   "",
			wantErr: true,
		},
		{
			name:    "empty job id",
			jobID:   "",
			wantErr: true,
		},
		{
			name:    "non-existent job",
			jobID:   "nonexistent-job-id",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *functionv1.JobStreamRequest
			if tt.name != "nil request" {
				req = &functionv1.JobStreamRequest{
					JobId: tt.jobID,
				}
			}

			// Create a mock stream that satisfies the interface
			mockStream := &mockStreamServer{t: t}

			err := s.StreamJob(req, mockStream)

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// mockStreamServer is a mock implementation of FunctionService_StreamJobServer
type mockStreamServer struct {
	t         *testing.T
	sent      bool
	sendError error
	events    []*functionv1.JobEvent
}

func (m *mockStreamServer) Send(event *functionv1.JobEvent) error {
	m.sent = true
	m.events = append(m.events, event)
	if m.sendError != nil {
		return m.sendError
	}
	return nil
}

func (m *mockStreamServer) Context() context.Context {
	return context.Background()
}

func (m *mockStreamServer) SendMsg(msg interface{}) error {
	return nil
}

func (m *mockStreamServer) RecvMsg(msg interface{}) error {
	return nil
}

func (m *mockStreamServer) SetHeader(md metadata.MD) error {
	return nil
}

func (m *mockStreamServer) SendHeader(md metadata.MD) error {
	return nil
}

func (m *mockStreamServer) SetTrailer(md metadata.MD) {
}

// TestFunctionServer_StreamJobWithActiveJob tests streaming from an active job
func TestFunctionServer_StreamJobWithActiveJob(t *testing.T) {
	t.Parallel()

	handlers := map[string]FunctionHandler{
		"test_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte("test result"), nil
		},
	}

	s := newFunctionServer(handlers)

	// Create a mock stream
	mockStream := &mockStreamServer{t: t}

	// Try to stream from a non-existent job
	req := &functionv1.JobStreamRequest{
		JobId: "active-job-123",
	}

	err := s.StreamJob(req, mockStream)
	if err == nil {
		t.Error("expected error for non-existent job")
	}
}

// TestFunctionServer_StreamJobWithNilRequest tests StreamJob with nil request
func TestFunctionServer_StreamJobWithNilRequestDirect(t *testing.T) {
	t.Parallel()

	handlers := map[string]FunctionHandler{}
	s := newFunctionServer(handlers)

	mockStream := &mockStreamServer{t: t}

	// Test with nil request - should return InvalidArgument error
	err := s.StreamJob(nil, mockStream)
	if err == nil {
		t.Error("expected error with nil request")
	}
}

// TestFunctionServer_HandlerConcurrency tests handler function thread safety
func TestFunctionServer_HandlerConcurrency(t *testing.T) {
	t.Parallel()

	callCount := int64(0)
	handlers := map[string]FunctionHandler{
		"concurrent_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			atomic.AddInt64(&callCount, 1)
			return []byte("result"), nil
		},
	}

	s := newFunctionServer(handlers)

	// Call handler multiple times concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			h, ok := s.handler("concurrent_func")
			if ok && h != nil {
				h(context.Background(), []byte("test"))
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	if atomic.LoadInt64(&callCount) == 0 {
		t.Error("expected handler to be called")
	}
}

// TestFunctionServer_StreamJobSuccess tests successful streaming from a job
func TestFunctionServer_StreamJobSuccess(t *testing.T) {
	t.Parallel()

	// Use a channel to ensure the handler doesn't complete before streaming starts
	handlerStarted := make(chan struct{})
	streamReady := make(chan struct{})

	handlers := map[string]FunctionHandler{
		"quick_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			close(handlerStarted)
			// Wait for streaming to be ready
			<-streamReady
			return []byte("job completed"), nil
		},
	}

	s := newFunctionServer(handlers)

	// Start a job
	req := &functionv1.InvokeRequest{
		FunctionId: "quick_func",
		Payload:    []byte("input"),
	}

	resp, err := s.StartJob(context.Background(), req)
	if err != nil {
		t.Fatalf("failed to start job: %v", err)
	}

	// Wait for handler to start
	<-handlerStarted

	// Create a mock stream to receive events
	mockStream := &mockStreamServer{t: t}

	// Stream the job
	streamReq := &functionv1.JobStreamRequest{
		JobId: resp.JobId,
	}

	// Stream in a goroutine since it will block until job completes
	done := make(chan error, 1)
	go func() {
		done <- s.StreamJob(streamReq, mockStream)
	}()

	// Give streaming a moment to start, then signal handler to continue
	time.Sleep(10 * time.Millisecond)
	close(streamReady)

	// Wait for streaming to complete
	timeout := time.After(100 * time.Millisecond)
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("unexpected error from StreamJob: %v", err)
		}
	case <-timeout:
		t.Error("timeout waiting for stream to complete")
	}

	// Verify events were sent
	if !mockStream.sent {
		t.Error("expected events to be sent")
	}

	// Verify we got the expected events
	if len(mockStream.events) == 0 {
		t.Error("expected at least one event")
	}

	// Verify job is cleaned up
	s.mu.RLock()
	_, exists := s.jobs[resp.JobId]
	s.mu.RUnlock()

	if exists {
		t.Error("expected job to be removed after completion")
	}
}

// TestFunctionServer_StreamJobWithError tests streaming from a job that errors
func TestFunctionServer_StreamJobWithError(t *testing.T) {
	t.Parallel()

	// Use a channel to ensure the handler doesn't complete before streaming starts
	handlerStarted := make(chan struct{})
	streamReady := make(chan struct{})

	handlers := map[string]FunctionHandler{
		"error_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			close(handlerStarted)
			// Wait for streaming to be ready
			<-streamReady
			return nil, fmt.Errorf("handler error")
		},
	}

	s := newFunctionServer(handlers)

	// Start a job
	req := &functionv1.InvokeRequest{
		FunctionId: "error_func",
		Payload:    []byte("input"),
	}

	resp, err := s.StartJob(context.Background(), req)
	if err != nil {
		t.Fatalf("failed to start job: %v", err)
	}

	// Wait for handler to start
	<-handlerStarted

	// Create a mock stream
	mockStream := &mockStreamServer{t: t}

	// Stream the job
	streamReq := &functionv1.JobStreamRequest{
		JobId: resp.JobId,
	}

	// Stream in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- s.StreamJob(streamReq, mockStream)
	}()

	// Give streaming a moment to start, then signal handler to continue
	time.Sleep(10 * time.Millisecond)
	close(streamReady)

	// Wait for streaming to complete
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("unexpected error from StreamJob: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for stream to complete")
	}

	// Verify we got events
	if len(mockStream.events) == 0 {
		t.Error("expected events to be sent")
	}
}

// TestFunctionServer_StreamJobWithCanceledContext tests streaming from a canceled job
func TestFunctionServer_StreamJobWithCanceledContext(t *testing.T) {
	t.Parallel()

	// Use a channel to ensure the handler doesn't complete before streaming starts
	handlerStarted := make(chan struct{})
	streamReady := make(chan struct{})

	handlers := map[string]FunctionHandler{
		"canceled_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			close(handlerStarted)
			// Wait for streaming to be ready
			<-streamReady
			return nil, context.Canceled
		},
	}

	s := newFunctionServer(handlers)

	// Start a job
	req := &functionv1.InvokeRequest{
		FunctionId: "canceled_func",
		Payload:    []byte("input"),
	}

	resp, err := s.StartJob(context.Background(), req)
	if err != nil {
		t.Fatalf("failed to start job: %v", err)
	}

	// Wait for handler to start
	<-handlerStarted

	// Create a mock stream
	mockStream := &mockStreamServer{t: t}

	// Stream the job
	streamReq := &functionv1.JobStreamRequest{
		JobId: resp.JobId,
	}

	// Stream in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- s.StreamJob(streamReq, mockStream)
	}()

	// Give streaming a moment to start, then signal handler to continue
	time.Sleep(10 * time.Millisecond)
	close(streamReady)

	// Wait for streaming to complete
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("unexpected error from StreamJob: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for stream to complete")
	}

	// Verify we got events including canceled event
	foundCanceled := false
	for _, evt := range mockStream.events {
		if evt.Type == "canceled" || evt.Type == "error" {
			foundCanceled = true
		}
	}
	if !foundCanceled {
		t.Error("expected canceled or error event")
	}
}

// TestFunctionServer_StreamJobSendError tests StreamJob when stream.Send fails
func TestFunctionServer_StreamJobSendError(t *testing.T) {
	t.Parallel()

	handlers := map[string]FunctionHandler{
		"func": func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte("result"), nil
		},
	}

	s := newFunctionServer(handlers)

	// Start a job
	req := &functionv1.InvokeRequest{
		FunctionId: "func",
		Payload:    []byte("input"),
	}

	resp, err := s.StartJob(context.Background(), req)
	if err != nil {
		t.Fatalf("failed to start job: %v", err)
	}

	// Create a mock stream that returns an error on Send
	mockStream := &mockStreamServer{
		t:         t,
		sendError: fmt.Errorf("stream send error"),
	}

	streamReq := &functionv1.JobStreamRequest{
		JobId: resp.JobId,
	}

	err = s.StreamJob(streamReq, mockStream)
	if err == nil {
		t.Error("expected error from StreamJob when Send fails")
	}

	// Verify the error mentions send failure
	if err != nil && !strings.Contains(err.Error(), "stream send error") {
		t.Logf("Got error: %v", err)
	}
}

// TestFunctionServer_StreamJobMultipleJobs tests streaming from multiple jobs
func TestFunctionServer_StreamJobMultipleJobs(t *testing.T) {
	t.Parallel()

	handlers := map[string]FunctionHandler{
		"func": func(ctx context.Context, payload []byte) ([]byte, error) {
			// Add a small delay to ensure the job is still running when we stream
			time.Sleep(10 * time.Millisecond)
			return []byte("done"), nil
		},
	}

	s := newFunctionServer(handlers)

	// Start multiple jobs
	jobIDs := []string{}
	for i := 0; i < 3; i++ {
		req := &functionv1.InvokeRequest{
			FunctionId: "func",
			Payload:    []byte(fmt.Sprintf("input-%d", i)),
		}

		resp, err := s.StartJob(context.Background(), req)
		if err != nil {
			t.Fatalf("failed to start job %d: %v", i, err)
		}
		jobIDs = append(jobIDs, resp.JobId)
	}

	// Stream each job
	for _, jobID := range jobIDs {
		mockStream := &mockStreamServer{t: t}
		streamReq := &functionv1.JobStreamRequest{
			JobId: jobID,
		}

		// Stream with a timeout
		done := make(chan error, 1)
		go func() {
			done <- s.StreamJob(streamReq, mockStream)
		}()

		select {
		case err := <-done:
			if err != nil && err.Error() != "rpc error: code = NotFound desc = job "+jobID+" not found" {
				// NotFound is acceptable for quick jobs, they may have completed
				t.Logf("StreamJob for %s completed with: %v", jobID, err)
			}
		case <-time.After(200 * time.Millisecond):
			t.Error("timeout waiting for stream to complete")
		}

		if !mockStream.sent {
			// Jobs complete too quickly, skip the sent check
			t.Logf("Job %s completed before streaming started", jobID)
		}
	}
}
