package croupier

import (
	"context"
	"testing"
	"time"

	functionv1 "github.com/cuihairu/croupier/pkg/pb/croupier/function/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/metadata"
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
		name    string
		jobID   string
		wantErr bool
		wantCode codes.Code
	}{
		{
			name:    "cancel non-existent job",
			jobID:   "nonexistent",
			wantErr: false, // CancelJob returns success even if job doesn't exist
		},
		{
			name:    "empty job id",
			jobID:   "",
			wantErr: true,
			wantCode: codes.InvalidArgument,
		},
		{
			name:    "nil request",
			jobID:   "",
			wantErr: true,
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
		handlerCalled := false
		handlers := map[string]FunctionHandler{
			"error_func": func(ctx context.Context, payload []byte) ([]byte, error) {
				handlerCalled = true
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

		if !handlerCalled {
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
		handlerCalled := false
		handlers := map[string]FunctionHandler{
			"success_func": func(ctx context.Context, payload []byte) ([]byte, error) {
				handlerCalled = true
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

		if !handlerCalled {
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
	t      *testing.T
	sent   bool
	events []*functionv1.JobEvent
}

func (m *mockStreamServer) Send(event *functionv1.JobEvent) error {
	m.sent = true
	m.events = append(m.events, event)
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
