package main

import (
	"errors"
	"log"
	"net"
	"net/http"

	"github.com/roostermade/allowance/internal/config"
	"github.com/roostermade/allowance/internal/db"
)

func main() {
	server, listener, cleanup, err := run()
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	log.Printf("RoosterMade Allowance is running on %s", listener.Addr().String())
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func run() (*http.Server, net.Listener, func(), error) {
	cfg := config.Load()

	conn, err := db.Open(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	mux := http.NewServeMux()
	server := &http.Server{Handler: mux}

	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		_ = conn.Close()
		return nil, nil, nil, err
	}

	cleanup := func() {
		_ = server.Close()
		_ = conn.Close()
	}

	return server, listener, cleanup, nil
}
