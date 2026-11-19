// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package db

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/config"
)

// writeCA writes a self-signed CA certificate as PEM and returns its path.
func writeCA(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test MySQL CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTLSConfigFor(t *testing.T) {
	if c, err := TLSConfigFor(config.DBConfig{}); c != nil || err != nil {
		t.Errorf("neither set: %v, %v (want nil: the driver's tls=true)", c, err)
	}

	ca := writeCA(t)
	c, err := TLSConfigFor(config.DBConfig{TLSCA: ca, TLSServerName: "mysql.internal.wso2.com"})
	if err != nil {
		t.Fatal(err)
	}
	if c.RootCAs == nil || c.ServerName != "mysql.internal.wso2.com" || c.MinVersion != tls.VersionTLS12 || c.InsecureSkipVerify {
		t.Errorf("config = roots %v, server %q, min %x, skip %v", c.RootCAs != nil, c.ServerName, c.MinVersion, c.InsecureSkipVerify)
	}

	c, err = TLSConfigFor(config.DBConfig{TLSServerName: "mysql.internal.wso2.com"})
	if err != nil || c.RootCAs != nil || c.ServerName != "mysql.internal.wso2.com" {
		t.Errorf("server name only: %+v, %v", c, err)
	}

	if _, err := TLSConfigFor(config.DBConfig{TLSCA: filepath.Join(t.TempDir(), "missing.pem")}); err == nil || !strings.Contains(err.Error(), "DB_TLS_CA") {
		t.Errorf("missing CA file: err = %v", err)
	}
	notPEM := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(notPEM, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := TLSConfigFor(config.DBConfig{TLSCA: notPEM}); err == nil || !strings.Contains(err.Error(), "no PEM certificate") {
		t.Errorf("non-PEM CA file: err = %v", err)
	}
}

// A bad CA stops New before any connection is tried.
func TestNewRefusesBadCA(t *testing.T) {
	_, err := New(&config.Config{DB: config.DBConfig{Host: "127.0.0.1", Port: 1, User: "u", Password: "p", TLSCA: filepath.Join(t.TempDir(), "missing.pem")}})
	if err == nil || !strings.Contains(err.Error(), "DB_TLS_CA") {
		t.Errorf("err = %v", err)
	}
}
