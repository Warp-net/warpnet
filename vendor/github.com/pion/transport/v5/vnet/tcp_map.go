// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"errors"
	"net"
	"sync"
)

var (
	errNoSuchTCPConn      = errors.New("no such TCPConn")
	errNoSuchTCPListener  = errors.New("no such TCPListener")
	errTCPConnAlreadyUsed = errors.New("tcp connection tuple already in use")
)

type tcpListenerMap struct {
	portMap map[int][]*TCPListener
	mutex   sync.RWMutex
}

func newTCPListenerMap() *tcpListenerMap {
	return &tcpListenerMap{portMap: map[int][]*TCPListener{}}
}

func (m *tcpListenerMap) insert(listener *TCPListener) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	addr := listener.Addr().(*net.TCPAddr) //nolint:forcetypeassert

	listeners, ok := m.portMap[addr.Port]
	if ok {
		if addr.IP.IsUnspecified() {
			return errAddressAlreadyInUse
		}
		for _, existing := range listeners {
			eaddr := existing.Addr().(*net.TCPAddr) //nolint:forcetypeassert
			if eaddr.IP.IsUnspecified() || eaddr.IP.Equal(addr.IP) {
				return errAddressAlreadyInUse
			}
		}
		listeners = append(listeners, listener)
	} else {
		listeners = []*TCPListener{listener}
	}

	m.portMap[addr.Port] = listeners

	return nil
}

func (m *tcpListenerMap) find(addr *net.TCPAddr) (*TCPListener, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	listeners, ok := m.portMap[addr.Port]
	if !ok {
		return nil, false
	}

	if addr.IP.IsUnspecified() {
		if len(listeners) == 0 {
			return nil, false
		}

		return listeners[0], true
	}

	for _, l := range listeners {
		eaddr := l.Addr().(*net.TCPAddr) //nolint:forcetypeassert
		if eaddr.IP.IsUnspecified() || eaddr.IP.Equal(addr.IP) {
			return l, true
		}
	}

	return nil, false
}

func (m *tcpListenerMap) delete(addr net.Addr) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	tcpAddr := addr.(*net.TCPAddr) //nolint:forcetypeassert
	listeners, ok := m.portMap[tcpAddr.Port]
	if !ok {
		return errNoSuchTCPListener
	}

	if tcpAddr.IP.IsUnspecified() {
		// insert() prevents any other listener from sharing a port with a
		// 0.0.0.0 listener, so there is exactly one entry for this port.
		// Deleting the whole port slot is therefore always correct here.
		delete(m.portMap, tcpAddr.Port)

		return nil
	}

	newListeners := make([]*TCPListener, 0, len(listeners))
	removed := false
	for _, l := range listeners {
		eaddr := l.Addr().(*net.TCPAddr) //nolint:forcetypeassert
		if eaddr.IP.Equal(tcpAddr.IP) {
			removed = true

			continue
		}
		newListeners = append(newListeners, l)
	}

	if !removed {
		return errNoSuchTCPListener
	}

	if len(newListeners) == 0 {
		delete(m.portMap, tcpAddr.Port)
	} else {
		m.portMap[tcpAddr.Port] = newListeners
	}

	return nil
}

type tcpConnMap struct {
	portMap map[int][]*TCPConn
	mutex   sync.RWMutex
}

func newTCPConnMap() *tcpConnMap {
	return &tcpConnMap{portMap: map[int][]*TCPConn{}}
}

func (m *tcpConnMap) insert(conn *TCPConn) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	laddr := conn.LocalAddr().(*net.TCPAddr)  //nolint:forcetypeassert
	raddr := conn.RemoteAddr().(*net.TCPAddr) //nolint:forcetypeassert

	conns := m.portMap[laddr.Port]
	for _, existing := range conns {
		eL := existing.LocalAddr().(*net.TCPAddr)  //nolint:forcetypeassert
		eR := existing.RemoteAddr().(*net.TCPAddr) //nolint:forcetypeassert
		if (eL.IP.IsUnspecified() || laddr.IP.IsUnspecified() || eL.IP.Equal(laddr.IP)) &&
			eR.IP.Equal(raddr.IP) && eR.Port == raddr.Port {
			return errTCPConnAlreadyUsed
		}
	}

	m.portMap[laddr.Port] = append(conns, conn)

	return nil
}

func (m *tcpConnMap) findByChunk(tcp *chunkTCP) (*TCPConn, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	dst := tcp.DestinationAddr().(*net.TCPAddr) //nolint:forcetypeassert
	src := tcp.SourceAddr().(*net.TCPAddr)      //nolint:forcetypeassert

	conns, ok := m.portMap[dst.Port]
	if !ok {
		return nil, false
	}

	for _, c := range conns {
		laddr := c.LocalAddr().(*net.TCPAddr)  //nolint:forcetypeassert
		raddr := c.RemoteAddr().(*net.TCPAddr) //nolint:forcetypeassert
		if (laddr.IP.IsUnspecified() || laddr.IP.Equal(dst.IP)) && raddr.IP.Equal(src.IP) && raddr.Port == src.Port {
			return c, true
		}
	}

	return nil, false
}

func (m *tcpConnMap) deleteConn(c *TCPConn) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	laddr := c.LocalAddr().(*net.TCPAddr) //nolint:forcetypeassert
	conns, ok := m.portMap[laddr.Port]
	if !ok {
		return errNoSuchTCPConn
	}

	newConns := []*TCPConn{}
	for _, existing := range conns {
		if existing == c {
			continue
		}
		newConns = append(newConns, existing)
	}

	if len(newConns) == len(conns) {
		return errNoSuchTCPConn
	}
	if len(newConns) == 0 {
		delete(m.portMap, laddr.Port)
	} else {
		m.portMap[laddr.Port] = newConns
	}

	return nil
}

func (m *tcpConnMap) deleteByAddr(laddr, raddr *net.TCPAddr) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	conns, ok := m.portMap[laddr.Port]
	if !ok {
		return errNoSuchTCPConn
	}

	newConns := make([]*TCPConn, 0, len(conns))
	for _, c := range conns {
		claddr := c.LocalAddr().(*net.TCPAddr)  //nolint:forcetypeassert
		craddr := c.RemoteAddr().(*net.TCPAddr) //nolint:forcetypeassert
		if claddr.IP.Equal(laddr.IP) && craddr.IP.Equal(raddr.IP) && craddr.Port == raddr.Port {
			continue // remove — full 4-tuple match
		}
		newConns = append(newConns, c)
	}

	if len(newConns) == len(conns) {
		return errNoSuchTCPConn
	}
	if len(newConns) == 0 {
		delete(m.portMap, laddr.Port)
	} else {
		m.portMap[laddr.Port] = newConns
	}

	return nil
}

func (m *tcpConnMap) portInUse(ip net.IP, port int) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	conns, ok := m.portMap[port]
	if !ok {
		return false
	}
	for _, c := range conns {
		laddr := c.LocalAddr().(*net.TCPAddr) //nolint:forcetypeassert
		if laddr.IP.IsUnspecified() || laddr.IP.Equal(ip) {
			return true
		}
	}

	return false
}
