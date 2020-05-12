package multiplexer

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/gravitational/teleport/lib/defaults"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// NewTLSNextProtoListener
func NewTLSNextProtoListener(listener net.Listener) *TLSNextProtoListener {
	context, cancel := context.WithCancel(context.TODO())
	return &TLSNextProtoListener{
		tlsListener:   listener,
		http2Listener: newListener(context, listener.Addr()),
		httpListener:  newListener(context, listener.Addr()),
		cancel:        cancel,
		context:       context,
	}
}

// TLSNextProtoListner allows to split HTTP 1.1 and HTTP2
type TLSNextProtoListener struct {
	tlsListener   net.Listener
	http2Listener *Listener
	httpListener  *Listener
	cancel        context.CancelFunc
	context       context.Context
	isClosed      int32
}

func (l *TLSNextProtoListener) HTTP2() net.Listener {
	return l.http2Listener
}

func (l *TLSNextProtoListener) HTTP() net.Listener {
	return l.httpListener
}

func (l *TLSNextProtoListener) Serve() error {
	backoffTimer := time.NewTicker(5 * time.Second)
	defer backoffTimer.Stop()
	for {
		conn, err := l.tlsListener.Accept()
		if err == nil {
			tlsConn, ok := conn.(*tls.Conn)
			if !ok {
				conn.Close()
				log.WithError(err).Error("Expected tls.Conn, got %T, internal usage error.", conn)
				continue
			}
			go l.detectAndForward(tlsConn)
			continue
		}
		if atomic.LoadInt32(&l.isClosed) == 1 {
			return trace.ConnectionProblem(nil, "listener is closed")
		}
		select {
		case <-backoffTimer.C:
		case <-l.context.Done():
			return trace.ConnectionProblem(nil, "listener is closed")
		}
	}
}

func (l *TLSNextProtoListener) detectAndForward(conn *tls.Conn) {
	err := conn.SetReadDeadline(time.Now().Add(defaults.ReadHeadersTimeout))
	if err != nil {
		log.WithError(err).Debugf("Failed to set connection deadline.")
		conn.Close()
		return
	}
	if err := conn.Handshake(); err != nil {
		if trace.Unwrap(err) != io.EOF {
			log.WithError(err).Warning("Handshake failed.")
		}
		conn.Close()
		return
	}

	switch conn.ConnectionState().NegotiatedProtocol {
	case "h2":
		select {
		case l.http2Listener.connC <- conn:
		case <-l.context.Done():
			conn.Close()
			return
		}
	case "http/1.1", "":
		select {
		case l.httpListener.connC <- conn:
		case <-l.context.Done():
			conn.Close()
			return
		}
	default:
		conn.Close()
		log.WithError(err).Errorf("unsupported protocol: %v", conn.ConnectionState().NegotiatedProtocol)
		return
	}
}

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (l *TLSNextProtoListener) Close() error {
	defer l.cancel()
	atomic.StoreInt32(&l.isClosed, 1)
	return l.tlsListener.Close()
}

// Addr returns the listener's network address.
func (l *TLSNextProtoListener) Addr() net.Addr {
	return l.tlsListener.Addr()
}
