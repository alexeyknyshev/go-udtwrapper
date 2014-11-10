package udt

import (
	"errors"
	"fmt"
	"net"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	sockaddr "github.com/jbenet/go-sockaddr"
)

// #cgo CFLAGS: -Wall
// #cgo LDFLAGS: libudt.dylib
//
// #include "udt_c.h"
// #include <errno.h>
// #include <arpa/inet.h>
// #include <string.h>
import "C"

// returned when calling functions on a fd that's closing.
var errClosing = errors.New("file descriptor closing")

// UDP_RCVBUF_SIZE is the default UDP_RCVBUF size.
var UDP_RCVBUF_SIZE = uint32(20971520) // 20MB

func init() {
	// adjust the rcvbuf to our max.
	max, err := maxRcvBufSize()
	if err == nil {
		max = uint32(float32(max) * 0.5) // 0.5 of max.
		if max < UDP_RCVBUF_SIZE {
			UDP_RCVBUF_SIZE = max
		}
	}
}

// udtLock is a lock on the entire udt API. WHAT!? you might say,
// and you'd be right to scream. The udt API is not re-entrant,
// and in particular it _sets a global error and has the user
// fetch it with a function!!! (errno style. wtf)_
//
// Since we're probably paying the lovely cost of a syscall on
// such calls this isn't sooo bad. But it's still bad.
var udtLock sync.Mutex

// udtFD (wraps udt.socket)
type udtFD struct {
	fdmu sync.Mutex

	// immutable until Close
	sock        C.UDTSOCKET
	family      int
	sotype      int
	refcnt      int32
	isClosing   bool
	isConnected bool
	net         string
	laddr       *UDTAddr
	raddr       *UDTAddr
}

func sysInit() {
}

// lastError returns the last error as a Go string.
// caller should be holding udtLock, or errors may be reported
// incorrectly...
func lastError() error {
	return errors.New(C.GoString(C.udt_getlasterror_desc()))
}

func newFD(sock C.UDTSOCKET, family, sotype int, net string) *udtFD {
	return &udtFD{sock: sock, family: family, sotype: sotype, net: net}
}

func (fd *udtFD) init() error {
	return nil
}

func (fd *udtFD) LocalAddr() net.Addr {
	return fd.laddr
}

func (fd *udtFD) RemoteAddr() net.Addr {
	return fd.raddr
}

func (fd *udtFD) setAddr(laddr, raddr *UDTAddr) {
	fd.laddr = laddr
	fd.raddr = raddr
	runtime.SetFinalizer(fd, (*udtFD).Close)
}

func (fd *udtFD) name() string {
	var ls, rs string
	if fd.laddr != nil {
		ls = fd.laddr.String()
	}
	if fd.raddr != nil {
		rs = fd.raddr.String()
	}
	return fd.net + ":" + ls + "->" + rs
}

func (fd *udtFD) connect(la, ra syscall.Sockaddr) error {

	// convert the given syscall.Sockaddr to a syscall.RawSockaddrAny
	// and then to a C.struct_sockaddr_any.
	rra, ralen, err := sockaddr.SockaddrToAny(ra)
	if err != nil {
		return err
	}
	cra := (*C.struct_sockaddr)(unsafe.Pointer(rra))

	udtLock.Lock()
	if C.udt_connect(fd.sock, cra, C.int(ralen)) == C.ERROR {
		err := lastError()
		udtLock.Unlock()
		return err
	}
	udtLock.Unlock()

	for {
		// TODO: replace this with proper net waiting on a Write.
		// polling (EEEEW).
		<-time.After(time.Microsecond * 10)

		nerrlen := C.int(C.sizeof_int)
		nerr := C.int(0)

		udtLock.Lock()
		if C.udt_getsockopt(fd.sock, syscall.SOL_SOCKET, syscall.SO_ERROR, unsafe.Pointer(&nerr), &nerrlen) == C.ERROR {
			err := lastError()
			udtLock.Unlock()
			return err
		}
		udtLock.Unlock()

		switch err := syscall.Errno(nerr); err {
		case syscall.EINPROGRESS, syscall.EALREADY, syscall.EINTR:
		case syscall.Errno(0), syscall.EISCONN:
			return nil
		default:
			return err
		}
	}
}

func (fd *udtFD) destroy() {
	closeSocket(fd.sock)
	fd.sock = -1
	runtime.SetFinalizer(fd, nil)
}

// // Add a reference to this fd.
// // Returns an error if the fd cannot be used.
// func (fd *udtFD) incref() error {
// 	if err := fd.lock(); err != nil {
// 		return err
// 	}
// 	fd.fdmu.Unlock()
// 	return nil
// }

