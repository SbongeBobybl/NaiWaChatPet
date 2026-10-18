//go:build windows

package main

import (
	"codexpet/reconstruction/organizer"
	"codexpet/reconstruction/tidy"
	"context"
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

var organizerService *organizer.Server

type carryRequest struct {
	ctx      context.Context
	plan     tidy.Plan
	journals string
	progress func(organizer.Status)
	done     chan tidyOutcome
}

var carryShutdown = make(chan struct{})
var carryRequests = make(chan carryRequest, 1)

func carryConfirmed(ctx context.Context, plan tidy.Plan, journals string, progress func(organizer.Status)) (tidy.Report, error) {
	r := carryRequest{ctx, plan, journals, progress, make(chan tidyOutcome, 1)}
	select {
	case carryRequests <- r:
	case <-carryShutdown:
		return tidy.Report{}, errors.New("奶蛙已退出")
	case <-ctx.Done():
		return tidy.Report{}, ctx.Err()
	}
	// Once accepted, wait for the UI thread to close the journal, including cancellation.
	select {
	case o := <-r.done:
		return o.report, o.err
	case <-carryShutdown:
		return tidy.Report{}, errors.New("奶蛙已退出，未送达的文件保持原位")
	}
}

func (a *desktopHost) startStandaloneTidy() {
	if a.trip != nil {
		a.stopTrip()
		return
	}
	if a.tidying != nil {
		a.say("请等待当前整理结束。")
		return
	}
	if organizerService == nil {
		desktop, e := desktopFolder()
		if tidyTarget != "" {
			desktop, e = tidyTarget, nil
		}
		if e != nil {
			nativeMessage(a.hwnd, "整理桌面", e.Error(), 0x30)
			return
		}
		journals, e := tidyJournals()
		if e != nil {
			nativeMessage(a.hwnd, "整理桌面", e.Error(), 0x30)
			return
		}
		organizerService, e = organizer.New(organizer.Backend{Carry: carryConfirmed, Root: desktop, Journals: journals, Hidden: systemHidden, ReadLayout: readLayout, ApplyLayout: applyLayout, Model: func() any {
			if companionService == nil {
				return map[string]any{"Configured": false}
			}
			return companionService.OrganizerModel()
		}, Advise: func(ctx context.Context, h []tidy.Hint, consent bool) ([]tidy.Suggestion, error) {
			if companionService == nil {
				return nil, errors.New("模型服务尚未启动，可使用离线整理")
			}
			return companionService.AdviseOrganize(ctx, h, consent)
		}})
		if e != nil {
			nativeMessage(a.hwnd, "整理桌面", e.Error(), 0x30)
			return
		}
	}
	target := organizerService.URL()
	go func() {
		p, _ := syscall.UTF16PtrFromString(target)
		verb := utf("open")
		code, _, _ := shellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(p)), 0, 0, 1)
		runtime.KeepAlive(p)
		runtime.KeepAlive(verb)
		if code <= 32 {
			nativeMessage(0, "整理预览", "无法打开默认浏览器，请检查 Windows 浏览器关联。", 0x30)
		}
	}()
}
func (a *desktopHost) stepOrganizer() {
	a.stepExternalCarry()
	select {
	case r := <-carryRequests:
		if r.ctx.Err() != nil {
			r.done <- tidyOutcome{err: r.ctx.Err()}
			return
		}
		if a.trip != nil || a.tidying != nil || a.play == nil || a.play.stand == nil || a.play.class == nil || a.play.fight != nil {
			r.done <- tidyOutcome{err: errors.New("奶蛙暂时无法搬运，请结束当前动作后重试")}
			return
		}
		if e := a.startTrip(r.plan, r.journals); e != nil {
			r.done <- tidyOutcome{err: e}
			return
		}
		a.trip.ctx, a.trip.done, a.trip.progress = r.ctx, r.done, r.progress
		a.trip.total = len(r.plan.Moves)
	default:
	}
	if a.trip != nil && a.trip.ctx != nil && a.trip.ctx.Err() != nil {
		a.stopTrip()
	}
}
