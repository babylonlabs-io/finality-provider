package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
)

// GetHMACKeyFromEnv retrieves the HMAC key from the environment variable
func GetHMACKeyFromEnv() (string, error) {
	key := os.Getenv(client.HMACKeyEnvVar)
	if key == "" {
		return "", fmt.Errorf("HMAC_KEY environment variable not set")
	}

	return key, nil
}

// HMACUnaryServerInterceptor creates a gRPC server interceptor that verifies HMAC signatures
// on incoming requests. It bypasses authentication for the Ping method and SaveEOTSKeyName.
func HMACUnaryServerInterceptor(hmacKey string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Skip authentication for specific methods using exact matching
		// NOTE: we should disable hmac on pings to allow for health checks
		// NOTE: SaveEOTSKeyName is a local key management operation that doesn't require HMAC
		switch info.FullMethod {
		case "/proto.EOTSManager/Ping", "/proto.EOTSManager/SaveEOTSKeyName":
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Errorf(codes.Unauthenticated, "metadata is not provided")
		}

		// Get HMAC from metadata
		values := md.Get(client.HMACHeaderKey)
		if len(values) == 0 {
			return nil, status.Errorf(codes.Unauthenticated, "HMAC signature not provided")
		}
		receivedHMAC := values[0]

		msg, ok := req.(proto.Message)
		if !ok {
			return nil, status.Errorf(codes.Internal, "request is not a protobuf message")
		}

		data, err := proto.Marshal(msg)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to marshal request: %v", err)
		}

		// Generate HMAC using SHA-256
		h := hmac.New(sha256.New, []byte(hmacKey))
		h.Write(data)
		expectedHMAC := base64.StdEncoding.EncodeToString(h.Sum(nil))

		// Compare HMACs using constant-time comparison to avoid timing attacks
		if !hmac.Equal([]byte(receivedHMAC), []byte(expectedHMAC)) {
			return nil, status.Errorf(codes.Unauthenticated, "invalid HMAC signature")
		}

		return handler(ctx, req)
	}
}
