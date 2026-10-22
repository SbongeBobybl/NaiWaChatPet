package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"codexpet/reconstruction/fightnet"
	"codexpet/reconstruction/play"
	"codexpet/reconstruction/rig"
)

func main() {
	version := flag.Bool("version", false, "print combat build and protocol version")
	address := flag.String("listen", ":28760", "TCP listen address")
	cert := flag.String("tls-cert", "", "TLS certificate PEM")
	key := flag.String("tls-key", "", "TLS private key PEM")
	maxRooms := flag.Int("max-rooms", 128, "maximum concurrent rooms")
	rulesPath := flag.String("rules", "", "optional combat JSON configuration")
	skillPath := flag.String("skill-assets", "", "optional original skill skeleton directory")
	flag.Parse()
	if *skillPath != "" {
		library, e := rig.LoadSkillAnimations(os.DirFS(*skillPath), "naiwa.json")
		if e == nil {
			e = rig.RegisterSkillAnimations(library)
		}
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
	}
	if *version {
		fmt.Printf("%s protocol %d\n", play.GameBuild, fightnet.ProtocolVersion)
		return
	}
	var listener net.Listener
	var err error
	if *cert != "" || *key != "" {
		var pair tls.Certificate
		pair, err = tls.LoadX509KeyPair(*cert, *key)
		if err == nil {
			listener, err = tls.Listen("tcp", *address, &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})
		}
	} else {
		listener, err = net.Listen("tcp", *address)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	config := fightnet.ServerConfig{MaxRooms: *maxRooms}
	if *rulesPath != "" {
		rules, e := play.ReadCombatRules(*rulesPath)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		config.Rules = &rules
	}
	server := fightnet.NewServer(config)
	fmt.Printf("%s listening on %s\n", server, listener.Addr())
	err = server.Serve(ctx, listener)
	server.Close()
	server.Wait()
	if err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
