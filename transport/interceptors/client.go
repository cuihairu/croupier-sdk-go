package interceptors

import (
    "context"
    "math"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

type Config struct { Timeout time.Duration; MaxAttempts int; BackoffBase time.Duration }
func defaults() Config { return Config{Timeout: 5 * time.Second, MaxAttempts: 3, BackoffBase: 100*time.Millisecond} }

func Chain(cfg *Config) []grpc.DialOption {
    c := defaults(); if cfg != nil { c = *cfg }
    ui := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
        if _, ok := ctx.Deadline(); !ok && c.Timeout > 0 { var cancel context.CancelFunc; ctx, cancel = context.WithTimeout(ctx, c.Timeout); defer cancel() }
        for a:=1; ; a++ {
            if err := invoker(ctx, method, req, reply, cc, opts...); err != nil {
                if a >= c.MaxAttempts { return err }
                st, _ := status.FromError(err)
                if st.Code()!=codes.Unavailable && st.Code()!=codes.DeadlineExceeded { return err }
                d := time.Duration(float64(c.BackoffBase)*math.Pow(2, float64(a-1)))
                select { case <-time.After(d): case <-ctx.Done(): return ctx.Err() }
                continue
            }
            return nil
        }
    }
    si := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
        if _, ok := ctx.Deadline(); !ok && c.Timeout > 0 { var cancel context.CancelFunc; ctx, cancel = context.WithTimeout(ctx, c.Timeout); defer cancel() }
        for a:=1; ; a++ {
            cs, err := streamer(ctx, desc, cc, method, opts...)
            if err != nil {
                if a >= c.MaxAttempts { return nil, err }
                st, _ := status.FromError(err)
                if st.Code()!=codes.Unavailable && st.Code()!=codes.DeadlineExceeded { return nil, err }
                d := time.Duration(float64(c.BackoffBase)*math.Pow(2, float64(a-1)))
                select { case <-time.After(d): case <-ctx.Done(): return nil, ctx.Err() }
                continue
            }
            return cs, nil
        }
    }
    return []grpc.DialOption{ grpc.WithChainUnaryInterceptor(ui), grpc.WithChainStreamInterceptor(si) }
}

