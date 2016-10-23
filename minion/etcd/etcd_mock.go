package etcd

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Mock implements a fake etcd.Store interface suitable for unit testing.  It
// more-or-less attempts to mimic the semantics of the Etcd backend, though it may not
// be perfect in that respect.  It has several notable limitations listed below:
//
// * The Watch() function is not yet implemented.
// * ttls are currently ignored.

// specifically that we don't respect ttl a the moment.
type mock struct {
	*sync.Mutex
	root        Tree
	expires     map[string]time.Time
	writes      *int
	reads       *int
	currentTime *time.Time
}

// NewMock creates a new mock etcd store for use of the unit tests.
func NewMock() Store {
	m := mock{writes: new(int), reads: new(int)}
	m.Mutex = &sync.Mutex{}
	m.root.Children = make(map[string]Tree)
	m.expires = map[string]time.Time{}
	now := time.Now()
	m.currentTime = &now
	return m
}

func newTestMock() mock {
	return NewMock().(mock)
}

func (m mock) Watch(path string, rateLimit time.Duration) chan struct{} {
	panic("Unimplemented")
}

func (m mock) Get(path string) (string, error) {
	m.Lock()
	defer m.Unlock()
	return m.get(path)
}

func (m mock) get(path string) (string, error) {
	m.expire()
	dir, node := dirPath(path)
	tree, err := m.getTree(dir)
	if err != nil {
		return "", err
	}

	result, ok := tree.Children[node]
	if !ok {
		return "", errors.New("undefined key")
	}

	return result.Value, nil
}

func (m mock) Create(path, value string, ttl time.Duration) error {
	m.Lock()
	defer m.Unlock()
	return m.create(path, value, ttl)
}

func (m mock) create(path, value string, ttl time.Duration) error {
	*m.writes++
	m.expire()
	tree, node, err := m.createPrefix(path)
	if err != nil {
		return err
	}

	if _, ok := tree.Children[node]; ok && !m.expired(path) {
		return errors.New("create: key exists")
	}

	tree.Children[node] = Tree{node, value, make(map[string]Tree)}
	m.setTimer(path, ttl)
	return nil
}

func (m mock) update(path, value string, ttl time.Duration) error {
	*m.writes++
	m.expire()
	tree, node, err := m.createPrefix(path)
	if err != nil {
		return err
	}

	if _, ok := tree.Children[node]; !ok {
		return errors.New("undefined key")
	}

	t := tree.Children[node]
	t.Value = value
	tree.Children[node] = t
	m.setTimer(path, ttl)
	return nil
}

func (m mock) Mkdir(dir string, ttl time.Duration) error {
	m.Lock()
	defer m.Unlock()
	return m.mkdir(dir, ttl)
}

func (m mock) mkdir(dir string, ttl time.Duration) error {
	if dir == "/" {
		return nil
	}

	if _, err := m.get(dir); err == nil {
		return errors.New("mkdir: key exists")
	}

	t := m.root
	for _, p := range splitPath(dir) {
		mp := t.Children
		if _, ok := mp[p]; !ok {
			mp[p] = Tree{p, "", make(map[string]Tree)}
		}

		t = mp[p]
	}

	m.setTimer(dir, ttl)
	return nil
}

func (m mock) GetTree(path string) (Tree, error) {
	m.Lock()
	defer m.Unlock()
	m.expire()
	return m.getTree(path)
}

func (m mock) getTree(path string) (Tree, error) {
	*m.reads++
	if path == "/" {
		return m.root, nil
	}

	t := m.root
	for _, dir := range splitPath(path) {
		var ok bool
		t, ok = t.Children[dir]
		if !ok {
			return Tree{}, errors.New("no such directory")
		}
	}

	return t, nil
}

func (m mock) Delete(path string) error {
	dir, node := dirPath(path)
	tree, err := m.getTree(dir)
	if err != nil {
		return err
	}

	if _, ok := tree.Children[node]; !ok {
		return errors.New("undefined key")
	}

	delete(tree.Children, node)
	delete(m.expires, path)
	return nil
}

func (m mock) Set(path, value string, ttl time.Duration) error {
	m.Lock()
	defer m.Unlock()

	if _, err := m.get(path); err != nil {
		return m.create(path, value, 0)
	}

	return m.update(path, value, ttl)
}

func (m mock) Refresh(path, value string, ttl time.Duration) error {
	m.Lock()
	defer m.Unlock()

	if m.expired(path) {
		return errors.New("path expired")
	}

	curValue, err := m.get(path)
	if err != nil {
		return fmt.Errorf("no such node '%s'", path)
	}

	if curValue != value {
		return fmt.Errorf("key '%s' does not have value '%s'", path, value)
	}

	return m.update(path, value, ttl)
}

func (m mock) RefreshDir(dir string, ttl time.Duration) error {
	return m.Refresh(dir, "", ttl)
}

func (m mock) expired(path string) bool {
	expireTime, ok := m.expires[path]
	return ok && !expireTime.IsZero() && m.now().After(expireTime)
}

func (m mock) setTimer(path string, ttl time.Duration) {
	if ttl <= 0 {
		m.expires[path] = time.Time{} // special case for no expiration
	} else {
		m.expires[path] = m.now().Add(ttl)
	}
}

func (m mock) expire() {
	for path := range m.expires {
		if m.expired(path) {
			m.Delete(path)
		}
	}
}

func (m mock) advanceTime(shift time.Duration) {
	*m.currentTime = (*m.currentTime).Add(shift)
}

func (m mock) now() time.Time {
	return *m.currentTime
}

func (m mock) createPrefix(path string) (Tree, string, error) {
	dir, node := dirPath(path)
	m.mkdir(dir, 0)

	tree, err := m.getTree(dir)
	return tree, node, err
}

func splitPath(path string) []string {
	if path[0] != '/' {
		// If this was real code, we should just return an error.  Since this is
		// just for the unit tests panicing is fine though.
		panic("invalid path")
	}

	// Since the path starts with "/" the first element is "", so we lop it off.
	return strings.Split(path, "/")[1:]
}

func dirPath(path string) (string, string) {
	slice := splitPath(path)
	return "/" + strings.Join(slice[:len(slice)-1], "/"), slice[len(slice)-1]
}
