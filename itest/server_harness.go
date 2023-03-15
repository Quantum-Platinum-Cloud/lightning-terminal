package itest

import (
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"sync"
	"time"

	"github.com/lightninglabs/loop/swapserverrpc"
	"github.com/lightninglabs/pool/auctioneerrpc"
	"github.com/lightningnetwork/lnd/cert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	// DefaultAutogenValidity is the default validity of a self-signed
	// certificate. The value corresponds to 14 months
	// (14 months * 30 days * 24 hours).
	DefaultAutogenValidity = 14 * 30 * 24 * time.Hour
)

type loopPoolServer struct {
	auctioneerrpc.UnimplementedChannelAuctioneerServer
	swapserverrpc.UnimplementedSwapServerServer
}

type serverHarness struct {
	serverHost string
	mockServer *grpc.Server

	certFile string
	server   *loopPoolServer

	errChan chan error

	wg sync.WaitGroup
}

func newServerHarness(serverHost string) *serverHarness {
	return &serverHarness{
		serverHost: serverHost,
		errChan:    make(chan error, 1),
	}
}

func (s *serverHarness) stop() {
	s.mockServer.Stop()
	s.wg.Wait()
}

func (s *serverHarness) start() error {
	tempDirName, err := ioutil.TempDir("", "litditest")
	if err != nil {
		return err
	}

	s.certFile = filepath.Join(tempDirName, "proxy.cert")
	keyFile := filepath.Join(tempDirName, "proxy.key")
	creds, err := genCertPair(s.certFile, keyFile)
	if err != nil {
		return err
	}

	httpListener, err := net.Listen("tcp", s.serverHost)
	if err != nil {
		return err
	}

	s.mockServer = grpc.NewServer(grpc.Creds(creds))
	s.server = &loopPoolServer{}

	auctioneerrpc.RegisterChannelAuctioneerServer(s.mockServer, s.server)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.errChan <- s.mockServer.Serve(httpListener)
	}()

	return nil
}

// genCertPair generates a pair of private key and certificate and returns them
// in different formats needed to spin up test servers and clients.
func genCertPair(certFile, keyFile string) (credentials.TransportCredentials,
	error) {

	certBytes, keyBytes, err := cert.GenCertPair(
		"itest autogenerated cert", nil, nil, false,
		DefaultAutogenValidity,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to generate cert pair: %v", err)
	}

	// Now that we have the certificate and key, we'll store them
	// to the file system.
	err = cert.WriteCertPair(certFile, keyFile, certBytes, keyBytes)
	if err != nil {
		return nil, fmt.Errorf("unable to write cert pair: %v", err)
	}

	creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("unable to load cert file: "+
			"%v", err)
	}
	return creds, nil
}
