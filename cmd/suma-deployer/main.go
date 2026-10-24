package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ai-mada/suma/internal/deployer"
)

func main() {
	config := flag.String("config", "", "administrator local static-provider JSON configuration")
	flag.Parse()
	if *config == "" {
		log.Fatal("--config is required")
	}
	file, err := os.Open(*config)
	if err != nil {
		log.Fatal("provider configuration unavailable")
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var cfg deployer.Config
	err = decoder.Decode(&cfg)
	_ = file.Close()
	if err != nil {
		log.Fatal("invalid provider configuration")
	}
	if cfg.ManagementAddress == "" {
		cfg.ManagementAddress = "127.0.0.1:4350"
	}
	host, _, err := net.SplitHostPort(cfg.ManagementAddress)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		log.Fatal("management_address must be an explicit loopback address")
	}
	listener, err := net.Listen("tcp", cfg.ManagementAddress)
	if err != nil {
		log.Fatal("provider management listener unavailable")
	}
	provider, err := deployer.New(cfg)
	if err != nil {
		_ = listener.Close()
		log.Fatal(err)
	}
	defer provider.Close()
	server := &http.Server{Handler: provider.Handler(), ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	log.Printf("local static provider management listening on %s", listener.Addr())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
	case err = <-done:
		if err != http.ErrServerClosed {
			log.Print("provider management listener stopped")
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}
