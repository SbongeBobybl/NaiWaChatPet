package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func acquireDeskBoxPetInstance() (func(), bool, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, false, err
	}
	hash := sha256.Sum256([]byte(user.User.Sid.String()))
	name, err := windows.UTF16PtrFromString(fmt.Sprintf(`Local\NaiWa.DeskBox.Pet.%x`, hash[:12]))
	if err != nil {
		return nil, false, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return func() { windows.CloseHandle(handle) }, true, nil
}

func (a *desktopHost) openDeskBoxPreview() {
	// Opening the organizer must not leave a reply or its separate skin window.
	if a.chat != nil {
		a.chat.hideReply()
	}
	exe, err := os.Executable()
	if err != nil {
		a.say("无法定位 DeskBox，请从整合版入口启动。")
		return
	}
	host := deskBoxExecutable(exe, os.Getenv("NAIWA_DESKBOX_EXE"))
	host, err = filepath.Abs(host)
	if err != nil || !deskBoxFileExists(host) {
		a.say("未找到整理组件，请保留奶蛙旁的 Organizer 文件夹。")
		return
	}
	command := exec.Command(host, "--ai-organize")
	command.Dir = filepath.Dir(host)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err = command.Start(); err != nil {
		a.say("整理预览暂时无法打开。")
		return
	}
	go command.Wait()
}

// A shipped bundle always owns its organizer. Overrides are only a development
// fallback for a standalone pet executable without an Organizer directory.
func deskBoxExecutable(exe, developmentOverride string) string {
	bundled := filepath.Join(filepath.Dir(exe), "Organizer", "DeskBox.exe")
	if _, err := os.Stat(filepath.Dir(bundled)); err == nil || developmentOverride == "" {
		return bundled
	}
	return developmentOverride
}
func deskBoxFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
func (a *desktopHost) startTidy() {
	if a.trip != nil {
		a.stopTrip()
		return
	}
	a.openDeskBoxPreview()
}
func (a *desktopHost) startLegacyTidy() { a.openDeskBoxPreview() }
func (a *desktopHost) startUntidy()     { a.openDeskBoxPreview() }
