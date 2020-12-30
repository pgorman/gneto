// Copyright 2020 Paul Gorman. Licensed under the AGPL.

// Gneto makes Gemini pages available over HTTP.

package main

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"log"
	"math/big"
	mathrand "math/rand"
	"net"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

type clientCertificate struct {
	Cert     tls.Certificate
	CertName string
	Expires  string
	Host     string
	Path     []string
	URL      string
}

type serverCertificate struct {
	host    string
	expires time.Time
	cert    string
}

// checkServerCert checks the Gemini server cert against known certs (TOFU).
func checkServerCert(u *url.URL, conn *tls.Conn) string {
	var warning string

	pc := serverCertificate{
		host:    u.Host,
		expires: conn.ConnectionState().PeerCertificates[0].NotAfter,
		cert:    base64.StdEncoding.EncodeToString(conn.ConnectionState().PeerCertificates[0].Raw),
	}

	muServerCerts.Lock()
	for i, c := range serverCerts {
		if c.host == pc.host {
			if c.cert == pc.cert {
				break
			} else {
				warning = fmt.Sprintf("The TLS certificate %s sent does not match the certificate it sent last time, which was set to expire on %v. However, we will proceed with the request, and trust the new certificate in the future.", c.host, c.expires)
				serverCerts[i].cert = pc.cert
				serverCerts[i].expires = pc.expires
				serverCertsChanged = true
			}
		} else {
			if i == len(serverCerts)-1 {
				serverCerts = append(serverCerts, pc)
				serverCertsChanged = true
			}
		}
	}
	if len(serverCerts) == 0 {
		serverCerts = append(serverCerts, pc)
		serverCertsChanged = true
	}
	muServerCerts.Unlock()

	return warning
}

// deleteClientCert removes the TLS client certificate from clientCerts that
// best matches URL u. Returns a non-nil error if no client cert matches the URL.
func deleteClientCert(u *url.URL) error {
	var err error
	var bestMatchIndex int
	var bestMatchScore int

	splitPath := strings.Split(u.Path, "/")

	muClientCerts.Lock()

	for i, c := range clientCerts {
		if u.Host != c.Host {
			continue
		}
		score := 1
		for i, p := range splitPath {
			if p == c.Path[i] {
				score++
			}
		}
		if score > bestMatchScore {
			bestMatchScore = score
			bestMatchIndex = i
		}
		if score > len(splitPath) {
			break
		}
	}

	if bestMatchScore > 0 {
		newCerts := make([]clientCertificate, 0, len(clientCerts))
		for i, c := range clientCerts {
			if i == bestMatchIndex {
				continue
			}
			newCerts = append(newCerts, c)
		}
		clientCerts = newCerts
		if optLogLevel > 1 {
			log.Printf("deleteClientCert: deleted client certificate for %s", u.String())
		}
	} else {
		err = fmt.Errorf("deleteClientCert: no certificate found matching URL '%s'", u.String())
	}
	muClientCerts.Unlock()

	return err
}

// makeCert returns a self-signed TLS certificate.
// If rsaBits is less than 2048 (e.g., 0), makeCert returns an ed25519 certificate.
func makeCert(starts time.Time, expires time.Time, name string, rsaBits int) (tls.Certificate, error) {
	var err error
	var priv interface{}
	selfCA := true

	if starts.IsZero() {
		starts = time.Now()
	}

	if rsaBits >= 2048 {
		priv, err = rsa.GenerateKey(rand.Reader, 2048)
	} else {
		_, priv, err = ed25519.GenerateKey(rand.Reader)
	}
	if err != nil {
		log.Println("makeCert: failed to generate private key:", err)
	}

	keyUsage := x509.KeyUsageDigitalSignature
	if _, isRSA := priv.(*rsa.PrivateKey); isRSA {
		keyUsage |= x509.KeyUsageKeyEncipherment
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		log.Println("makeCert: failed to generate serial number:", err)
	}

	if name == "" {
		ri, err := rand.Int(rand.Reader, big.NewInt(100000000))
		if err != nil {
			log.Println("makeCert: failed to generate big int for cert info:", err)
		}
		name = ri.String()
	}

	certInfo := x509.Certificate{
		BasicConstraintsValid: true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:              keyUsage,
		NotAfter:              expires,
		NotBefore:             starts,
		SerialNumber:          serialNumber,
		Subject: pkix.Name{
			Organization: []string{name},
			CommonName:   name,
		},
	}

	certInfo.DNSNames = append(certInfo.DNSNames, "localhost")
	if optAddr != "" {
		if ip := net.ParseIP(optAddr); ip != nil {
			certInfo.IPAddresses = append(certInfo.IPAddresses, ip)
		} else {
			certInfo.DNSNames = append(certInfo.DNSNames, optAddr)
		}
	} else {
		certInfo.IPAddresses = append(certInfo.IPAddresses, net.ParseIP("127.0.0.1"))
	}
	hn, err := os.Hostname()
	if err == nil {
		certInfo.DNSNames = append(certInfo.DNSNames, hn)
	}

	if selfCA {
		certInfo.IsCA = true
		certInfo.KeyUsage = certInfo.KeyUsage | x509.KeyUsageCertSign
	}

	x509Cert, err := x509.CreateCertificate(rand.Reader, &certInfo, &certInfo, publicKey(priv), priv)
	if err != nil {
		log.Fatalf("failed to create certificate: %v", err)
	}

	// Note: We've generate an x509 cert but need to return a TLS cert.
	// These are two different object types.
	//
	// https://stackoverflow.com/questions/34192230/how-to-turn-an-x509-certificate-into-a-tls-certificate-in-go
	// > But if you have an x509.Certificate, you already have a tls.Certificate;
	// > just put the x509.Certificate's Raw bytes into a tls.Certificate's Certificate slice.
	// > TLS servers will need the PrivateKey field set to successfully complete the handshake.
	// > I think the rest is optional.

	tlsCert := tls.Certificate{
		Certificate: [][]byte{x509Cert},
		PrivateKey:  priv,
	}

	return tlsCert, err
}

