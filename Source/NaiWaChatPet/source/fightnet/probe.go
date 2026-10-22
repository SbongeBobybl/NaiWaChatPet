package fightnet

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

func NormalizeAddress(address string) (string, error) {
	address = strings.TrimSpace(address)
	target := address
	for _, prefix := range []string{"tcp://", "tls://"} {
		target = strings.TrimPrefix(target, prefix)
	}
	host, port, err := net.SplitHostPort(target)
	number, parseErr := strconv.Atoi(port)
	if err != nil || parseErr != nil || host == "" || number < 1 || number > 65535 || len(address) > 255 || strings.ContainsAny(host, "/\\ \t\r\n") {
		return "", fmt.Errorf("invalid server address: use host:port or tls://host:port")
	}
	return address, nil
}

type ProbeResult struct {
	Version   int
	RoundTrip time.Duration
}

// Probe checks the actual fight protocol, not just whether a TCP port is open.
// It never creates a room and always releases its temporary connection.
func Probe(ctx context.Context, address string) (ProbeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	started := time.Now()
	conn, err := dialConnection(ctx, address)
	if err != nil {
		return ProbeResult{}, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	conn.SetDeadline(deadline)
	if err := json.NewEncoder(conn).Encode(Message{Version: ProtocolVersion, Type: "ping"}); err != nil {
		return ProbeResult{}, err
	}
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024), MaxMessageBytes)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return ProbeResult{}, err
		}
		return ProbeResult{}, fmt.Errorf("server closed the connection without a fight-protocol response")
	}
	var update Update
	if err := json.Unmarshal(scanner.Bytes(), &update); err != nil {
		return ProbeResult{}, fmt.Errorf("not a NaiWa fight server: %w", err)
	}
	if update.Version != ProtocolVersion {
		return ProbeResult{}, fmt.Errorf("protocol version mismatch: server %d, client %d", update.Version, ProtocolVersion)
	}
	if update.Type != "pong" {
		return ProbeResult{}, fmt.Errorf("server rejected connection test: %s", update.Error)
	}
	return ProbeResult{Version: update.Version, RoundTrip: time.Since(started)}, nil
}
