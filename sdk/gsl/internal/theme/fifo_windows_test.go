//go:build windows

package theme_test

import "errors"

func createFIFO(path string) error {
	return errors.New("FIFOs not supported on Windows")
}