// matchClientCert returns the TLS client certificate from clientCerts that
// best matches URL u, or nil if none of the certificates match.
func matchClientCert(u *url.URL) tls.Certificate {
	var matchingCert tls.Certificate
	var bestMatchIndex int
	var bestMatchScore int

	splitPath := strings.Split(u.Path, "/")

	muClientCerts.RLock()
	for i, c := range clientCerts {
		if u.Host != c.Host {
			continue
		}
		score := 1
		for i, p := range splitPath {
			if p == c.Path[i] {
				score++
			}
		}
		if score > bestMatchScore {
			bestMatchScore = score
			bestMatchIndex = i
		}
		if score > len(splitPath) {
			break
		}
	}

	if bestMatchScore > 0 {
		matchingCert = clientCerts[bestMatchIndex].Cert
		if optLogLevel > 1 {
			log.Printf("matchCert: URL %s matched client certificate: %s%s",
				u.String(), clientCerts[bestMatchIndex].Host,
				strings.Join(clientCerts[bestMatchIndex].Path, "/"))
		}
	}
	muClientCerts.RUnlock()

	return matchingCert
}

func publicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case ed25519.PrivateKey:
		return k.Public().(ed25519.PublicKey)
	default:
		return nil
	}
}

// purgeOldClientCertificates removes expired certificates from clientCerts.
func purgeOldClientCertificates() {
	for {
		now := time.Now()
		expired := 0

		muClientCerts.Lock()
		freshCerts := make([]clientCertificate, 0, len(clientCerts))
		for _, c := range clientCerts {
			if now.Before(c.Cert.Leaf.NotAfter) {
				freshCerts = append(freshCerts, c)
			} else {
				expired++
			}
		}
		clientCerts = freshCerts
		muClientCerts.Unlock()

		if optLogLevel > 1 {
			log.Printf("purgeOldClientCertificates: purged %d expired certificates, kept %d certificates", expired, len(clientCerts))
		}

		time.Sleep(time.Hour)
	}
}

// saveClientCert adds a TLS client certificate to clientCerts.
func saveClientCert(u *url.URL, name string) {
	var err error
	var newCert clientCertificate

	newCert.URL = u.String()
	newCert.Host = u.Host
	newCert.Path = strings.Split(u.Path, "/")
	newCert.CertName = name
	starts := time.Now().Add(-time.Hour * time.Duration(24*(mathrand.Intn(100)+1)))
	expires := time.Now().Add(time.Hour * time.Duration(optHours))
	newCert.Expires = expires.String()
	newCert.Cert, err = makeCert(starts, expires, name, 2048)
	if err != nil {
		log.Println("proxyGemini: transient client cert generation failed:", err)
	}

	muClientCerts.Lock()
	clientCerts = append(clientCerts, newCert)
	muClientCerts.Unlock()
}

// saveTOFU saves known TLS server certificates to a file.
func saveTOFU() {
	d, err := os.UserCacheDir()
	if err != nil {
		optTrust = true
		log.Println("saveTOFU: unable to find cache directory, so certificate validation is disabled:", err)
	}
	tofuFile := path.Join(d, "gneto-tofu.txt")

	f, err := os.Open(tofuFile)
	if err != nil {
		log.Printf("saveTOFU: failed to read TOFU cache file '%s': %v", tofuFile, err)
	}
	scanner := bufio.NewScanner(f)
	muServerCerts.Lock()
	for scanner.Scan() {
		split := strings.Split(scanner.Text(), " ")
		exp, err := time.Parse(time.RFC3339, split[1])
		if len(split) == 3 && err == nil {
			c := serverCertificate{
				host:    split[0],
				expires: exp,
				cert:    split[2],
			}
			serverCerts = append(serverCerts, c)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("saveTOFU: failed reading line from '%s': %v", tofuFile, err)
	}
	muServerCerts.Unlock()
	f.Close()

	for {
		now := time.Now()
		if serverCertsChanged {
			muServerCerts.Lock()
			certs := make([]serverCertificate, 0, len(serverCerts))
			for _, c := range serverCerts {
				if now.After(c.expires) {
					continue
				}
				certs = append(certs, c)
			}
			serverCerts = certs
			muServerCerts.Unlock()

			f, err := os.Create(tofuFile)
			if err != nil {
				log.Printf("saveTOFU: failed to open TOFU cache file '%s' for writing: %v", tofuFile, err)
				continue
			}
			muServerCerts.RLock()
			for _, c := range serverCerts {
				fmt.Fprintf(f, "%s %s %s\n", c.host, c.expires.Format(time.RFC3339), c.cert)
			}
			muServerCerts.RUnlock()
			f.Close()
		}

		time.Sleep(10 * time.Minute)
	}
}
