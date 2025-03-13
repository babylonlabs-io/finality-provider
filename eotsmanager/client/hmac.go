package client

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

const (
	// HMACHeaderKey is the metadata key for the HMAC
	HMACHeaderKey = "X-FPD-HMAC"
)

// HMACUnaryClientInterceptor creates a gRPC client interceptor that adds HMAC
// to outgoing requests. It skips adding HMAC for the Ping method and SaveEOTSKeyName.
func HMACUnaryClientInterceptor(hmacKey string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// Skip authentication for specific methods using exact matching
		// NOTE: we should disable hmac on pings to allow for health checks
		// NOTE: SaveEOTSKeyName is a local key management operation that doesn't require HMAC
		switch method {
		case "/proto.EOTSManager/Ping", "/proto.EOTSManager/SaveEOTSKeyName":

			return invoker(ctx, method, req, reply, cc, opts...)
		}

		// If HMAC key is empty, skip authentication
		if hmacKey == "" {
			fmt.Printf("HMAC authentication disabled for client: empty HMAC key\n")
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		msg, ok := req.(proto.Message)
		if !ok {
			fmt.Printf("HMAC generation failed: request is not a protobuf message\n")

			return fmt.Errorf("request is not a protobuf message")
		}

		data, err := proto.Marshal(msg)
		if err != nil {
			fmt.Printf("HMAC generation failed: failed to marshal request: %v\n", err)
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		// Generate HMAC using SHA-256
		h := hmac.New(sha256.New, []byte(hmacKey))
		h.Write(data)
		hmacValue := base64.StdEncoding.EncodeToString(h.Sum(nil))

		// Add HMAC to outgoing context
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		} else {
			md = md.Copy()
		}
		md.Set(HMACHeaderKey, hmacValue)
		newCtx := metadata.NewOutgoingContext(ctx, md)

		fmt.Printf("HMAC added to client request for method: %s\n", method)
		fmt.Printf("  HMAC value: %s\n", hmacValue)

		return invoker(newCtx, method, req, reply, cc, opts...)
	}
}
