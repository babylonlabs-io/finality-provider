package client

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

const (
	// HMACKeyEnvVar is the environment variable name for the HMAC key
	HMACKeyEnvVar = "HMAC_KEY"
	// HMACHeaderKey is the metadata key for the HMAC signature
	HMACHeaderKey = "X-FPD-HMAC"
)

// GetHMACKey retrieves the HMAC key from the environment variable
func GetHMACKey() (string, error) {
	key := os.Getenv(HMACKeyEnvVar)
	if key == "" {
		return "", fmt.Errorf("HMAC_KEY environment variable not set")
	}
	return key, nil
}

// HMACUnaryClientInterceptor creates a gRPC client interceptor that adds HMAC signatures
// to outgoing requests. It bypasses authentication for the Ping method.
func HMACUnaryClientInterceptor(hmacKey string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// Skip authentication for Ping method
		// NOTE: we should disable hmac on pings to allow for health checks
		if strings.HasSuffix(method, "/Ping") {
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		msg, ok := req.(proto.Message)
		if !ok {
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		data, err := proto.Marshal(msg)
		if err != nil {
			return err
		}

		// Generate HMAC using SHA-256
		h := hmac.New(sha256.New, []byte(hmacKey))
		h.Write(data)
		signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

		// Add HMAC to the context metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}
		md = md.Copy()
		md.Set(HMACHeaderKey, signature)
		ctx = metadata.NewOutgoingContext(ctx, md)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
