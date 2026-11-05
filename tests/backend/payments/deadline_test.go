package payments

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"testing"
	"time"

	"vanillapay/tests/backend/testsupport"
)

// Hold the physical socket open after TLS Close has sent close_notify, until
// the peer's final HTTP chunk has reached Read. This makes the cancellation /
// clean-EOF race deterministic without changing the HTTP or TLS implementation.
type deadlineCloseDialer struct {
	network   *testsupport.PublicNetwork
	closing   chan struct{}
	received  chan struct{}
	exhausted chan struct{}
	closed    chan struct{}
}

func (d *deadlineCloseDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := d.network.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	return &deadlineCloseConn{Conn: conn, dialer: d}, nil
}

type deadlineCloseConn struct {
	net.Conn
	dialer   *deadlineCloseDialer
	once     sync.Once
	readOnce sync.Once
	closeErr error
}

func (c *deadlineCloseConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	select {
	case <-c.dialer.closing:
		if n > 0 {
			c.readOnce.Do(func() { close(c.dialer.received) })
		}
	default:
	}
	return n, err
}

func (c *deadlineCloseConn) Close() error {
	c.once.Do(func() {
		close(c.dialer.closing)
		select {
		case <-c.dialer.received:
		case <-time.After(2 * time.Second):
			close(c.dialer.exhausted)
		}
		c.closeErr = c.Conn.Close()
		close(c.dialer.closed)
	})
	return c.closeErr
}

func TestHTTP_CallbackDeadlineCleanEOF(t *testing.T) {
	for _, useContext := range []bool{false, true} {
		name := "client_timeout"
		if useContext {
			name = "context_deadline"
		}
		t.Run(name, func(t *testing.T) {
			network := testsupport.NewPublicNetwork(t)
			network.SetDNS("callback.vp.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
			dialer := &deadlineCloseDialer{network: network, closing: make(chan struct{}), received: make(chan struct{}), exhausted: make(chan struct{}), closed: make(chan struct{})}
			endpointClosed := make(chan struct{})
			network.ServeTLS(t, "callback.vp.test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Proto != "HTTP/1.1" {
					t.Errorf("protocol = %s", r.Proto)
				}
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				<-dialer.closing
				close(endpointClosed)
			}))
			client := network.Client(250 * time.Millisecond)
			client.Dialer = dialer
			ctx := context.Background()
			if useContext {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 250*time.Millisecond)
				defer cancel()
				client.Timeout = time.Hour
			}
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://callback.vp.test/slow-body", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.DoPublic(ctx, request)
			if err != nil {
				t.Fatalf("did not reach response body: %v", err)
			}
			defer func() { _ = response.Body.Close() }()
			if response.Proto != "HTTP/1.1" || len(response.TransferEncoding) != 1 || response.TransferEncoding[0] != "chunked" {
				t.Fatalf("response framing = %s %v, want HTTP/1.1 chunked", response.Proto, response.TransferEncoding)
			}
			_, readErr := io.ReadAll(response.Body)
			select {
			case <-endpointClosed:
			default:
				t.Fatal("server did not observe connection cancellation")
			}
			select {
			case <-dialer.closed:
			case <-time.After(2 * time.Second):
				t.Fatal("cancelled physical connection remained open")
			}
			select {
			case <-dialer.exhausted:
				t.Fatal("close barrier did not receive the peer response")
			default:
			}
			if !errors.Is(readErr, context.DeadlineExceeded) {
				t.Fatalf("expired response body error = %v, want deadline exceeded", readErr)
			}
		})
	}
}
