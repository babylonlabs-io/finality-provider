package client

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	protobuf "google.golang.org/protobuf/proto"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/proto"
)

func TestHMACGeneration(t *testing.T) {
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
	expectedHMAC := base64.StdEncoding.EncodeToString(h.Sum(nil))

	var capturedMD metadata.MD
	fakeInvoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		require.True(t, ok)
		capturedMD = md
		return nil
	}

	interceptor := HMACUnaryClientInterceptor(testKey)
	err = interceptor(
		context.Background(),
		"/proto.EOTSManager/SignEOTS",
		testReq,
		nil,
		nil,
		fakeInvoker,
	)
	require.NoError(t, err)

	hmacValues := capturedMD.Get(HMACHeaderKey)
	require.Len(t, hmacValues, 1)
	require.Equal(t, expectedHMAC, hmacValues[0])
}
