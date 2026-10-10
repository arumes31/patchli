package webhooks

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestWebhookDialRejectsLocalDestinations(t *testing.T) {
	for _, address := range []string{"127.0.0.1:80", "[::1]:80", "10.0.0.1:80", "169.254.169.254:80", "localhost:80", "0.0.0.0:80"} {
		t.Run(address, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			conn, err := dialWebhook(ctx, "tcp", address)
			if conn != nil {
				_ = conn.Close()
				t.Fatal("connected to prohibited destination")
			}
			if !errors.Is(err, errNonPublicWebhook) {
				t.Fatalf("expected destination rejection, got %v", err)
			}
		})
	}
}

func TestWebhookAddressesRejectBeforeDial(t *testing.T) {
	for _, address := range []string{"100.64.0.1", "::ffff:100.64.0.1", "192.0.2.1", "0.1.2.3", "240.1.2.3", "198.18.0.1", "2002:7f00:1::", "64:ff9b::7f00:1", "2001:db8::1", "fc00::1"} {
		t.Run(address, func(t *testing.T) {
			addresses := []net.IPAddr{{IP: net.ParseIP("1.1.1.1")}, {IP: net.ParseIP(address)}}
			_, err := dialWebhookAddresses(context.Background(), "tcp", "443", addresses, func(context.Context, string, string) (net.Conn, error) {
				t.Fatal("dial called before all addresses were validated")
				return nil, nil
			})
			if !errors.Is(err, errNonPublicWebhook) {
				t.Fatalf("expected rejection, got %v", err)
			}
		})
	}
}

func TestWebhookDialsExactValidatedAddress(t *testing.T) {
	wantErr := errors.New("test dial result")
	_, err := dialWebhookAddresses(context.Background(), "tcp", "443", []net.IPAddr{{IP: net.ParseIP("1.1.1.1")}}, func(_ context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "1.1.1.1:443" {
			t.Fatalf("unexpected dial: %s %s", network, address)
		}
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("unexpected result: %v", err)
	}
}

func TestWebhookRedirectIsNotFollowed(t *testing.T) {
	if err := webhookClient.CheckRedirect(&http.Request{}, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("redirect allowed: %v", err)
	}
}
