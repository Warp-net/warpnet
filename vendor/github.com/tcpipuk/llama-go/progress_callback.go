package llama

/*
#include "wrapper.h"
#include <stdlib.h>
*/
import "C"
import "unsafe"

//export goProgressCallback
func goProgressCallback(progress C.float, userData unsafe.Pointer) C.bool {
	id := uintptr(userData)
	if cb, ok := progressCallbackRegistry.Load(id); ok {
		if callback, ok := cb.(ProgressCallback); ok {
			return C.bool(callback(float32(progress)))
		}
	}
	return C.bool(true) // Default: continue
}
