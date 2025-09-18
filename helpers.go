package arena

import "reflect"

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
