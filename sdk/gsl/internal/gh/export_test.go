package gh

// This file is compiled ONLY under `go test`, so it exposes internals to the
// external gh_test package without widening the real API surface.

// CacheFilePathForTest exposes cacheFilePath so tests can SEED the cache (and
// locate an entry) without hardcoding the on-disk key format. Assertions about
// cache CONTENT stay honest — only the path derivation is shared.
func CacheFilePathForTest(branch string) string { return cacheFilePath(branch) }
