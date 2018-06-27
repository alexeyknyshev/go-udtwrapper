package udt

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"
)

// #cgo linux CFLAGS: -DLINUX
// #cgo darwin CFLAGS: -DOSX
// #cgo freebsd CFLAGS: -DBSD
// #cgo windows CXXFLAGS: -DWIN32 -DUDT_EXPORTS
// #cgo windows LDFLAGS: -static-libgcc -static -lkernel32 -luser32 -lws2_32
// #cgo i386 CFLAGS: -DIA32
// #cgo amd64 CFLAGS: -DAMD64
// #cgo CFLAGS: -Wall -finline-functions -O3 -fno-strict-aliasing -fvisibility=hidden
// #cgo LDFLAGS: -lm
// #include "udt_c_connector.h"
// #include "udt_c_errors.h"
// #include <errno.h>
// #include <string.h>
import "C"

// returned when calling functions on a fd that's closing.
var errClosing = errors.New("file descriptor closing")

var (
	// UDP_RCVBUF_SIZE is the default UDP_RCVBUF size.
	UDP_RCVBUF_SIZE = uint32(20971520) // 20MB

	// UDT_SNDTIMEO is the udt_send() timeout in milliseconds
	// note this doesnt change the interface, we use it as a poor polling
	UDT_SNDTIMEO_MS = C.int(UDT_ASYNC_TIMEOUT)

	// UDT_RCVTIMEO is the udt_recv() timeout in milliseconds
	// note this doesnt change the interface, we use it as a poor polling
	UDT_RCVTIMEO_MS = C.int(UDT_ASYNC_TIMEOUT)

	// UDT_ASYNC_TIMEOUT (in ms)
	UDT_ASYNC_TIMEOUT = 40
)

// rebind this here for type safety
var INVALID_SOCK C.UDTSOCKET = C.UDTSOCKET(C.INVALID_SOCK)

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

type signal struct{}
type semaphore chan struct{}

// udtFD (wraps udt.socket)
type udtFD struct {
	refcnt int32
	bound  bool

	// immutable until Close
	sock C.UDTSOCKET

	net   string
	laddr *UDTAddr
	raddr *UDTAddr
}

func newFD(sock C.UDTSOCKET, laddr, raddr *UDTAddr, net string) (*udtFD, error) {
	lac := laddr.copy()
	rac := raddr.copy()
	fd := &udtFD{sock: sock, laddr: lac, raddr: rac, net: net}

	return fd, nil
}