// // Remove a reference to this FD and close if we've been asked to do so
// // (and there are no references left).
// func (fd *udtFD) decref() {
// 	fd.fdmu.Lock()
// 	if fd.isClosing {
// 		fd.fdmu.Unlock()
// 		return
// 	}
// 	fd.fdmu.unlock()
// }

// Add a reference to this fd and lock for reading.
// Returns an error if the fd cannot be used.
func (fd *udtFD) lock() error {
	fd.fdmu.Lock()
	if fd.isClosing {
		fd.fdmu.Unlock()
		return errClosing
	}
	fd.refcnt++
	return nil
}

// Unlock for reading and remove a reference to this FD.
func (fd *udtFD) unlock() {
	fd.refcnt--
	if fd.refcnt == 0 {
		fd.destroy()
	}
	fd.fdmu.Unlock()
}

func (fd *udtFD) Close() error {
	if err := fd.lock(); err != nil {
		return err
	}

	// Unblock any I/O.  Once it all unblocks and returns,
	// so that it cannot be referring to fd.sysfd anymore,
	// the final decref will close fd.sysfd.  This should happen
	// fairly quickly, since all the I/O is non-blocking, and any
	// attempts to block in the pollDesc will return errClosing.

	// TODO
	fd.isClosing = true
	fd.unlock()
	return nil
}

// func (fd *udtFD) shutdown(how int) error {
// 	if err := fd.incref(); err != nil {
// 		return err
// 	}
// 	defer fd.decref()

// 	if err := fd.closeSocket(); err != nil {
// 		return &net.OpError{"shutdown", fd.net, fd.laddr, err}
// 	}
// 	return nil
// }

func closeSocket(sock C.UDTSOCKET) error {
	udtLock.Lock()
	defer udtLock.Unlock()

	if C.udt_close(sock) == C.ERROR {
		return lastError()
	}
	return nil
}

func (fd *udtFD) Read(p []byte) (n int, err error) {
	panic("not yet implemented")
	// 	if err := fd.readLock(); err != nil {
	// 		return 0, err
	// 	}
	// 	defer fd.readUnlock()
	// 	if err := fd.pd.PrepareRead(); err != nil {
	// 		return 0, &net.OpError{"read", fd.net, fd.raddr, err}
	// 	}
	// 	for {
	// 		n, err = syscall.Read(int(fd.sysfd), p)
	// 		if err != nil {
	// 			n = 0
	// 			if err == syscall.EAGAIN {
	// 				if err = fd.pd.WaitRead(); err == nil {
	// 					continue
	// 				}
	// 			}
	// 		}
	// 		err = chkReadErr(n, err, fd)
	// 		break
	// 	}
	// 	if err != nil && err != io.EOF {
	// 		err = &net.OpError{"read", fd.net, fd.raddr, err}
	// 	}
	// 	return
}

// func (fd *udtFD) readFrom(p []byte) (n int, sa syscall.Sockaddr, err error) {
// 	if err := fd.readLock(); err != nil {
// 		return 0, nil, err
// 	}
// 	defer fd.readUnlock()
// 	if err := fd.pd.PrepareRead(); err != nil {
// 		return 0, nil, &net.OpError{"read", fd.net, fd.laddr, err}
// 	}
// 	for {
// 		n, sa, err = syscall.Recvfrom(fd.sysfd, p, 0)
// 		if err != nil {
// 			n = 0
// 			if err == syscall.EAGAIN {
// 				if err = fd.pd.WaitRead(); err == nil {
// 					continue
// 				}
// 			}
// 		}
// 		err = chkReadErr(n, err, fd)
// 		break
// 	}
// 	if err != nil && err != io.EOF {
// 		err = &net.OpError{"read", fd.net, fd.laddr, err}
// 	}
// 	return
// }

// func (fd *udtFD) readMsg(p []byte, oob []byte) (n, oobn, flags int, sa syscall.Sockaddr, err error) {
// 	if err := fd.readLock(); err != nil {
// 		return 0, 0, 0, nil, err
// 	}
// 	defer fd.readUnlock()
// 	if err := fd.pd.PrepareRead(); err != nil {
// 		return 0, 0, 0, nil, &net.OpError{"read", fd.net, fd.laddr, err}
// 	}
// 	for {
// 		n, oobn, flags, sa, err = syscall.Recvmsg(fd.sysfd, p, oob, 0)
// 		if err != nil {
// 			// TODO(dfc) should n and oobn be set to 0
// 			if err == syscall.EAGAIN {
// 				if err = fd.pd.WaitRead(); err == nil {
// 					continue
// 				}
// 			}
// 		}
// 		err = chkReadErr(n, err, fd)
// 		break
// 	}
// 	if err != nil && err != io.EOF {
// 		err = &net.OpError{"read", fd.net, fd.laddr, err}
// 	}
// 	return
// }

