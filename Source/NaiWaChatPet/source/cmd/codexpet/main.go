package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"codexpet/reconstruction/companion"
)

// The final bundle can provide this variable in a separate embed adapter.
// The initial embedded placeholder is blocked by the release gate. -assets
// provides a debugging override without changing the default embedded bundle.
var bundledAssets fs.FS
var showStartupDialog = true
var companionService *companion.Server

var animationDirectory string

var startMode string

// showOnLaunch has the pet perform its routine once it has said hello.
var showOnLaunch bool

// tidyTarget replaces the desktop as the folder to tidy, so the organizer can
// be tried on a copy; tidyOnLaunch starts a tidy shortly after start-up.
var (
	tidyTarget   string
	tidyOnLaunch bool
	// fightOnLaunch starts an arena bout in which the pet fights by itself.
	fightOnLaunch bool
)

type hostOptions struct{ Storage, PeerDir string }

func logDiagnostic(err error) (string, error) {
	cause := err
	dir, dirErr := companion.ProfileDirectory()
	if dirErr != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "logs")
	if startMode == "demo" {
		if exe, e := os.Executable(); e == nil {
			dir = filepath.Join(filepath.Dir(exe), "demo_logs")
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "diagnostic.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := fmt.Fprintln(f, cause)
	closeErr := f.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return path, nil
}

func startupHint(err error) string {
	if err == nil {
		return "\u7a0b\u5e8f\u672a\u80fd\u542f\u52a8\u3002"
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "asset") || strings.Contains(text, "runtime.json") || strings.Contains(text, "release gate"):
		return "\u7a0b\u5e8f\u8d44\u6e90\u6216\u914d\u7f6e\u68c0\u67e5\u672a\u901a\u8fc7\u3002"
	case strings.Contains(text, "received-file directory"):
		return "\u65e0\u6cd5\u521b\u5efa\u63a5\u6536\u6587\u4ef6\u5939\u3002"
	case strings.Contains(text, "peer heartbeat directory"):
		return "\u65e0\u6cd5\u521b\u5efa\u5b9e\u4f8b\u95f4\u901a\u4fe1\u76ee\u5f55\u3002"
	default:
		return "Windows \u7a97\u53e3\u6216\u52a8\u753b\u521d\u59cb\u5316\u672a\u5b8c\u6210\u3002"
	}
}

// traceStartup persists milestones even when the GUI subsystem has no console.
// No screenshots, API keys or private data are collected.
func traceStartup(stage string) {
	_, _ = logDiagnostic(fmt.Errorf("[%s] STARTUP_TRACE %s", time.Now().Format(time.RFC3339Nano), stage))
}

