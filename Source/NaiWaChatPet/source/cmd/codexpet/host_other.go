//go:build !windows

package main

import "errors"

func runHost(*loadedAssets, hostOptions) error {
	return errors.New("the desktop host requires Windows")
}

func showFailure(error) {}
