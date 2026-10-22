package fightnet

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"codexpet/reconstruction/play"
)

const LANDiscoveryPort = 28761
const lanMagic = "NAIWA_FIGHT_LAN"

type LANRoom struct {
	Code    string `json:"code"`
	Players int    `json:"players"`
	Address string `json:"-"`
}

type lanPacket struct {
	Instance string   `json:"instance,omitempty"`
	Magic    string   `json:"magic"`
	Version  int      `json:"version"`
	Nonce    string   `json:"nonce"`
	Port     int      `json:"port,omitempty"`
	Room     *LANRoom `json:"room,omitempty"`
}

// LANHost runs the same authoritative server in the pet process. TCP uses a
// free port; the bounded UDP responder advertises waiting rooms on the LAN.
type LANHost struct {
	instance                 string
	server                   *Server
	listener                 net.Listener
	discovery                *net.UDPConn
	directDiscovery          net.Listener
	cancel                   context.CancelFunc
	serveDone, discoveryDone chan struct{}
	directDone               chan struct{}
	once                     sync.Once
}

func StartLANHost(parent context.Context, rules *play.CombatRules) (*LANHost, error) {
	ctx, cancel := context.WithCancel(parent)
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		cancel()
		return nil, fmt.Errorf("无法启动内置房间服务：%w", err)
	}
	discovery, err := net.ListenUDP("udp4", &net.UDPAddr{Port: LANDiscoveryPort})
	if err != nil {
		listener.Close()
		cancel()
		return nil, fmt.Errorf("局域网发现端口正被占用，请关闭另一份房主程序后重试：%w", err)
	}
	direct, err := net.Listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(LANDiscoveryPort)))
	if err != nil {
		discovery.Close()
		listener.Close()
		cancel()
		return nil, fmt.Errorf("热点兼容发现端口正被占用，请关闭另一份房主程序后重试：%w", err)
	}
	var ownedRules *play.CombatRules
	if rules != nil {
		copy := *rules
		copy.Skills = append([]play.SkillDefinition(nil), rules.Skills...)
		ownedRules = &copy
	}
	h := &LANHost{instance: randomID(8), server: NewServer(ServerConfig{Rules: ownedRules, MaxRooms: 1, MaxConnections: 8}), listener: listener, discovery: discovery, directDiscovery: direct, cancel: cancel, serveDone: make(chan struct{}), discoveryDone: make(chan struct{}), directDone: make(chan struct{})}
	go func() { defer close(h.serveDone); _ = h.server.Serve(ctx, listener) }()
	go func() { defer close(h.discoveryDone); h.respond(ctx) }()
	go func() { defer close(h.directDone); h.respondDirect(ctx) }()
	go func() { <-ctx.Done(); h.Close() }()
	return h, nil
}

func (h *LANHost) LocalAddress() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(h.listener.Addr().(*net.TCPAddr).Port))
}

func (h *LANHost) Close() {
	if h == nil {
		return
	}
	h.once.Do(func() {
		h.cancel()
		h.discovery.Close()
		h.directDiscovery.Close()
		h.listener.Close()
		h.server.Close()
		<-h.serveDone
		h.server.Wait()
		<-h.discoveryDone
		<-h.directDone
	})
}

func validRoomCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, r := range code {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

func localIPv4Networks() []*net.IPNet {
	var networks []*net.IPNet
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			if network, ok := address.(*net.IPNet); ok && network.IP.To4() != nil && !network.IP.IsLinkLocalUnicast() {
				networks = append(networks, &net.IPNet{IP: network.IP.To4(), Mask: network.Mask})
			}
		}
	}
	return networks
}

func onLocalNetwork(ip net.IP, networks []*net.IPNet) bool {
	if ip.IsLoopback() {
		return true
	}
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (h *LANHost) waitingRooms() []LANRoom {
	h.server.mu.Lock()
	defer h.server.mu.Unlock()
	var rooms []LANRoom
	for _, r := range h.server.rooms {
		if r.state != "waiting" {
			continue
		}
		players := 0
		for _, slot := range r.slots {
			if slot != nil {
				players++
			}
		}
		if players < 2 {
			rooms = append(rooms, LANRoom{Code: r.code, Players: players})
		}
		if len(rooms) >= 8 {
			break
		}
	}
	return rooms
}

func (h *LANHost) respond(ctx context.Context) {
	buf := make([]byte, 1025)
	networks := localIPv4Networks()
	updated := time.Now()
	window := time.Now()
	responses := 0
	for {
		n, source, err := h.discovery.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
		now := time.Now()
		if now.Sub(updated) > 20*time.Second {
			networks = localIPv4Networks()
			updated = now
		}
		if !onLocalNetwork(source.IP, networks) || n > 1024 {
			continue
		}
		var request lanPacket
		if json.Unmarshal(buf[:n], &request) != nil || !validLANQuery(request) {
			continue
		}
		if now.Sub(window) >= time.Second {
			window = now
			responses = 0
		}
		for _, room := range h.waitingRooms() {
			if responses >= 32 {
				break
			}
			packet := h.advertisement(request.Nonce, room)
			data, _ := json.Marshal(packet)
			h.discovery.SetWriteDeadline(now.Add(100 * time.Millisecond))
			h.discovery.WriteToUDP(data, source)
			responses++
		}
	}
}
