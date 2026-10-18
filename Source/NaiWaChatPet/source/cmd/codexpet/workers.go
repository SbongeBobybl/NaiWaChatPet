package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"codexpet/reconstruction/safestore"
)

type transferBatch struct{ Paths []string }
type transferProblem struct {
	Source                 string
	Target                 string
	Stage                  string
	Detail                 string
	TargetVerified         bool
	SourceRemoved          bool
	SourceRemovalUncertain bool
}
type transferReport struct {
	Stored         int
	SourceRetained int
	Targets        []string
	Problems       []transferProblem
}
type peerRecord struct {
	Protocol string   `json:"protocol"`
	Owner    string   `json:"owner"`
	PID      int      `json:"pid"`
	StampNS  int64    `json:"stamp_ns"`
	Rect     rectJSON `json:"rect"`
}

var peerOwnerPattern = regexp.MustCompile(`^[0-9]+-[a-f0-9]{24}$`)

const peerProtocol = "codexpet-reconstruction-peer-v1"

type hostWorkers struct {
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	transfers chan transferBatch
	results   chan transferReport
	poses     chan rectJSON
	peers     chan []rectJSON
	problems  chan error
}

func newWorkers(options hostOptions) (*hostWorkers, error) {
	if err := os.MkdirAll(options.Storage, 0700); err != nil {
		return nil, fmt.Errorf("received-file directory: %w", err)
	}
	if err := os.MkdirAll(options.PeerDir, 0700); err != nil {
		return nil, fmt.Errorf("peer heartbeat directory: %w", err)
	}
	var token [12]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, err
	}
	owner := fmt.Sprintf("%d-%s", os.Getpid(), hex.EncodeToString(token[:]))
	ctx, cancel := context.WithCancel(context.Background())
	w := &hostWorkers{ctx: ctx, cancel: cancel, transfers: make(chan transferBatch, 16), results: make(chan transferReport, 16), poses: make(chan rectJSON, 1), peers: make(chan []rectJSON, 1), problems: make(chan error, 1)}
	w.wg.Add(2)
	go func() { defer w.wg.Done(); w.transferLoop(options.Storage) }()
	go func() { defer w.wg.Done(); w.peerLoop(options.PeerDir, owner) }()
	return w, nil
}
func (w *hostWorkers) close() { w.cancel(); w.wg.Wait() }
func (w *hostWorkers) transferLoop(storage string) {
	for {
		select {
		case <-w.ctx.Done():
			return
		case batch := <-w.transfers:
			report := transferReport{}
			for _, source := range batch.Paths {
				if err := w.ctx.Err(); err != nil {
					return
				}
				target, err := uniqueTarget(storage, source, time.Now())
				if err != nil {
					report.Problems = append(report.Problems, transferProblem{Source: source, Stage: "target-name", Detail: err.Error()})
					continue
				}
				result, err := safestore.Transfer(w.ctx, source, target)
				if result.TargetVerified {
					report.Stored++
					if result.SourceRetainedForStreams {
						report.SourceRetained++
					}
					report.Targets = append(report.Targets, target)
				}
				if err != nil {
					problem := transferProblem{Source: source, Target: target, Detail: err.Error(), TargetVerified: result.TargetVerified, SourceRemoved: result.SourceRemoved, SourceRemovalUncertain: result.SourceRemovalUncertain}
					var transferErr *safestore.TransferError
					if errors.As(err, &transferErr) {
						problem.Stage = transferErr.Stage
					}
					report.Problems = append(report.Problems, problem)
				} else if !result.TargetVerified || (!result.SourceRemoved && !result.SourceRetainedForStreams) {
					report.Problems = append(report.Problems, transferProblem{Source: source, Target: target, Stage: "verification", Detail: "transfer returned without confirming destination verification and source removal", TargetVerified: result.TargetVerified, SourceRemoved: result.SourceRemoved, SourceRemovalUncertain: result.SourceRemovalUncertain})
				}
			}
			select {
			case w.results <- report:
			case <-w.ctx.Done():
				return
			}
		}
	}
}
func replaceLatest[T any](channel chan T, value T) {
	select {
	case channel <- value:
	default:
		select {
		case <-channel:
		default:
		}
		select {
		case channel <- value:
		default:
		}
	}
}
func (w *hostWorkers) problem(err error) {
	select {
	case w.problems <- err:
	default:
	}
}
func (w *hostWorkers) peerLoop(dir, owner string) {
	ownPath := filepath.Join(dir, owner+".json")
	temporary := ownPath + ".tmp"
	defer os.Remove(temporary)
	defer os.Remove(ownPath)
	writes := time.NewTicker(200 * time.Millisecond)
	defer writes.Stop()
	checks := time.NewTicker(140 * time.Millisecond)
	defer checks.Stop()
	var current rectJSON
	write := func() {
		if !current.valid() {
			return
		}
		record := peerRecord{peerProtocol, owner, os.Getpid(), time.Now().UnixNano(), current}
		b, err := json.Marshal(record)
		if err == nil {
			err = os.WriteFile(temporary, b, 0600)
		}
		if err == nil {
			err = os.Rename(temporary, ownPath)
		}
		if err != nil {
			w.problem(fmt.Errorf("peer heartbeat: %w", err))
		}
	}
	for {
		select {
		case <-w.ctx.Done():
			return
		case current = <-w.poses:
		case <-writes.C:
			write()
		case <-checks.C:
			peers, err := readPeers(dir, owner, time.Now())
			if err != nil {
				w.problem(err)
				continue
			}
			replaceLatest(w.peers, peers)
		}
	}
}
func readPeers(dir, owner string, now time.Time) ([]rectJSON, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("peer lookup: %w", err)
	}
	var peers []rectJSON
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		other := strings.TrimSuffix(name, ".json")
		if other == owner || !peerOwnerPattern.MatchString(other) {
			continue
		}
		// The directory is owned by this host. Ignore links and unknown protocol data.
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
			continue
		}
		file, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		b, readErr := io.ReadAll(io.LimitReader(file, 4097))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(b) > 4096 {
			continue
		}
		var p peerRecord
		if json.Unmarshal(b, &p) != nil || p.Protocol != peerProtocol || p.Owner != other || p.PID <= 0 || !p.Rect.valid() {
			continue
		}
		stamp := time.Unix(0, p.StampNS)
		age := now.Sub(stamp)
		if age < 0 || age > 5*time.Second {
			continue
		}
		peers = append(peers, p.Rect)
	}
	return peers, nil
}
func (w *hostWorkers) submit(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if len(paths) > 1024 {
		return errors.New("at most 1024 files can be accepted in one drop")
	}
	copyPaths := append([]string(nil), paths...)
	select {
	case w.transfers <- transferBatch{copyPaths}:
		return nil
	case <-w.ctx.Done():
		return errors.New("file worker is closing")
	default:
		return errors.New("file queue is full; these files were not transferred")
	}
}
