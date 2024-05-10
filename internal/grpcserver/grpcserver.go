package grpcserver

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/theheadmen/urlShort/internal/models"
	pb "github.com/theheadmen/urlShort/internal/proto"
	"github.com/theheadmen/urlShort/internal/serverapi"
	config "github.com/theheadmen/urlShort/internal/serverconfig"
	"github.com/theheadmen/urlShort/internal/storage"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const (
	jwtSecretKey = "my-jwt-secret-key"
)

type grpcServer struct {
	pb.UnimplementedURLShortenerServiceServer
	storage     storage.Storage
	configStore config.ConfigStore
}

func (s *grpcServer) ShortenURL(ctx context.Context, in *pb.Request) (*pb.Response, error) {
	userID, ok := ctx.Value(models.UserIDCredentials{}).(int)
	if !ok {
		return nil, status.Error(codes.Internal, "cannot get userID from creds")
	}

	shortenURL := serverapi.GenerateShortURL(in.Url)
	// Implement the logic to shorten the URL
	_, err := s.storage.StoreURL(ctx, shortenURL, in.Url, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cannot store data for user: %v", err)
	}
	servShortURL := s.configStore.FlagShortRunAddr

	return &pb.Response{Result: servShortURL + "/" + shortenURL}, nil
}

func (s *grpcServer) GetURL(ctx context.Context, in *pb.Request) (*pb.Response, error) {
	// Implement the logic to get the URL
	originalURL, _, err := s.storage.GetURLForAnyUserID(ctx, in.Url)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cannot get data for user: %v", err)
	}
	return &pb.Response{Result: originalURL.OriginalURL}, nil
}

func (s *grpcServer) ShortenURLBatch(stream pb.URLShortenerService_ShortenURLBatchServer) error {
	servShortURL := s.configStore.FlagShortRunAddr
	userID, ok := stream.Context().Value(models.UserIDCredentials{}).(int)
	if !ok {
		return status.Error(codes.Internal, "cannot get userID from creds")
	}

	// Implement the logic to shorten URLs in batch
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, "cannot read data from stream: %v", err)
		}

		shortenURL := serverapi.GenerateShortURL(req.OriginalUrl)
		// Shorten the URL and send the response
		_, err = s.storage.StoreURL(stream.Context(), shortenURL, req.OriginalUrl, userID)
		if err != nil {
			return status.Errorf(codes.Internal, "cannot store data for user: %v", err)
		}
		if err := stream.Send(&pb.BatchResponse{CorrelationId: req.CorrelationId, ShortUrl: servShortURL + "/" + shortenURL}); err != nil {
			return status.Errorf(codes.Internal, "cannot send data for user: %v", err)
		}
	}
}

func (s *grpcServer) GetURLsByUserID(in *pb.Request, stream pb.URLShortenerService_GetURLsByUserIDServer) error {
	userID, ok := stream.Context().Value(models.UserIDCredentials{}).(int)
	if !ok {
		return status.Error(codes.Internal, "cannot get userID from creds")
	}
	savedURLs, err := s.storage.ReadAllDataForUserID(stream.Context(), userID)
	servShortURL := s.configStore.FlagShortRunAddr
	if err != nil {
		return status.Errorf(codes.NotFound, "cannot read data for user: %v", err)
	}

	if len(savedURLs) == 0 {
		return status.Errorf(codes.NotFound, "We find no urls for user: %v", err)
	}

	for _, savedURL := range savedURLs {
		if err := stream.Send(&pb.BatchByUserIDResponse{ShortUrl: servShortURL + "/" + savedURL.ShortURL, OriginalUrl: savedURL.OriginalURL}); err != nil {
			return status.Errorf(codes.Internal, "cannot send data for user: %v", err)
		}
	}

	return nil
}

func (s *grpcServer) DeleteURLs(stream pb.URLShortenerService_DeleteURLsServer) error {
	userID, ok := stream.Context().Value(models.UserIDCredentials{}).(int)
	if !ok {
		return status.Error(codes.Internal, "cannot get userID from creds")
	}

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.Response{Result: "URLs deleted"})
		}
		if err != nil {
			return status.Errorf(codes.Internal, "cannot read data from stream: %v", err)
		}

		err = s.storage.DeleteByUserID(stream.Context(), []string{req.Url}, userID)
		if err != nil {
			return status.Errorf(codes.Internal, "cannot delete data from stream: %v", err)
		}
	}
}