// lastErrorOp returns the last error as a net.OpError.
func (fd *udtFD) lastErrorOp(op string) *net.OpError {
	return &net.OpError{
		Op:     op,
		Net:    fd.net,
		Addr:   fd.laddr,
		Source: fd.raddr,
		Err:    lastError(),
	}
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

func (fd *udtFD) bind() error {
	_, sa, salen, err := fd.laddr.socketArgs()
	if err != nil {
		return err
	}

	// cast sockaddr
	csa := (*C.struct_sockaddr)(unsafe.Pointer(sa))
	if C.udt_bind(fd.sock, csa, C.int(salen)) != 0 {
		return fd.lastErrorOp("bind")
	}

	return nil
}

func (fd *udtFD) listen(backlog int) error {

	if C.udt_listen(fd.sock, C.int(backlog)) == C.ERROR {
		return fd.lastErrorOp("listen")
	}
	return nil
}

func (fd *udtFD) accept() (*udtFD, error) {
	var sa syscall.RawSockaddrAny
	var salen C.int

	sock2 := C.udt_accept(fd.sock, (*C.struct_sockaddr)(unsafe.Pointer(&sa)), &salen)
	if sock2 == (C.UDTSOCKET)(C.INVALID_SOCK) {
		err := fd.lastErrorOp("accept")
		return nil, err
	}

	raddr, err := addrWithSockaddr(&sa)
	if err != nil {
		closeSocket(sock2)
		return nil, fmt.Errorf("error converting address: %s", err)
	}

	remotefd, err := newFD(sock2, fd.laddr, raddr, fd.net)
	if err != nil {
		closeSocket(sock2)
		return nil, err
	}

	return remotefd, nil
}

func (fd *udtFD) connect(raddr *UDTAddr) error {

	_, sa, salen, err := raddr.socketArgs()
	if err != nil {
		return err
	}
	csa := (*C.struct_sockaddr)(unsafe.Pointer(sa))

	if C.udt_connect(fd.sock, csa, C.int(salen)) == C.ERROR {
		err := lastError()
		return fmt.Errorf("error connecting: %s", err)
	}

	fd.raddr = raddr
	if fd.laddr == nil {
		var lsa syscall.RawSockaddrAny
		var namelen C.int
		if C.udt_getsockname(fd.sock, (*C.struct_sockaddr)(unsafe.Pointer(&lsa)), &namelen) == C.ERROR {
			err := lastError()
			return fmt.Errorf("error getting local sockaddr: %s", err)
		}
		laddr, err := addrWithSockaddr(&lsa)
		if err != nil {
			return err
		}

		fd.laddr = laddr
	}
	return nil
}

func (fd *udtFD) Close() error {
	err := closeSocket(fd.sock)
	fd.sock = -1
	if err != nil {
		if err.Error() == "Operation not supported: Invalid socket ID." {
			// this ones okay, just means its already closed, somewhere
			return nil
		}
		return err
	}
	return nil
}

// net.Conn functions

func (fd *udtFD) LocalAddr() net.Addr {
	return fd.laddr
}

func (fd *udtFD) RemoteAddr() net.Addr {
	return fd.raddr
}

func (fd *udtFD) SetDeadline(t time.Time) error {
	err := fd.SetReadDeadline(t)
	if err != nil {
		return err
	}
	return fd.SetWriteDeadline(t)
}

func (fd *udtFD) SetReadDeadline(t time.Time) error {
	d := C.int(t.Sub(time.Now()).Nanoseconds() / 1e6)
	if C.udt_setsockopt(fd.sock, 0, C.UDT_RCVTIMEO, unsafe.Pointer(&d), C.sizeof_int) != 0 {
		return fmt.Errorf("failed to set ReadDeadline: %s", lastError())
	}
	return nil
}

func (fd *udtFD) SetWriteDeadline(t time.Time) error {
	d := C.int(t.Sub(time.Now()).Nanoseconds() / 1e6)
	if C.udt_setsockopt(fd.sock, 0, C.UDT_SNDTIMEO, unsafe.Pointer(&d), C.sizeof_int) != 0 {
		return fmt.Errorf("failed to set WriteDeadline: %s", lastError())
	}
	return nil
}

func (fd *udtFD) SetRendezvous(value bool) error {
	return fd.setBoolOpt(C.UDT_RENDEZVOUS, value)
}

func (fd *udtFD) setBoolOpt(option int, value bool) error {
	var b C.int
	if value {
		b = 1
	} else {
		b = 0
	}
	return fd.setSockOpt(option, unsafe.Pointer(&b), int(unsafe.Sizeof(b)))
}

func (fd *udtFD) setSockOpt(option int, optval unsafe.Pointer, optlen int) error {
	n := int(C.udt_setsockopt(fd.sock, 0, C.SOCKOPT(option), optval, (C.int)(optlen)))
	if n == C.UDT_SUCCESS {
		return nil
	}
	return lastError()
}

// lastError returns the last error as a Go string.
func lastError() error {
	return errors.New(C.GoString(C.udt_getlasterror_desc()))
}

func socket(addrfamily int) (sock C.UDTSOCKET, reterr error) {

	sock = C.udt_socket(C.int(addrfamily), C.SOCK_STREAM, 0)
	if sock == (C.UDTSOCKET)(C.INVALID_SOCK) {
		return (C.UDTSOCKET)(C.INVALID_SOCK), fmt.Errorf("invalid socket: %s", lastError())
	}

	return sock, nil
}

func closeSocket(sock C.UDTSOCKET) error {
	if C.udt_close(sock) == C.ERROR {
		return lastError()
	}
	return nil
}

type PerfMonData struct {
	// global measurements
	MsTimeStamp        int64 // time since the UDT entity is started, in milliseconds
	PktSentTotal       int64 // total number of sent data packets, including retransmissions
	PktRecvTotal       int64 // total number of received packets
	PktSndLossTotal    int   // total number of lost packets (sender side)
	PktRcvLossTotal    int   // total number of lost packets (receiver side)
	PktRetransTotal    int   // total number of retransmitted packets
	PktSentACKTotal    int   // total number of sent ACK packets
	PktRecvACKTotal    int   // total number of received ACK packets
	PktSentNAKTotal    int   // total number of sent NAK packets
	PktRecvNAKTotal    int   // total number of received NAK packets
	UsSndDurationTotal int64 // total time duration when UDT is sending data (idle time exclusive)

	// local measurements
	PktSent       int64   // number of sent data packets, including retransmissions
	PktRecv       int64   // number of received packets
	PktSndLoss    int     // number of lost packets (sender side)
	PktRcvLoss    int     // number of lost packets (receiver side)
	PktRetrans    int     // number of retransmitted packets
	PktSentACK    int     // number of sent ACK packets
	PktRecvACK    int     // number of received ACK packets
	PktSentNAK    int     // number of sent NAK packets
	PktRecvNAK    int     // number of received NAK packets
	MbpsSendRate  float64 // sending rate in Mb/s
	MbpsRecvRate  float64 // receiving rate in Mb/s
	UsSndDuration int64   // busy sending time (i.e., idle time exclusive)

	// instant measurements
	UsPktSndPeriod      float64 // packet sending period, in microseconds
	PktFlowWindow       int     // flow window size, in number of packets
	PktCongestionWindow int     // congestion window size, in number of packets
	PktFlightSize       int     // number of packets on flight
	MsRTT               float64 // RTT, in milliseconds
	MbpsBandwidth       float64 // estimated bandwidth, in Mb/s
	ByteAvailSndBuf     int     // available UDT sender buffer size
	ByteAvailRcvBuf     int     // available UDT receiver buffer size
}

func makePerfMonData(traceinfo *C.TRACEINFO) *PerfMonData {
	return &PerfMonData{
		// global measurements
		MsTimeStamp:        int64(traceinfo.msTimeStamp),
		PktSentTotal:       int64(traceinfo.pktSentTotal),
		PktRecvTotal:       int64(traceinfo.pktRecvTotal),
		PktSndLossTotal:    int(traceinfo.pktSndLossTotal),
		PktRcvLossTotal:    int(traceinfo.pktRcvLossTotal),
		PktRetransTotal:    int(traceinfo.pktRetransTotal),
		PktSentACKTotal:    int(traceinfo.pktSentACKTotal),
		PktRecvACKTotal:    int(traceinfo.pktRecvACKTotal),
		PktSentNAKTotal:    int(traceinfo.pktSentNAKTotal),
		PktRecvNAKTotal:    int(traceinfo.pktRecvNAKTotal),
		UsSndDurationTotal: int64(traceinfo.usSndDurationTotal),

		// local measurements
		PktSent:       int64(traceinfo.pktSent),
		PktRecv:       int64(traceinfo.pktRecv),
		PktSndLoss:    int(traceinfo.pktSndLoss),
		PktRcvLoss:    int(traceinfo.pktRcvLoss),
		PktRetrans:    int(traceinfo.pktRetrans),
		PktSentACK:    int(traceinfo.pktSentACK),
		PktRecvACK:    int(traceinfo.pktRecvACK),
		PktSentNAK:    int(traceinfo.pktSentNAK),
		PktRecvNAK:    int(traceinfo.pktRecvNAK),
		MbpsSendRate:  float64(traceinfo.mbpsSendRate),
		MbpsRecvRate:  float64(traceinfo.mbpsRecvRate),
		UsSndDuration: int64(traceinfo.usSndDuration),

		// instant measurements
		UsPktSndPeriod:      float64(traceinfo.usPktSndPeriod),
		PktFlowWindow:       int(traceinfo.pktFlowWindow),
		PktCongestionWindow: int(traceinfo.pktCongestionWindow),
		PktFlightSize:       int(traceinfo.pktFlightSize),
		MsRTT:               float64(traceinfo.msRTT),
		MbpsBandwidth:       float64(traceinfo.mbpsBandwidth),
		ByteAvailSndBuf:     int(traceinfo.byteAvailSndBuf),
		ByteAvailRcvBuf:     int(traceinfo.byteAvailRcvBuf),
	}
}

func (fd *udtFD) PerfMon(clear bool) (*PerfMonData, error) {
	var traceinfo C.TRACEINFO
	var c int
	if clear {
		c = 1
	} else {
		c = 0
	}
	if C.udt_perfmon(fd.sock, &traceinfo, C.int(c)) == C.ERROR {
		return nil, lastError()
	}
	return makePerfMonData(&traceinfo), nil
}

// dialFD sets up a udtFD
func dialFD(laddr, raddr *UDTAddr, rendezvous bool) (*udtFD, error) {

	if raddr == nil {
		return nil, &net.OpError{"dial", "udt", laddr, raddr, errors.New("invalid remote address")}
	}

	if laddr != nil && laddr.AF() != raddr.AF() {
		return nil, &net.OpError{"dial", "udt", laddr, raddr, errors.New("differing remote address network")}
	}

	sock, err := socket(raddr.AF())
	if err != nil {
		return nil, err
	}

	fd, err := newFD(sock, laddr, raddr, "udt")
	if err != nil {
		closeSocket(sock)
		return nil, err
	}

	if rendezvous {
		fd.SetRendezvous(true)
	}

	if laddr != nil {
		if err := fd.bind(); err != nil {
			fd.Close()
			return nil, err
		}
	}

	if err := fd.connect(raddr); err != nil {
		fd.Close()
		return nil, err
	}

	return fd, nil
}

// listenFD sets up a udtFD
func listenFD(laddr *UDTAddr) (*udtFD, error) {

	if laddr == nil {
		return nil, &net.OpError{
			Op:     "dial",
			Net:    "udt",
			Addr:   nil,
			Source: nil,
			Err:    errors.New("invalid address"),
		}
	}

	sock, err := socket(laddr.AF())
	if err != nil {
		return nil, err
	}

	fd, err := newFD(sock, laddr, nil, "udt")
	if err != nil {
		closeSocket(sock)
		return nil, err
	}

	if err := fd.bind(); err != nil {
		fd.Close()
		return nil, err
	}

	// TODO: use maxListenerBacklog like golang.org/net/
	if err := fd.listen(syscall.SOMAXCONN); err != nil {
		fd.Close()
		return nil, err
	}

	return fd, nil
}
