# Arenalib – Simple Arena Memory Allocator for Go

Arenalib is a personal lightweight, practical memory allocator for Go that I use to make allocations faster for
short-lived objects by grouping them into large chunks. This reduces garbage collector (GC)
overhead and improves performance for workloads that create and discard many small objects.

##  Features
-  **Fast allocations** for byte buffers and POD (plain-old-data) types.
-  **Minimal API**: `Alloc`, `AllocAligned`, `AllocValue[T]`, `Reset`, `Release`, `Stats`.
-  **Thread safety**: Optional `ConcurrentArena` wrapper for safe concurrent use.
-  **Configurable** chunk size and zeroing behavior.
-  **Safety checks** to prevent unsafe pointer allocations.

##  Installation
```bash
go get github.com/soufianiso/arenalib
```

##  Quick Start
```go
package main

import (
    "fmt"
    "github.com/soufianiso/arenalib"
)

type Vec3 struct {
    X, Y, Z float32
}

func main() {
    // Create a new arena with 1 MiB chunks
    a := arena.New(arena.WithChunkSize(1 << 20))

    // Allocate a byte slice
    buf := a.Alloc(1024)
    fmt.Println("Buffer length:", len(buf))

    // Allocate a pointer-free struct
    v := a.AllocValue[Vec3]()
    v.X, v.Y, v.Z = 1, 2, 3
    fmt.Printf("Vec3: %+v\n", v)

    // Reuse memory without GC
    a.Reset()

    // Release all memory (eligible for GC)
    a.Release()
}
```

## 📊 Benchmarks
Run benchmarks to compare arena allocation to standard allocations:
```bash
go test -bench .
```
Use `benchstat` or `pprof` to analyze performance and GC activity for your workload.

## ⚠ Safety Notes
-  **Pointer-free types only**: `AllocValue` panics if the type contains Go pointers.
-  Do **not** use any previously allocated memory after calling `Reset` or `Release`.
-  This allocator reduces GC load but does **not** fully eliminate the garbage collector.

##  Roadmap
-  Additional helpers (e.g., `AllocString`).
- More performance tuning examples.


