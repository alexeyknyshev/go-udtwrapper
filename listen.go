package udt

import (
	"net"

	// sockaddr "github.com/jbenet/go-sockaddr"
	// sockaddrnet "github.com/jbenet/go-sockaddr/net"
)

// UDTListener is a network listener for UDT.
type UDTListener struct {
	net.Listener

	fd *udtFD
}

func (l *UDTListener) Accept() (c net.Conn, err error) {

	logf("Accept UDTListener")
	defer logf("UDTListener is accepted")
	cfd, err := l.fd.accept()
	if err != nil {
		logf("Accept err: ", err)
		return nil, err
	}

	return &UDTConn{
		udtFD: cfd,
		net:   l.fd.LocalAddr().Network(),
	}, nil
}

func (l *UDTListener) Close() error {
	logf("Close UDTListener")
	defer logf("UDTListener is closed")
	err := l.fd.Close()
	if err != nil {
		logf("Failed to close UDTListener: ", err)
	}
	return err
}

func (l *UDTListener) Addr() net.Addr {
	return l.fd.LocalAddr()
}

// ListenUDT listens for incoming UDT packets addressed to the local
// address laddr.  Net must be "udt", "udt4", or "udt6".  If laddr has
// a port of 0, ListenUDT will choose an available port.
// The LocalAddr method of the returned UDTConn can be used to
// discover the port.  The returned connection's ReadFrom and WriteTo
// methods can be used to receive and send UDT packets with per-packet
// addressing.
func ListenUDT(network string, laddr *UDTAddr) (*UDTListener, error) {
	logf("ListenUDT")
	defer logf("defer ListenUDT")
	switch network {
	case "udt", "udt4", "udt6":
	default:
		return nil, &net.OpError{Op: "listen", Net: network, Addr: laddr, Err: net.UnknownNetworkError(network)}
	}
	if laddr == nil {
		laddr = &UDTAddr{addr: &net.UDPAddr{}}
	}

	fdl, err := listenFD(laddr)
	if err != nil {
		logf("Failed to Listen udt: ", err)
		return nil, err
	}
	return &UDTListener{fd: fdl}, nil
}

// Listen listens for incoming UDT packets addressed to the local
// address laddr.  Net must be "udt", "udt4", or "udt6".  If laddr has
// a port of 0, ListenUDT will choose an available port.
// The LocalAddr method of the returned UDTConn can be used to
// discover the port.  The returned connection's ReadFrom and WriteTo
// methods can be used to receive and send UDT packets with per-packet
// addressing.
func Listen(network, address string) (net.Listener, error) {
	laddr, err := ResolveUDTAddr(network, address)
	if err != nil {
		return nil, err
	}
	return ListenUDT(network, laddr)
}