// func chkReadErr(n int, err error, fd *udtFD) error {
// 	if n == 0 && err == nil && fd.sotype != syscall.SOCK_DGRAM && fd.sotype != syscall.SOCK_RAW {
// 		return io.EOF
// 	}
// 	return err
// }

func (fd *udtFD) Write(p []byte) (nn int, err error) {
	panic("not yet implemented")
	// 	if err := fd.writeLock(); err != nil {
	// 		return 0, err
	// 	}
	// 	defer fd.writeUnlock()
	// 	if err := fd.pd.PrepareWrite(); err != nil {
	// 		return 0, &net.OpError{"write", fd.net, fd.raddr, err}
	// 	}
	// 	for {
	// 		var n int
	// 		n, err = syscall.Write(int(fd.sysfd), p[nn:])
	// 		if n > 0 {
	// 			nn += n
	// 		}
	// 		if nn == len(p) {
	// 			break
	// 		}
	// 		if err == syscall.EAGAIN {
	// 			if err = fd.pd.WaitWrite(); err == nil {
	// 				continue
	// 			}
	// 		}
	// 		if err != nil {
	// 			n = 0
	// 			break
	// 		}
	// 		if n == 0 {
	// 			err = io.ErrUnexpectedEOF
	// 			break
	// 		}
	// 	}
	// 	if err != nil {
	// 		err = &net.OpError{"write", fd.net, fd.raddr, err}
	// 	}
	// 	return nn, err
}

// func (fd *udtFD) writeTo(p []byte, sa syscall.Sockaddr) (n int, err error) {
// 	if err := fd.writeLock(); err != nil {
// 		return 0, err
// 	}
// 	defer fd.writeUnlock()
// 	if err := fd.pd.PrepareWrite(); err != nil {
// 		return 0, &net.OpError{"write", fd.net, fd.raddr, err}
// 	}
// 	for {
// 		err = syscall.Sendto(fd.sysfd, p, 0, sa)
// 		if err == syscall.EAGAIN {
// 			if err = fd.pd.WaitWrite(); err == nil {
// 				continue
// 			}
// 		}
// 		break
// 	}
// 	if err == nil {
// 		n = len(p)
// 	} else {
// 		err = &net.OpError{"write", fd.net, fd.raddr, err}
// 	}
// 	return
// }

// func (fd *udtFD) writeMsg(p []byte, oob []byte, sa syscall.Sockaddr) (n int, oobn int, err error) {
// 	if err := fd.writeLock(); err != nil {
// 		return 0, 0, err
// 	}
// 	defer fd.writeUnlock()
// 	if err := fd.pd.PrepareWrite(); err != nil {
// 		return 0, 0, &net.OpError{"write", fd.net, fd.raddr, err}
// 	}
// 	for {
// 		n, err = syscall.SendmsgN(fd.sysfd, p, oob, sa, 0)
// 		if err == syscall.EAGAIN {
// 			if err = fd.pd.WaitWrite(); err == nil {
// 				continue
// 			}
// 		}
// 		break
// 	}
// 	if err == nil {
// 		oobn = len(oob)
// 	} else {
// 		err = &net.OpError{"write", fd.net, fd.raddr, err}
// 	}
// 	return
// }

// func (fd *udtFD) accept(toAddr func(syscall.Sockaddr) Addr) (netfd *udtFD, err error) {
// 	if err := fd.readLock(); err != nil {
// 		return nil, err
// 	}
// 	defer fd.readUnlock()

// 	var s int
// 	var rsa syscall.Sockaddr
// 	if err = fd.pd.PrepareRead(); err != nil {
// 		return nil, &net.OpError{"accept", fd.net, fd.laddr, err}
// 	}
// 	for {
// 		s, rsa, err = accept(fd.sysfd)
// 		if err != nil {
// 			if err == syscall.EAGAIN {
// 				if err = fd.pd.WaitRead(); err == nil {
// 					continue
// 				}
// 			} else if err == syscall.ECONNABORTED {
// 				// This means that a socket on the listen queue was closed
// 				// before we Accept()ed it; it's a silly error, so try again.
// 				continue
// 			}
// 			return nil, &net.OpError{"accept", fd.net, fd.laddr, err}
// 		}
// 		break
// 	}

