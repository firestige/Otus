// Package kafkautil provides shared Kafka connection helpers.
package kafkautil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"

	"icc.tech/capture-agent/internal/config"
)

// BuildDialer constructs a *kafka.Dialer with optional SASL and TLS settings.
// Returns kafka.DefaultDialer when both SASL and TLS are disabled.
func BuildDialer(saslCfg config.SASLConfig, tlsCfg config.TLSConfig) (*kafka.Dialer, error) {
	if !saslCfg.Enabled && !tlsCfg.Enabled {
		return kafka.DefaultDialer, nil
	}

	d := &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
	}

	if saslCfg.Enabled {
		switch strings.ToUpper(saslCfg.Mechanism) {
		case "PLAIN":
			d.SASLMechanism = plain.Mechanism{
				Username: saslCfg.Username,
				Password: saslCfg.Password,
			}
		case "SCRAM-SHA-256":
			m, err := scram.Mechanism(scram.SHA256, saslCfg.Username, saslCfg.Password)
			if err != nil {
				return nil, fmt.Errorf("build SCRAM-SHA-256 mechanism: %w", err)
			}
			d.SASLMechanism = m
		case "SCRAM-SHA-512":
			m, err := scram.Mechanism(scram.SHA512, saslCfg.Username, saslCfg.Password)
			if err != nil {
				return nil, fmt.Errorf("build SCRAM-SHA-512 mechanism: %w", err)
			}
			d.SASLMechanism = m
		default:
			return nil, fmt.Errorf("unsupported SASL mechanism: %q (supported: PLAIN, SCRAM-SHA-256, SCRAM-SHA-512)", saslCfg.Mechanism)
		}
	}

	if tlsCfg.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: tlsCfg.InsecureSkipVerify, //nolint:gosec // controlled by operator config
		}
		if tlsCfg.CACert != "" {
			pem, err := os.ReadFile(tlsCfg.CACert)
			if err != nil {
				return nil, fmt.Errorf("read CA cert %q: %w", tlsCfg.CACert, err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("parse CA cert %q: no valid PEM block found", tlsCfg.CACert)
			}
			tlsConfig.RootCAs = pool
		}
		if tlsCfg.ClientCert != "" && tlsCfg.ClientKey != "" {
			cert, err := tls.LoadX509KeyPair(tlsCfg.ClientCert, tlsCfg.ClientKey)
			if err != nil {
				return nil, fmt.Errorf("load client cert/key: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		d.TLS = tlsConfig
	}

	return d, nil
}
