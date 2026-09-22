// Package siteregistry provides a global registry for site modules.
// Sites register themselves via init() functions in their packages.
package siteregistry

import (
	"sync"

	"akyuu/internal/site"
)

var (
	mu    sync.RWMutex
	sites = make(map[string]site.Site)
	names []string
)

// Register adds a site to the global registry.
// Called by site modules in their init() function.
func Register(s site.Site) {
	mu.Lock()
	defer mu.Unlock()
	name := s.GetName()
	if _, exists := sites[name]; exists {
		panic("site: duplicate registration for " + name)
	}
	sites[name] = s
	names = append(names, name)
}

// Get returns a site by name, or nil if not found.
func Get(name string) site.Site {
	mu.RLock()
	defer mu.RUnlock()
	return sites[name]
}

// All returns all registered sites.
func All() []site.Site {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]site.Site, 0, len(names))
	for _, n := range names {
		out = append(out, sites[n])
	}
	return out
}

// Names returns all registered site names.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, len(names))
	copy(out, names)
	return out
}

// Count returns the number of registered sites.
func Count() int {
	mu.RLock()
	defer mu.RUnlock()
	return len(sites)
}

// Clear removes all registered sites (for testing).
func Clear() {
	mu.Lock()
	defer mu.Unlock()
	sites = make(map[string]site.Site)
	names = nil
}
