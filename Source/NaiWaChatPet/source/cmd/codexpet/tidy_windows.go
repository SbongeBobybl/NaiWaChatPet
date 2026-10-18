//go:build windows

package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"codexpet/reconstruction/companion"
	"codexpet/reconstruction/tidy"
)

const (
	menuTidy   = 326
	menuUntidy = 327
	// tidyShow is how long the pet carries its box at the least, so even an
	// instant tidy reads as work being done.
	tidyShow = 3200 * time.Millisecond
)

var tidyAttributes = kernelDLL.NewProc("GetFileAttributesW")

// tidyJob is a tidy or an undo running in the background while the pet
// carries a box on screen.
type tidyJob struct {
	started time.Duration
	undo    bool
	done    chan tidyOutcome
	outcome *tidyOutcome
}

type tidyOutcome struct {
	report tidy.Report
	err    error
}

// desktopFolder is the user's desktop, wherever it has been redirected to.
func desktopFolder() (string, error) {
	var folder [32768]uint16
	// CSIDL_DESKTOPDIRECTORY, which follows OneDrive and other redirection.
	hr, _, _ := shellDLL.NewProc("SHGetFolderPathW").Call(0, 0x10, 0, 0, uintptr(unsafe.Pointer(&folder[0])))
	if e := shortcutError("SHGetFolderPathW", hr); e != nil {
		return "", e
	}
	desktop := syscall.UTF16ToString(folder[:])
	if desktop == "" {
		return "", errors.New("desktop folder unavailable")
	}
	return desktop, nil
}

func tidyJournals() (string, error) {
	dir, err := companion.ProfileDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tidy"), nil
}

// systemHidden reports files Explorer hides, such as desktop.ini.
func systemHidden(path string) bool {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return true
	}
	attributes, _, _ := tidyAttributes.Call(uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
	// FILE_ATTRIBUTE_HIDDEN | FILE_ATTRIBUTE_SYSTEM; all ones means failure.
	return uint32(attributes) == 0xFFFFFFFF || attributes&0x6 != 0
}

func (a *desktopHost) ask(title, question string) bool {
	t, q := utf(title), utf(question)
	// MB_YESNO | MB_ICONQUESTION; IDYES is 6.
	result, _ := call(messageBox, a.hwnd, uintptr(unsafe.Pointer(q)), uintptr(unsafe.Pointer(t)), 0x24)
	runtime.KeepAlive(t)
	runtime.KeepAlive(q)
	return result == 6
}

// startTidy plans a tidy of the desktop, asks once, then runs it.
func (a *desktopHost) startStandaloneLegacyTidy() {
	if a.trip != nil {
		a.stopTrip()
		a.say("好，送完手里这箱就停下。")
		return
	}
	if a.tidying != nil {
		a.say("正在整理中，马上就好。")
		return
	}
	desktop, err := desktopFolder()
	if tidyTarget != "" {
		desktop, err = tidyTarget, nil
	}
	if err == nil {
		var plan tidy.Plan
		if plan, err = tidy.Scan(desktop, tidy.Options{Keep: func(name string) bool { return name == petShortcut }, Hidden: systemHidden}); err == nil {
			if len(plan.Moves) == 0 {
				text := "桌面已经很整齐啦，没有需要整理的文件。"
				if s := skippedText(plan.Skipped); s != "" {
					text += "\n" + s
				}
				a.say(text)
				a.emote("proud", 4*time.Second)
				return
			}
			if !a.ask("一键整理桌面", tidyQuestion(plan)) {
				return
			}
			journals, err := tidyJournals()
			if err != nil {
				nativeMessage(a.hwnd, "一键整理桌面", "找不到保存撤销记录的位置，这次先不整理。", 0x30)
				return
			}
			// The pet carries the files by hand; without its rigs it plays
			// the filmed scene while they move.
			if a.play == nil || a.startTrip(plan, journals) != nil {
				a.runTidy(false, func() (tidy.Report, error) { return tidy.Apply(plan, journals) })
			}
			return
		}
	}
	_, _ = logDiagnostic(fmt.Errorf("tidy: %w", err))
	nativeMessage(a.hwnd, "一键整理桌面", "读取桌面失败："+err.Error(), 0x30)
}

// startUntidy puts the last tidy back, after saying how much will move.
func (a *desktopHost) startStandaloneUntidy() {
	if a.tidying != nil {
		a.say("正在整理中，等这次做完再撤销吧。")
		return
	}
	journals, err := tidyJournals()
	if err != nil {
		return
	}
	_, last, ok := tidy.Latest(journals)
	if !ok {
		a.say("还没有可以撤销的整理。")
		return
	}
	question := fmt.Sprintf("把 %s 整理的 %d 个文件放回桌面原来的位置？\n整理后被移走、改名或桌面上已有同名文件的，会保持现状。", last.Time.Format("1月2日 15:04"), len(last.Moves))
	if !a.ask("撤销上次整理", question) {
		return
	}
	a.runTidy(true, func() (tidy.Report, error) { return tidy.Undo(journals) })
}

func (a *desktopHost) runTidy(undo bool, work func() (tidy.Report, error)) {
	a.setControl(false)
	if a.chat != nil {
		a.chat.HideInput()
	}
	if a.emotions != nil {
		a.emotions.InterruptAnimation("tidy")
	}
	a.emote("organize", 30*time.Second)
	job := &tidyJob{started: a.now(), undo: undo, done: make(chan tidyOutcome, 1)}
	a.tidying = job
	go func() {
		defer func() {
			if p := recover(); p != nil {
				job.done <- tidyOutcome{err: fmt.Errorf("tidy stopped: %v", p)}
			}
		}()
		report, err := work()
		job.done <- tidyOutcome{report, err}
	}()
}

// stepTidy finishes a tidy once its files have moved and the pet has carried
// the box for long enough to be seen doing it.
func (a *desktopHost) stepTidy(now time.Duration) {
	job := a.tidying
	if job == nil {
		return
	}
	if job.outcome == nil {
		select {
		case o := <-job.done:
			job.outcome = &o
		default:
		}
	}
	if job.outcome == nil || now-job.started < tidyShow {
		return
	}
	a.tidying = nil
	if a.emotions != nil && a.emotions.Scheduling().Current == "organize" {
		a.emotions.ReturnToIdle()
	}
	o := job.outcome
	text := tidySummary(o.report, job.undo)
	if o.err != nil {
		_, _ = logDiagnostic(fmt.Errorf("tidy: %w", o.err))
		text += "\n" + o.err.Error()
	}
	a.say(text)
	if len(o.report.Moved) > 0 && !job.undo {
		a.emote("proud", 4*time.Second)
	}
}

// tidyMenu adds the desktop-tidying entries to the pet's menu.
func (a *desktopHost) tidyMenu(menu uintptr) {
	item := func(id uintptr, label string, flags uintptr) {
		text := utf(label)
		_, _ = call(appendMenu, menu, flags, id, uintptr(unsafe.Pointer(text)))
		runtime.KeepAlive(text)
	}
	if a.trip != nil {
		item(menuTidy, "停止整理（送完手里这箱）", 0)
		return
	}
	item(menuTidy, "桌面整理", 0)
}
