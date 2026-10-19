//go:build windows

package video

import "os"

// tryLockFile always fails on Windows: there is no flock, and the refresher is
// not started there (SpawnRefresher refuses), so the segment stays hidden.
func tryLockFile(string) (*os.File, bool) { return nil, false }
