package service

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/lightningnetwork/lnd/kvdb"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/metrics"
)

// Server is the main daemon construct for the Finality Provider server. It handles
// spinning up the RPC sever, the database, and any other components that the
// Taproot Asset server needs to function.
type Server struct {
	started int32

	cfg    *fpcfg.Config
	logger *zap.Logger

	rpcServer *rpcServer
	db        kvdb.Backend

	quit chan struct{}
}

// NewFinalityProviderServer creates a new server with the given config.
func NewFinalityProviderServer(cfg *fpcfg.Config, l *zap.Logger, fpa *FinalityProviderApp, db kvdb.Backend) *Server {
	return &Server{
		cfg:       cfg,
		logger:    l,
		rpcServer: newRPCServer(fpa),
		db:        db,
		quit:      make(chan struct{}, 1),
	}
}

// RunUntilShutdown runs the main EOTS manager server loop until a signal is
// received to shut down the process.
func (s *Server) RunUntilShutdown(ctx context.Context) error {
	if atomic.AddInt32(&s.started, 1) != 1 {
		return nil
	}

	// Start the metrics server.
	promAddr, err := s.cfg.Metrics.Address()
	if err != nil {
		return fmt.Errorf("failed to get prometheus address: %w", err)
	}
	metricsServer := metrics.Start(promAddr, s.logger)

	defer func() {
		s.logger.Info("Shutdown complete")
	}()

	defer func() {
		s.logger.Info("Closing database...")
		if err := s.db.Close(); err != nil {
			s.logger.Error(fmt.Sprintf("Failed to close database: %v", err)) // Log the error
		} else {
			s.logger.Info("Database closed")
		}
		metricsServer.Stop(ctx)
		s.logger.Info("Metrics server stopped")
	}()

	listenAddr := s.cfg.RPCListener
	// we create listeners from the RPCListeners defined
	// in the config.
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}
	defer func() {
		_ = lis.Close()
	}()

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	if err := s.rpcServer.RegisterWithGrpcServer(grpcServer); err != nil {
		return fmt.Errorf("failed to register gRPC server: %w", err)
	}

	// All the necessary parts have been registered, so we can
	// actually start listening for requests.
	s.startGrpcListen(grpcServer, []net.Listener{lis})

	s.logger.Info("Finality Provider Daemon is fully active!")

	// Wait for shutdown signal from either a graceful server stop or from
	// the interrupt handler.
	<-ctx.Done()

	return nil
}

// startGrpcListen starts the GRPC server on the passed listeners.
func (s *Server) startGrpcListen(grpcServer *grpc.Server, listeners []net.Listener) {
	// Use a WaitGroup, so we can be sure the instructions on how to input the
	// password is the last thing to be printed to the console.
	var wg sync.WaitGroup

	for _, lis := range listeners {
		wg.Add(1)
		go func(lis net.Listener) {
			s.logger.Info("RPC server listening", zap.String("address", lis.Addr().String()))

			// Close the ready chan to indicate we are listening.
			defer lis.Close()

			wg.Done()
			_ = grpcServer.Serve(lis)
		}(lis)
	}

	// Wait for gRPC servers to be up running.
	wg.Wait()
}
