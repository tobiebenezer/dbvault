package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dbvault/dbvault/internal/application/controlplane"
	"github.com/dbvault/dbvault/internal/platform/controllerapi"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8081", "controller listen address")
	token := flag.String("token", os.Getenv("DBVAULT_CONTROLLER_TOKEN"), "controller bearer token")
	insecureDemo := flag.Bool("insecure-demo", false, "run without authentication (demos only; logs a loud warning)")
	tlsCert := flag.String("tls-cert", "", "server TLS certificate")
	tlsKey := flag.String("tls-key", "", "server TLS private key")
	clientCA := flag.String("client-ca", "", "client CA for mutual TLS")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if *token == "" && !*insecureDemo {
		fmt.Fprintln(os.Stderr, "refusing to start: controller token not configured")
		fmt.Fprintln(os.Stderr, "set --token (or DBVAULT_CONTROLLER_TOKEN); pass --insecure-demo only for local demos")
		os.Exit(2)
	}
	if *token == "" && *insecureDemo {
		logger.Warn("controller running WITHOUT authentication (--insecure-demo)")
	}
	server := &http.Server{Addr: *listen, Handler: controllerapi.New(controlplane.NewStore(), *token).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	if *tlsCert != "" || *tlsKey != "" {
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS13}
	}
	if *clientCA != "" {
		pool, err := certificatePool(*clientCA)
		if err != nil {
			logger.Error("controller.client_ca_failed", "error", err)
			os.Exit(1)
		}
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}
	}
	go func() {
		var err error
		if *tlsCert != "" || *tlsKey != "" {
			if *tlsCert == "" || *tlsKey == "" {
				err = fmt.Errorf("both --tls-cert and --tls-key are required")
			} else {
				err = server.ListenAndServeTLS(*tlsCert, *tlsKey)
			}
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			logger.Error("controller.http_failed", "error", err)
			os.Exit(1)
		}
	}()
	logger.Info("controller.started", "listen", *listen, "mutual_tls", *clientCA != "")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func certificatePool(path string) (*x509.CertPool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(b) {
		return nil, fmt.Errorf("no certificates found in %s", path)
	}
	return pool, nil
}