// 	if netfd, err = newFD(s, fd.family, fd.sotype, fd.net); err != nil {
// 		closesocket(s)
// 		return nil, err
// 	}
// 	if err = netfd.init(); err != nil {
// 		fd.Close()
// 		return nil, err
// 	}
// 	lsa, _ := syscall.Getsockname(netfd.sysfd)
// 	netfd.setAddr(toAddr(lsa), toAddr(rsa))
// 	return netfd, nil
// }

// // tryDupCloexec indicates whether F_DUPFD_CLOEXEC should be used.
// // If the kernel doesn't support it, this is set to 0.
// var tryDupCloexec = int32(1)

// func dupCloseOnExec(fd int) (newfd int, err error) {
// 	if atomic.LoadInt32(&tryDupCloexec) == 1 {
// 		r0, _, e1 := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_DUPFD_CLOEXEC, 0)
// 		if runtime.GOOS == "darwin" && e1 == syscall.EBADF {
// 			// On OS X 10.6 and below (but we only support
// 			// >= 10.6), F_DUPFD_CLOEXEC is unsupported
// 			// and fcntl there falls back (undocumented)
// 			// to doing an ioctl instead, returning EBADF
// 			// in this case because fd is not of the
// 			// expected device fd type.  Treat it as
// 			// EINVAL instead, so we fall back to the
// 			// normal dup path.
// 			// TODO: only do this on 10.6 if we can detect 10.6
// 			// cheaply.
// 			e1 = syscall.EINVAL
// 		}
// 		switch e1 {
// 		case 0:
// 			return int(r0), nil
// 		case syscall.EINVAL:
// 			// Old kernel. Fall back to the portable way
// 			// from now on.
// 			atomic.StoreInt32(&tryDupCloexec, 0)
// 		default:
// 			return -1, e1
// 		}
// 	}
// 	return dupCloseOnExecOld(fd)
// }

// // dupCloseOnExecUnixOld is the traditional way to dup an fd and
// // set its O_CLOEXEC bit, using two system calls.
// func dupCloseOnExecOld(fd int) (newfd int, err error) {
// 	syscall.ForkLock.RLock()
// 	defer syscall.ForkLock.RUnlock()
// 	newfd, err = syscall.Dup(fd)
// 	if err != nil {
// 		return -1, err
// 	}
// 	syscall.CloseOnExec(newfd)
// 	return
// }

// func (fd *udtFD) dup() (f *os.File, err error) {
// 	ns, err := dupCloseOnExec(fd.sysfd)
// 	if err != nil {
// 		return nil, &net.OpError{"dup", fd.net, fd.laddr, err}
// 	}

// 	// We want blocking mode for the new fd, hence the double negative.
// 	// This also puts the old fd into blocking mode, meaning that
// 	// I/O will block the thread instead of letting us use the epoll server.
// 	// Everything will still work, just with more threads.
// 	if err = syscall.SetNonblock(ns, false); err != nil {
// 		return nil, &net.OpError{"setnonblock", fd.net, fd.laddr, err}
// 	}

// 	return os.NewFile(uintptr(ns), fd.name()), nil
// }

// func skipRawSocketTests() (skip bool, skipmsg string, err error) {
// 	if os.Getuid() != 0 {
// 		return true, "skipping test; must be root", nil
// 	}
// 	return false, "", nil
// }

func (fd *udtFD) SetDeadline(t time.Time) error {
	panic("not yet implemented")
}

func (fd *udtFD) SetReadDeadline(t time.Time) error {
	panic("not yet implemented")
}

func (fd *udtFD) SetWriteDeadline(t time.Time) error {
	panic("not yet implemented")
}

func socket(laddr *UDTAddr) (C.UDTSOCKET, error) {
	af, sa, salen, err := laddr.socketArgs()
	if err != nil {
		return 0, err
	}

	udtLock.Lock()
	sock := C.udt_socket(C.int(af), C.SOCK_STREAM, 0)
	if sock == C.INVALID_SOCK {
		err := lastError()
		udtLock.Unlock()
		return 0, fmt.Errorf("invalid socket: %s", err)
	}

	// reduce maximum size 
	if C.udt_setsockopt(sock, 0, C.UDP_RCVBUF, unsafe.Pointer(&UDP_RCVBUF_SIZE), C.sizeof_int) != 0 {
		err := lastError()
		udtLock.Unlock()
		return 0, fmt.Errorf("failed to set rcvbuf: %d, %s", UDP_RCVBUF_SIZE, err)		
	}

	// cast sockaddr
	csa := (*C.struct_sockaddr)(unsafe.Pointer(sa))
	if C.udt_bind(sock, csa, C.int(salen)) != 0 {
		err := lastError()
		udtLock.Unlock()
		return 0, fmt.Errorf("failed to bind: %s, %s", laddr, err)
	}
	udtLock.Unlock()
	return sock, nil
}