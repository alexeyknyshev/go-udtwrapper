package udt

import (
	"io"
	"unsafe"
	"fmt"
	"time"
)

// #cgo linux CFLAGS: -DLINUX
// #cgo darwin CFLAGS: -DOSX
// #cgo freebsd CFLAGS: -DBSD
// #cgo windows CXXFLAGS: -DWIN32 -DUDT_EXPORTS
// #cgo windows LDFLAGS: -static-libgcc -static-libstdc++ -static -lkernel32 -luser32 -lws2_32
// #cgo i386 CFLAGS: -DIA32
// #cgo amd64 CFLAGS: -DAMD64
// #cgo CFLAGS: -g -Wall -finline-functions -O3 -fno-strict-aliasing -fvisibility=hidden
// #cgo LDFLAGS: -lstdc++ -lm
// #include "udt_c.h"
// #include <errno.h>
// #include <string.h>
import "C"

func slice2cbuf(buf []byte) *C.char {
	return (*C.char)(unsafe.Pointer(&buf[0]))
}

func logf(format string, v ...interface{}) {
	format = time.Now().Format("15:04:05.000000") + " :: " + format + "\n"
	fmt.Printf(format, v...)
}

// udtIOError interprets the udt_getlasterror_code and returns an
// error if IO systems should stop.
func (fd *udtFD) udtIOError() error {
	ec := C.udt_getlasterror_code()
	logf("udtIOError: last error code", ec)
	switch ec {
	case C.UDT_SUCCESS: // success :)
		return io.EOF
	default: // unexpected error, bail
		return lastError()
	}
}

func (fd *udtFD) Read(buf []byte) (int, error) {
	logf("Read to udtFD")
	n := int(C.udt_recv(fd.sock, slice2cbuf(buf), C.int(len(buf)), 0))
	if C.int(n) == C.ERROR {
		// got problems?
		err := fd.udtIOError()
		logf("Read err: %v", err)
		return 0, err
	}
	logf("Read %d bytes| text: %s\n", n, string(buf))
	return n, nil
}

func (fd *udtFD) Write(buf []byte) (writecnt int, err error) {
	logf("Write to udtFD")
	for len(buf) > writecnt {
		n, err := fd.write(buf[writecnt:])
		if err != nil {
			logf("Write err:", err)
			return writecnt, err
		}

		writecnt += n
	}
	logf("Wrote %d bytes| text: %s\n", writecnt, string(buf))
	return writecnt, nil
}

func (fd *udtFD) write(buf []byte) (int, error) {
	n := int(C.udt_send(fd.sock, slice2cbuf(buf), C.int(len(buf)), 0))
	if C.int(n) == C.ERROR {
		// UDT Error?
		return 0, fd.udtIOError()
	}

	return n, nil
}

type socketStatus C.enum_UDTSTATUS

func getSocketStatus(sock C.UDTSOCKET) socketStatus {
	return socketStatus(C.udt_getsockstate(sock))
}

func (s socketStatus) inSetup() bool {
	switch C.enum_UDTSTATUS(s) {
	case C.INIT, C.OPENED, C.LISTENING, C.CONNECTING:
		return true
	}
	return false
}

func (s socketStatus) inTeardown() bool {
	switch C.enum_UDTSTATUS(s) {
	case C.BROKEN, C.CLOSED, C.NONEXIST: // c.CLOSING
		return true
	}
	return false
}

func (s socketStatus) inConnected(sock C.UDTSOCKET) bool {
	return C.enum_UDTSTATUS(s) == C.CONNECTED
}
