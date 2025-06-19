package binding

/*
#cgo CFLAGS: DLOG_DISABLE_LOGS=1
#cgo LDFLAGS: -L. -lbinding -lstdc++ -lm
#cgo CXXFLAGS: -I. -I./llama.cpp -Wno-format -Wno-delete-incomplete -w -DLOG_DISABLE_LOGS=1
#include "binding.h"
*/
import "C"
