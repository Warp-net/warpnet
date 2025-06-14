package raft

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var errVoterNotFound = errors.New("consensus: voter not found")

type votersCacher interface {
	addVoter(key ServerID, srv Server)
	getVoter(key ServerID) (_ Server, err error)
	removeVoter(key ServerID) error
	print()
	cap() int
}

type voterTimed struct {
	voter   Server
	addedAt time.Time
}

type votersCache struct {
	mutex *sync.RWMutex
	m     map[ServerID]voterTimed
}

// Voters cache helps to track voters, count them and prevent voters deletion from 'flapping'
func newVotersCache() *votersCache {
	pc := &votersCache{
		mutex: new(sync.RWMutex),
		m:     make(map[ServerID]voterTimed),
	}
	return pc
}

func (d *votersCache) addVoter(key ServerID, srv Server) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.m[key] = voterTimed{srv, time.Now()}
}

var errTooSoonToRemoveVoter = errors.New("consensus: too soon to remove voter")

func (d *votersCache) removeVoter(key ServerID) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	v, ok := d.m[key]
	if !ok {
		return nil
	}

	if time.Since(v.addedAt) < (time.Minute * 5) {
		return errTooSoonToRemoveVoter // flapping prevention
	}

	delete(d.m, key)
	return nil
}

func (d *votersCache) getVoter(key ServerID) (_ Server, err error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	v, ok := d.m[key]
	if !ok {
		return Server{}, errVoterNotFound
	}

	return v.voter, nil
}

func (d *votersCache) cap() int {
	d.mutex.RLock()
	capacity := len(d.m)
	d.mutex.RUnlock()
	return capacity
}

func (d *votersCache) print() {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	fmt.Println("consensus: voters list in cache:")
	for k := range d.m {
		fmt.Printf("========== %s", k)
	}
}
