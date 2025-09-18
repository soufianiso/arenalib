package arena

import (
	"reflect"
	"sync"
	"unsafe"
)

const defaultChunkSize = 1 << 20 // 1MiB

// Option configures Arena on creation.
type Option func(*Arena)

// WithChunkSize sets the arena's backing chunk size (must be > 0).
func WithChunkSize(sz int) Option {
	return func(a *Arena) {
		if sz > 0 {
			a.chunkSize = sz
		}
	}
}

func WithZeroOnAlloc(z bool) Option {
	return func(a *Arena) {
		a.zeroOnAlloc = z
	}
}

type Arena struct {
	chunkSize   int
	zeroOnAlloc bool

	chunks [][]byte 
	off    int      
}

// New creates a new Arena with optional configuration.
func New(opts ...Option) *Arena {
	a := &Arena{
		chunkSize:   defaultChunkSize,
		zeroOnAlloc: true,
		chunks:      make([][]byte, 0, 4),
		off:         0,
	}
	for _, o := range opts {
		o(a)
	}
	if a.chunkSize <= 0 {
		a.chunkSize = defaultChunkSize
	}
	a.chunks = append(a.chunks, make([]byte, a.chunkSize))
	return a
}

func (a *Arena) Alloc(n int) []byte {
	return a.AllocAligned(n, 8)
}

func (a *Arena) AllocAligned(n int, align int) []byte {
	if n <= 0 {
		return nil
	}
	if align <= 0 {
		align = 8
	}
	// ensure power-of-two alignment: if not, fall back to 8
	if (align & (align - 1)) != 0 {
		align = 8
	}

	last := a.chunks[len(a.chunks)-1]
	// compute aligned offset
	off := a.off
	pad := (align - (off & (align - 1))) & (align - 1)
	off += pad

	if off+n <= len(last) {
		res := last[off : off+n]
		if a.zeroOnAlloc {
			zero(res)
		}
		a.off = off + n
		return res
	}

	// not enough room in current chunk; allocate a new chunk sized for n or chunkSize
	newSize := a.chunkSize
	if n+align > newSize {
		newSize = n + align
	}
	buf := make([]byte, newSize)
	a.chunks = append(a.chunks, buf)
	off = 0
	pad = (align - (off & (align - 1))) & (align - 1)
	off += pad
	res := buf[off : off+n]
	// buf is freshly allocated and therefore zeroed by Go; but keep consistent semantics
	if a.zeroOnAlloc {
		// already zeroed but keep for clarity; no-op in practice
	}
	a.off = off + n
	return res
}

// AllocValue allocates space for a typed value of type T inside the arena and returns *T.
// T must not contain pointers (POD). If T contains pointers, AllocValue will panic.
func (a *Arena) AllocValue[T any]() *T {
	var zeroT *T
	typ := reflect.TypeOf(zeroT).Elem()
	if containsPointers(typ) {
		panic("arena: AllocValue called for a type that contains Go pointers; allocate with new(T) instead")
	}
	sz := int(typ.Size())
	if sz == 0 {
		// weird zero-sized types: allocate a 1-byte region and return a pointer into it
		b := a.Alloc(1)
		return (*T)(unsafe.Pointer(&b[0]))
	}
	align := typ.Align()
	mem := a.AllocAligned(sz, align)
	return (*T)(unsafe.Pointer(&mem[0]))
}

func (a *Arena) Reset() {
	if len(a.chunks) == 0 {
		a.chunks = append(a.chunks, make([]byte, a.chunkSize))
		a.off = 0
		return
	}
	// Optionally zero the used portion of the first chunk to avoid leaking contents
	if a.zeroOnAlloc && a.off > 0 {
		zero(a.chunks[0][:a.off])
	}
	// drop other chunks so they can be GC'd
	for i := 1; i < len(a.chunks); i++ {
		a.chunks[i] = nil
	}
	a.chunks = a.chunks[:1]
	a.off = 0
}

func (a *Arena) Release() {
	for i := range a.chunks {
		a.chunks[i] = nil
	}
	a.chunks = nil
	a.off = 0
}

func (a *Arena) Stats() (used int, capacity int) {
	if len(a.chunks) == 0 {
		return 0, 0
	}
	used = 0
	for i, ch := range a.chunks {
		if i == len(a.chunks)-1 {
			used += a.off
		} else {
			used += len(ch)
		}
		capacity += len(ch)
	}
	return
}

type ConcurrentArena struct {
	mu sync.Mutex
	a  *Arena
}

func NewConcurrent(opts ...Option) *ConcurrentArena {
	return &ConcurrentArena{a: New(opts...)}
}

func (c *ConcurrentArena) Alloc(n int) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.a.Alloc(n)
}

func (c *ConcurrentArena) AllocAligned(n int, align int) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.a.AllocAligned(n, align)
}

func (c *ConcurrentArena) AllocValue[T any]() *T {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.a.AllocValue[T]()
}

func (c *ConcurrentArena) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.a.Reset()
}

func (c *ConcurrentArena) Release() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.a.Release()
}

func (c *ConcurrentArena) Stats() (used int, capacity int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.a.Stats()
}

// ------------------ helpers ------------------

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// containsPointers returns true if t (recursively) contains any Go pointers
// (ptr, slice, map, chan, func, interface, string, unsafe.Pointer).
// It is conservative but prevents unsafe use of AllocValue for pointerful types.
func containsPointers(t reflect.Type) bool {
	visited := make(map[reflect.Type]bool)
	return containsPointersRec(t, visited)
}

func containsPointersRec(t reflect.Type, visited map[reflect.Type]bool) bool {
	if t == nil {
		return false
	}
	if visited[t] {
		return false
	}
	visited[t] = true

	switch t.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func, reflect.Interface, reflect.UnsafePointer, reflect.String:
		return true
	case reflect.Array:
		return containsPointersRec(t.Elem(), visited)
	case reflect.Struct:
		// check fields
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			// skip unexported field? No — even unexported may contain pointers; check anyway
			if containsPointersRec(f.Type, visited) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
