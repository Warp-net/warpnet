package network

import "sync"

var bytePool = sync.Pool{New: func() interface{} { return []byte(nil) }}

func allocBytes(size int) []byte {
	bs := bytePool.Get().([]byte)
	if cap(bs) < size {
		bs = make([]byte, size)
	}
	return bs[:size]
}

func freeBytes(bs []byte) {
	bytePool.Put(bs[:0]) //nolint:staticcheck
}

var trafficPool = sync.Pool{New: func() interface{} { return new(traffic) }}

func allocTraffic() *traffic {
	tr := trafficPool.Get().(*traffic)
	tr.payload = allocBytes(0)
	return tr
}

func freeTraffic(tr *traffic) {
	freeBytes(tr.payload)
	path := tr.path[:0]
	from := tr.from[:0]
	*tr = traffic{}
	tr.path = path
	tr.from = from
	trafficPool.Put(tr)
}
