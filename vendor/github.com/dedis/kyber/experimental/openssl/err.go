// +build experimental

package openssl

// #include <string.h>
// #include <openssl/err.h>
// #cgo CFLAGS: -Wno-deprecated
// #cgo LDFLAGS: -lcrypto
import "C"

import (
	"unsafe"
)

// Get the last OpenSSL error and convert it to a human-readable string
func getErrString() string {
	e := C.ERR_get_error()

	buf := [120]byte{} // max length specified in OpenSSL doc
	bufp := (*_Ctype_char)(unsafe.Pointer(&buf[0]))
	C.ERR_error_string_n(e, bufp, C.size_t(len(buf)))

	return string(buf[:C.strlen(bufp)])
}
