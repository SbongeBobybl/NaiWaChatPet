package fightnet

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestLANCreateDiscoverAndJoinWithoutAddress(t *testing.T) {
	host, err := StartLANHost(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	owner := testClient(t, host.LocalAddress())
	owner.Send(Message{Type: "create"})
	welcome := waitUpdate(t, owner, func(u Update) bool { return u.Type == "welcome" })
	rooms, err := DiscoverLANRooms(context.Background(), 650*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	var found *LANRoom
	for i := range rooms {
		if rooms[i].Code == welcome.Room {
			found = &rooms[i]
			break
		}
	}
	if found == nil {
		t.Fatal("room not automatically discovered", rooms)
	}
	guest := testClient(t, found.Address)
	guest.Send(Message{Type: "join", Room: found.Code})
	joined := waitUpdate(t, guest, func(u Update) bool { return u.Type == "welcome" })
	if joined.Room != welcome.Room || joined.Player != 1 {
		t.Fatal("guest joined wrong room")
	}
	owner.Send(Message{Type: "ready"})
	guest.Send(Message{Type: "ready"})
	waitUpdate(t, guest, func(u Update) bool { return u.State == "countdown" })
	host.Close()
	select {
	case <-guest.Done():
	case <-time.After(time.Second):
		t.Fatal("host exit left guest connection open")
	}
}

// Do not let 127.0.0.1 or UDP make this test pass. Exercise the hotspot
// fallback over a real adapter's IPv4 address, then join its advertised room.
func TestLANHotspotFallbackWithoutBroadcastOrLoopback(t *testing.T) {
	var networks []*net.IPNet
	for _, network := range localIPv4Networks() {
		if network.IP.IsPrivate() && !network.IP.IsLoopback() {
			networks = append(networks, network)
			break
		}
	}
	if len(networks) == 0 {
		t.Skip("no active private IPv4 adapter; physical-adapter fallback cannot be checked")
	}
	host, err := StartLANHost(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	owner := testClient(t, host.LocalAddress())
	owner.Send(Message{Type: "create"})
	welcome := waitUpdate(t, owner, func(u Update) bool { return u.Type == "welcome" })
	rooms, err := discoverLANRooms(context.Background(), 900*time.Millisecond, networks, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 1 || rooms[0].Code != welcome.Room {
		t.Fatalf("TCP-only discovery failed: %v", rooms)
	}
	ip, _, err := net.SplitHostPort(rooms[0].Address)
	if err != nil || !net.ParseIP(ip).Equal(networks[0].IP) {
		t.Fatal("discovery did not use the selected real interface")
	}
	guest := testClient(t, rooms[0].Address)
	guest.Send(Message{Type: "join", Room: rooms[0].Code})
	joined := waitUpdate(t, guest, func(u Update) bool { return u.Type == "welcome" })
	if joined.Player != 1 || joined.Room != welcome.Room {
		t.Fatal("TCP-discovered room could not be joined")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := discoverLANRooms(ctx, time.Second, networks, false); err != context.Canceled {
		t.Fatal("discovery did not honor cancellation", err)
	}
}

func TestLANProbeTargetsAreBoundedAndUseEachInterface(t *testing.T) {
	var networks []*net.IPNet
	for _, value := range []string{"192.168.43.71/24", "172.20.10.3/28", "10.23.45.67/16"} {
		ip, network, _ := net.ParseCIDR(value)
		network.IP = ip.To4()
		networks = append(networks, network)
	}
	seen := map[string]bool{}
	targets := directDiscoveryTargets(networks)
	if len(targets) > 1024 {
		t.Fatal("unbounded subnet scan")
	}
	for _, target := range targets {
		seen[target.source.String()+"/"+target.target.String()] = true
		valid := false
		for _, network := range networks {
			if network.IP.Equal(target.source) && network.Contains(target.target) {
				valid = true
			}
		}
		if !valid {
			t.Fatal("probe escaped its selected local subnet", target)
		}
	}
	for last := 1; last < 255; last++ {
		if !seen["192.168.43.71/"+net.IPv4(192, 168, 43, byte(last)).String()] {
			t.Fatal("hotspot peer omitted", last)
		}
	}
	for _, gateway := range []string{"192.168.43.71/192.168.43.1", "172.20.10.3/172.20.10.1", "10.23.45.67/10.23.0.1"} {
		if !seen[gateway] {
			t.Fatal("hotspot/subnet gateway omitted", gateway)
		}
	}
}
