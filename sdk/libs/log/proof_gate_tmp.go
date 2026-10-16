package log

import "os"

// proofWrite exists only to prove the Security Scan gate bites (security-scan-green
// #394, plan task 7): a deliberate world-readable write that gosec G306 must flag and
// the merge queue must hold. This PR is never merged.
func proofWrite(p string) error {
	return os.WriteFile(p, []byte("proof"), 0o644)
}
