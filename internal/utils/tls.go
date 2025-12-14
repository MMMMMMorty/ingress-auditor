package utils

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/go-logr/logr"
)

const httpsPort = 443

// CheckTLS used the []byte PEM crt and PEM key to connect host with HTTPS
func CheckTLS(log logr.Logger, crtPEM, keyPEM []byte, host string) error {

	// Root CA pool
	rootCAs := x509.NewCertPool()
	rootCAs.AppendCertsFromPEM(crtPEM)

	tlsConfig := &tls.Config{
		RootCAs:    rootCAs,
		ServerName: host, // important: SNI
	}

	dialer := &net.Dialer{
		Timeout:   5 * time.Second, // connection timeout
		KeepAlive: 5 * time.Second,
	}

	address := net.JoinHostPort(host, strconv.Itoa(httpsPort))

	conn, err := tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %v", host, err)
	}

	defer func() {
		if err := conn.Close(); err != nil {
			log.Error(err, "failed to close connection")
		}
	}()

	return nil
}
