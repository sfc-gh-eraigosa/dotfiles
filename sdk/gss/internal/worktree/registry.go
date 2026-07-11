package worktree

import (
	"fmt"
	"sort"
	"sync"
)

// Factory constructs a new Backend instance.
type Factory func() Backend

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// Register makes a backend available under name. Adding a backend is one
// new sub-package plus one Register call (typically from init()). It
// panics on a duplicate name — a programming error, surfaced at startup.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := factories[name]; dup {
		panic(fmt.Sprintf("worktree: backend %q already registered", name))
	}
	factories[name] = f
}

// Open returns a fresh instance of the named backend, or an error if no
// backend is registered under name.
func Open(name string) (Backend, error) {
	mu.RLock()
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("worktree: unknown backend %q (registered: %v)", name, Registered())
	}
	return f(), nil
}

// Registered returns the sorted list of registered backend names.
func Registered() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(factories))
	for n := range factories {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