func main() {
	defer func() {
		if p := recover(); p != nil {
			e := fmt.Errorf("panic during startup: %v\n%s", p, debug.Stack())
			traceStartup("panic")
			_, _ = logDiagnostic(e)
			if showStartupDialog {
				showFailure(e)
			}
			os.Exit(1)
		}
	}()
	traceStartup("release v13 native conversation and emotion controller: process entered main")
	if err := entry(); err != nil {
		traceStartup("entry returned error: " + err.Error())
		fmt.Fprintln(os.Stderr, "Codex Pet:", err)
		if showStartupDialog {
			showFailure(err)
		}
		os.Exit(1)
	}
}
func entry() error {
	traceStartup("entry: parsing flags")
	assets := flag.String("assets", "", "directory containing released runtime.json and repaired assets")
	config := flag.String("config", "runtime.json", "asset-relative runtime configuration")
	storage := flag.String("storage", "", "received-file directory; default: ReceivedFiles beside the executable")
	peer := flag.String("peer-dir", "", "peer heartbeat directory; default: owned CodexPetReconstruction directory under the local temporary directory")
	flag.StringVar(&animationDirectory, "animations", "", "optional reviewed emotion animation directory")
	demo := flag.Bool("demo", startMode == "demo", "offline native chat demonstration; no model network traffic")
	check := flag.Bool("check", false, "validate assets/configuration without creating a window")
	renderCheck := flag.Bool("render-check", false, "validate a sample of every animation through premultiplied BGRA compositor")
	flag.BoolVar(&showOnLaunch, "play-demo", false, "perform the movement routine shortly after start-up")
	flag.StringVar(&tidyTarget, "desktop", "", "tidy this folder instead of the desktop, for trying the organizer on a copy")
	flag.BoolVar(&tidyOnLaunch, "tidy-demo", false, "start a tidy shortly after start-up")
	flag.BoolVar(&fightOnLaunch, "fight-demo", false, "start an arena bout the pet fights by itself")
	noShortcut := flag.Bool("no-shortcut", false, "leave the desktop shortcut alone, for test runs of a build outside the release folder")
	flag.Parse()
	if !*check && !*renderCheck {
		release, acquired, err := acquireDeskBoxPetInstance()
		if err != nil {
			return err
		}
		if !acquired {
			return nil
		}
		defer release()
	}
	showStartupDialog = !*check
	files := bundledAssets
	if *assets != "" {
		absolute, err := filepath.Abs(*assets)
		if err != nil {
			return err
		}
		files = os.DirFS(absolute)
	}
	if files == nil {
		return fmt.Errorf("no released asset bundle is available; use -assets with a directory whose release gate passed")
	}
	traceStartup("entry: before loadAssets")
	loaded, err := loadAssets(files, *config)
	if err != nil {
		return fmt.Errorf("asset validation failed: %w", err)
	}
	if *renderCheck {
		checked, err := renderPreflight(loaded)
		if err != nil {
			return fmt.Errorf("render-check failed: %w", err)
		}
		fmt.Printf("Render preflight passed: %d composited frames, 14 clip types; GUI not tested.\n", checked)
		return nil
	}
	if *check {
		traceStartup("entry: assets validated in -check")
		fmt.Printf("Asset/configuration checks passed: %d clips, %d x %d canvas. GUI playback has not been tested.\n", len(loaded.Counts), loaded.Canvas.Dx(), loaded.Canvas.Dy())
		return nil
	}
	traceStartup("entry: assets validated, starting render preflight")
	if _, err := renderPreflight(loaded); err != nil {
		return fmt.Errorf("pre-window render validation failed: %w", err)
	}
	traceStartup("entry: render preflight passed, starting GUI setup")
	if *storage == "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		*storage = filepath.Join(filepath.Dir(exe), "ReceivedFiles")
	}
	abs, err := filepath.Abs(*storage)
	if err != nil {
		return err
	}
	*storage = abs
	if *peer == "" {
		*peer = filepath.Join(os.TempDir(), "CodexPetReconstruction", "peers")
		if startMode == "demo" {
			exe, _ := os.Executable()
			*peer = filepath.Join(filepath.Dir(exe), "demo_peers")
		}
	}
	if !*noShortcut {
		if err := ensureDesktopShortcut(); err != nil {
			_, _ = logDiagnostic(err)
		}
	}
	// Local companion service never blocks core desktop-pet startup on failure.
	companionServer, companionErr := companion.Start(captureScreenJPEG)
	if companionErr != nil {
		_, _ = logDiagnostic(fmt.Errorf("optional DeepSeek companion unavailable: %w", companionErr))
	} else {
		companionService = companionServer
		if *demo {
			companionServer.EnableOfflineDemo()
		}
		configureCompanionCapture(companionServer)
		if closeAgent, err := startDeskBoxAgent(companionServer); err != nil {
			_, _ = logDiagnostic(fmt.Errorf("DeskBox agent unavailable: %w", err))
		} else {
			defer closeAgent()
		}
		defer func() { _ = companionServer.Close(); companionService = nil }()
	}
	traceStartup("entry: entering runHost")
	return runHost(loaded, hostOptions{*storage, *peer})
}
