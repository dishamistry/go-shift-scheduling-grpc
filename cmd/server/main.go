// Command server starts the shift scheduling gRPC server.
//
// Usage:
//
//	go run ./cmd/server            # listens on localhost:50051
//	go run ./cmd/server -port 6000 # listens on localhost:6000
//
// Press Ctrl+C to stop the server gracefully.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"

	shiftv1 "github.com/dishamistry/go-shift-scheduling-grpc/gen/shift/v1"
	"github.com/dishamistry/go-shift-scheduling-grpc/internal/grpcserver"
	"github.com/dishamistry/go-shift-scheduling-grpc/internal/scheduling"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	port := flag.Int("port", 50051, "TCP port to listen on (localhost only)")
	flag.Parse()

	if err := run(*port); err != nil {
		log.Fatal(err)
	}
}

func run(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1 and 65535", port)
	}

	lis, err := net.Listen("tcp", net.JoinHostPort("localhost", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	svc := scheduling.NewService(scheduling.NewMemoryRepository())
	srv := grpc.NewServer()
	shiftv1.RegisterShiftServiceServer(srv, grpcserver.NewServer(svc))
	// Reflection lets tools such as grpcurl discover the API without the .proto file.
	reflection.Register(srv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		log.Println("shutting down")
		srv.GracefulStop()
	}()

	log.Printf("shift scheduling gRPC server listening on %s", lis.Addr())
	return srv.Serve(lis)
}
