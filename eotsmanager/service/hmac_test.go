package service_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"testing"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/service"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	protobuf "google.golang.org/protobuf/proto"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/proto"
)

func TestHMACVerification(t *testing.T) {
	t.Parallel()
	testKey := "test-hmac-key"
	testReq := &proto.SignEOTSRequest{
		Uid:     []byte("test-uid"),
		ChainId: []byte("test-chain"),
		Msg:     []byte("test-message"),
		Height:  100,
	}
	data, err := protobuf.Marshal(testReq)
	require.NoError(t, err)

	h := hmac.New(sha256.New, []byte(testKey))
	h.Write(data)
	validHMAC := base64.StdEncoding.EncodeToString(h.Sum(nil))

	md := metadata.New(map[string]string{
		client.HMACHeaderKey: validHMAC,
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(_ context.Context, req interface{}) (interface{}, error) {
		return req, nil
	}

	interceptor := service.HMACUnaryServerInterceptor(testKey)
	_, err = interceptor(
		ctx,
		testReq,
		&grpc.UnaryServerInfo{FullMethod: "/proto.EOTSManager/SignEOTS"},
		handler,
	)
	require.NoError(t, err)

	// Test with invalid HMAC
	invalidMD := metadata.New(map[string]string{
		client.HMACHeaderKey: "invalid-hmac",
	})
	invalidCtx := metadata.NewIncomingContext(context.Background(), invalidMD)

	_, err = interceptor(
		invalidCtx,
		testReq,
		&grpc.UnaryServerInfo{FullMethod: "/proto.EOTSManager/SignEOTS"},
		handler,
	)
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHMACKeyRetrieval(t *testing.T) {
	originalKey := os.Getenv(client.HMACKeyEnvVar)
	defer t.Setenv(client.HMACKeyEnvVar, originalKey)

	testKey := "test-hmac-secret-key"
	t.Setenv(client.HMACKeyEnvVar, testKey)

	key, err := client.GetHMACKey()
	require.NoError(t, err)
	require.Equal(t, testKey, key)

	os.Unsetenv(client.HMACKeyEnvVar)
	_, err = client.GetHMACKey()
	require.Error(t, err)
	require.Contains(t, err.Error(), "environment variable not set")
}

func TestHMACAuthDisabled(t *testing.T) {
	t.Parallel()
	testReq := &proto.SignEOTSRequest{
		Uid:     []byte("test-uid"),
		ChainId: []byte("test-chain"),
		Msg:     []byte("test-message"),
		Height:  100,
	}

	data, err := protobuf.Marshal(testReq)
	require.NoError(t, err)

	h := hmac.New(sha256.New, []byte(""))
	h.Write(data)
	emptyKeyHMAC := base64.StdEncoding.EncodeToString(h.Sum(nil))

	md := metadata.New(map[string]string{
		client.HMACHeaderKey: emptyKeyHMAC,
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(_ context.Context, req interface{}) (interface{}, error) {
		return req, nil
	}

	interceptor := service.HMACUnaryServerInterceptor("")
	_, err = interceptor(
		ctx,
		testReq,
		&grpc.UnaryServerInfo{FullMethod: "/proto.EOTSManager/SignEOTS"},
		handler,
	)
	require.NoError(t, err)
}

func TestMissingHMACHeader(t *testing.T) {
	t.Parallel()
	testKey := "test-hmac-key"
	testReq := &proto.SignEOTSRequest{
		Uid:     []byte("test-uid"),
		ChainId: []byte("test-chain"),
		Msg:     []byte("test-message"),
		Height:  100,
	}

	md := metadata.New(map[string]string{})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(_ context.Context, req interface{}) (interface{}, error) {
		return req, nil
	}

	interceptor := service.HMACUnaryServerInterceptor(testKey)
	_, err := interceptor(
		ctx,
		testReq,
		&grpc.UnaryServerInfo{FullMethod: "/proto.EOTSManager/SignEOTS"},
		handler,
	)
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestConfigHMACKey(t *testing.T) {
	testKey := "config-hmac-key"
	cfg := &config.Config{
		HMACKey: testKey,
	}
	require.Equal(t, testKey, cfg.HMACKey)

	originalKey := os.Getenv(client.HMACKeyEnvVar)
	defer t.Setenv(client.HMACKeyEnvVar, originalKey)

	envKey := "env-hmac-key"
	t.Setenv(client.HMACKeyEnvVar, envKey)

	key, err := client.GetHMACKey()
	require.NoError(t, err)
	require.Equal(t, envKey, key)
}
