package grpcserver

import (
	"context"
	"log"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"github.com/theheadmen/urlShort/internal/models"
	pb "github.com/theheadmen/urlShort/internal/proto"
	config "github.com/theheadmen/urlShort/internal/serverconfig"
	"github.com/theheadmen/urlShort/internal/storage"
	"github.com/theheadmen/urlShort/internal/storage/file"

	"github.com/theheadmen/urlShort/internal/serverapi"
)

const bufSize = 1024 * 1024

var lis *bufconn.Listener

func NewTestConfigStore() *config.ConfigStore {
	return &config.ConfigStore{
		FlagRunAddr:      ":8080",
		FlagShortRunAddr: "http://localhost:8080",
		FlagLogLevel:     "debug",
		FlagFile:         "/tmp/short-url-db.json",
		FlagDB:           "",
	}
}

func init() {
	configStore := NewTestConfigStore()
	storager := file.NewFileStoragerWithoutReadingData(configStore.FlagFile, false /*isWithFile*/, make(map[storage.URLMapKey]models.SavedURL))
	storager.SaveUserID(1)
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer(
		grpc.UnaryInterceptor(UnaryServerInterceptor(storager)),
		grpc.StreamInterceptor(StreamServerInterceptor(storager)),
	)
	pb.RegisterURLShortenerServiceServer(s, &grpcServer{
		// You need to provide a mock or stub implementation of the storage interface here
		storage:     storager,
		configStore: *configStore,
	})
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Server exited with error: %v", err)
		}
	}()
}

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

type jwtCreds struct {
	token string
}

// RequireTransportSecurity implements credentials.PerRPCCredentials.
func (c *jwtCreds) RequireTransportSecurity() bool {
	return false
}

func (c *jwtCreds) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + c.token,
	}, nil
}

func TestGRPCServer(t *testing.T) {
	ctx := context.Background()
	userID := 1
	claims := serverapi.UserClaims{
		UserID: strconv.Itoa(userID),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			Issuer:    "myServer",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign and get the complete encoded token as a string using the secret
	signedToken, err := token.SignedString([]byte(jwtSecretKey))
	require.NoError(t, err)
	jwtCreds := &jwtCreds{token: signedToken}

	md := metadata.Pairs("authorization", signedToken)
	ctx = context.WithValue(ctx, models.UserIDCredentials{}, userID)
	ctx = metadata.NewIncomingContext(ctx, md)
	ctx = metadata.NewOutgoingContext(ctx, md)

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		t.Fatal("can't get md from context")
	}
	_, ok = md["authorization"]
	if !ok {
		t.Fatal("can't auth from md")
	}

	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(bufDialer), grpc.WithInsecure(), grpc.WithPerRPCCredentials(jwtCreds))
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	defer conn.Close()
	client := pb.NewURLShortenerServiceClient(conn)

	t.Run("ShortenURL", func(t *testing.T) {
		resp, err := client.ShortenURL(ctx, &pb.Request{Url: "http://example.com"})
		require.NoError(t, err)
		assert.Equal(t, "http://localhost:8080/8OamqXBC", resp.Result)
	})

	t.Run("GetURL", func(t *testing.T) {
		resp, err := client.GetURL(ctx, &pb.Request{Url: "8OamqXBC"})
		require.NoError(t, err)
		assert.Equal(t, "http://example.com", resp.Result)
	})

	t.Run("ShortenURLBatch", func(t *testing.T) {
		stream, err := client.ShortenURLBatch(ctx)
		require.NoError(t, err)
		for _, url := range []string{"http://example1.com", "http://example2.com"} {
			err := stream.Send(&pb.BatchRequest{OriginalUrl: url})
			require.NoError(t, err)
			resp, err := stream.Recv()
			require.NoError(t, err)
			// конкретные значения проверяем в основном тесте
			assert.NotEmpty(t, resp.ShortUrl)
		}
	})

	t.Run("GetURLsByUserID", func(t *testing.T) {
		stream, err := client.GetURLsByUserID(ctx, &pb.Request{})
		require.NoError(t, err)
		resp, err := stream.Recv()
		require.NoError(t, err)
		// конкретные значения проверяем в основном тесте
		assert.NotEmpty(t, resp.ShortUrl)
		assert.NotEmpty(t, resp.OriginalUrl)
		resp, err = stream.Recv()
		require.NoError(t, err)
		assert.NotEmpty(t, resp.ShortUrl)
		assert.NotEmpty(t, resp.OriginalUrl)
	})

	t.Run("DeleteURLs", func(t *testing.T) {
		stream, err := client.DeleteURLs(ctx)
		require.NoError(t, err)
		for _, url := range []string{"shortURL1", "shortURL2"} {
			err := stream.Send(&pb.Request{Url: url})
			require.NoError(t, err)
		}
		resp, err := stream.CloseAndRecv()
		require.NoError(t, err)
		assert.Equal(t, "URLs deleted", resp.Result)
	})

	t.Run("GetStats", func(t *testing.T) {
		_, err := client.GetStats(ctx, &pb.Request{})
		assert.Equal(t, "rpc error: code = PermissionDenied desc = no trusted subnet", err.Error())
	})
}
