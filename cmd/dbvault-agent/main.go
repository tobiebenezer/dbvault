package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	controller := flag.String("controller", "http://127.0.0.1:8081", "controller endpoint")
	agentID := flag.String("agent-id", "agent-local", "agent id")
	organisation := flag.String("organisation", "org-local", "organisation id")
	project := flag.String("project", "project-local", "project id")
	environment := flag.String("environment", "development", "environment id")
	token := flag.String("token", os.Getenv("DBVAULT_CONTROLLER_TOKEN"), "controller token")
	interval := flag.Duration("heartbeat", 30*time.Second, "heartbeat interval")
	caFile := flag.String("ca", "", "controller CA certificate")
	certFile := flag.String("cert", "", "agent client certificate")
	keyFile := flag.String("key", "", "agent client key")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	transport, err := buildTransport(*caFile, *certFile, *keyFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: transport}
	headers := map[string]string{"Authorization": "Bearer " + *token, "X-DBVault-Organisation": *organisation, "X-DBVault-Project": *project, "X-DBVault-Environment": *environment, "Content-Type": "application/json"}
	registerBody := map[string]any{"id": *agentID, "name": *agentID, "status": "online", "version": "0.7.0", "protocol_version": 1}
	if err := post(ctx, client, *controller+"/v1/agents/register", headers, registerBody); err != nil {
		fmt.Fprintln(os.Stderr, "agent registration:", err)
	}
	sendHeartbeat := func() error {
		return post(ctx, client, *controller+"/v1/agents/heartbeat", headers, map[string]string{"agent_id": *agentID})
	}
	_ = sendHeartbeat()
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := sendHeartbeat(); err != nil {
				fmt.Fprintln(os.Stderr, "agent heartbeat:", err)
			}
		}
	}
}

func post(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, value any) error {
	body, _ := json.Marshal(value)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("request status %s", res.Status)
	}
	return nil
}

func buildTransport(caFile, certFile, keyFile string) (*http.Transport, error) {
	configuration := &tls.Config{MinVersion: tls.VersionTLS13}
	if caFile != "" {
		b, err := os.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(b) {
			return nil, fmt.Errorf("invalid CA file")
		}
		configuration.RootCAs = pool
	}
	if certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			return nil, fmt.Errorf("both --cert and --key are required")
		}
		certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		configuration.Certificates = []tls.Certificate{certificate}
	}
	return &http.Transport{TLSClientConfig: configuration, MaxIdleConns: 10, IdleConnTimeout: 30 * time.Second}, nil
}
