package utils

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/go-logr/logr"
)

// CheckTLS used the []byte PEM crt and PEM key to connect host with HTTPS
func CheckTLS(log logr.Logger, crtPEM, keyPEM []byte, host string) error {

	// Load client certificate
	cert, err := tls.X509KeyPair(crtPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("failed to load cert/key: %v", err)
	}

	// Root CA pool
	rootCAs := x509.NewCertPool()
	rootCAs.AppendCertsFromPEM(crtPEM)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      rootCAs,
		ServerName:   host, // important: SNI
	}

	conn, err := tls.Dial("tcp", host+":443", tlsConfig)
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
