//go:build !windows

package main

import "errors"

func captureScreenJPEG() ([]byte, error) { return nil, errors.New("屏幕观察仅支持 Windows") }