func (s *grpcServer) GetStats(ctx context.Context, in *pb.Request) (*pb.StatsResponse, error) {
	trustedSubnet := s.configStore.FlagTrustedSubnet
	if trustedSubnet == "" {
		return nil, status.Error(codes.PermissionDenied, "no trusted subnet")
	}

	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "no peer info")
	}

	clientIP, ok := p.Addr.(*net.TCPAddr)
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "no correct addr")
	}

	_, subnet, _ := net.ParseCIDR(trustedSubnet)
	if !subnet.Contains(clientIP.IP) {
		return nil, status.Error(codes.PermissionDenied, "access is forbidden")
	}

	stats, err := s.storage.GetStats(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cannot get stats: %v", err)
	}
	return &pb.StatsResponse{Urls: int32(stats.URLs), Users: int32(stats.Users)}, nil
}

func makeNewCtxWithUserID(ctx context.Context, storage storage.Storage) (context.Context, error) {
	// If metadata is not provided, generate a new userID
	userID, err := storage.GetLastUserID(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get last userID: %v", err)
	}

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
	if err != nil {
		return nil, status.Errorf(codes.Internal, "internal Server Error: %v", err)
	}

	md := metadata.Pairs("authorization", signedToken)
	ctx = metadata.NewIncomingContext(ctx, md)
	ctx = metadata.NewOutgoingContext(ctx, md)
	ctx = context.WithValue(ctx, models.UserIDCredentials{}, userID)

	return ctx, nil
}

func makeNewCtxByMetada(ctx context.Context, storage storage.Storage, md metadata.MD) (context.Context, error) {
	authHeader, ok := md["authorization"]
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "authorization token is not provided")
	}

	// Expecting: Bearer <token>
	tokenParts := strings.Split(authHeader[0], " ")
	if len(tokenParts) != 2 || strings.ToLower(tokenParts[0]) != "bearer" {
		return nil, status.Errorf(codes.Unauthenticated, "authorization header is malformed")
	}

	token := tokenParts[1]
	userID, err := authenticate(token, storage)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
	}

	// Add userID to the context
	ctx = context.WithValue(ctx, models.UserIDCredentials{}, userID)
	return ctx, nil
}

// UnaryServerInterceptor returns a new unary grpcServer interceptor for authentication.
func UnaryServerInterceptor(storage storage.Storage) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			ctx, err := makeNewCtxWithUserID(ctx, storage)
			if err != nil {
				return nil, err
			}

			return handler(ctx, req)
		}

		ctx, err := makeNewCtxByMetada(ctx, storage, md)
		if err != nil {
			return nil, err
		}

		return handler(ctx, req)
	}
}

func StreamServerInterceptor(storage storage.Storage) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			ctx, err := makeNewCtxWithUserID(ss.Context(), storage)
			if err != nil {
				return err
			}

			wrapped := grpc_middleware.WrapServerStream(ss)
			wrapped.WrappedContext = ctx
			return handler(srv, wrapped)
		}

		ctx, err := makeNewCtxByMetada(ss.Context(), storage, md)
		if err != nil {
			return err
		}

		wrapped := grpc_middleware.WrapServerStream(ss)
		wrapped.WrappedContext = ctx
		return handler(srv, wrapped)
	}
}

// Implement JWT authentication logic
func authenticate(tokenString string, storage storage.Storage) (int, error) {
	token, err := jwt.ParseWithClaims(tokenString, &serverapi.UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate the alg is what you expect
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// You should replace this secret key with your own secret key
		secretKey := []byte(jwtSecretKey)
		return secretKey, nil
	})

	if err != nil {
		return 0, err
	}

	if claims, ok := token.Claims.(*serverapi.UserClaims); ok && token.Valid {
		userID, err := strconv.Atoi(claims.UserID)
		if err != nil {
			return 0, err
		}

		// Check if the userID exists in the storage
		if !storage.IsItCorrectUserID(userID) {
			return 0, fmt.Errorf("userID does not exist")
		}

		return userID, nil
	}

	return 0, fmt.Errorf("invalid token")
}

func MakeAndRunServer(storage storage.Storage, configStore config.ConfigStore) {
	lis, err := net.Listen("tcp", configStore.FlagRunAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer(
		grpc.UnaryInterceptor(UnaryServerInterceptor(storage)),
		grpc.StreamInterceptor(StreamServerInterceptor(storage)),
	)
	pb.RegisterURLShortenerServiceServer(s, &grpcServer{storage: storage, configStore: configStore})
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
