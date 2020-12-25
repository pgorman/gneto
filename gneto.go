// Copyright 2020 Paul Gorman. Licensed under the GPL.

// Gneto makes Gemini pages available over HTTP.
//
// See the Project Gemini documentation and spec at:
// https://gemini.circumlunar.space/docs/
// gemini://gemini.circumlunar.space/docs/

package main

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

var muPeerCerts sync.RWMutex
var peerCerts []certificate
var peerCertsChanged bool
var muCookies sync.RWMutex
var cookies []http.Cookie
var errRedirect error
var envPassword string
var maxRedirects int
var maxCookieLife time.Duration
var optAddr string
var optCertFile string
var optCSSFile string
var optDebug bool
var optHomeFile string
var optKeyFile string
var optPort string
var optRobots string
var optTrust bool
var optVerbose bool
var reGemBlank *regexp.Regexp
var reGemH1 *regexp.Regexp
var reGemH2 *regexp.Regexp
var reGemH3 *regexp.Regexp
var reGemLink *regexp.Regexp
var reGemList *regexp.Regexp
var reGemPre *regexp.Regexp
var reGemQuote *regexp.Regexp
var reStatus *regexp.Regexp
var tmpls *template.Template

type certificate struct {
	host    string
	expires time.Time
	cert    string
}

type templateData struct {
	HTML    template.HTML
	Error   string
	Logout  bool
	Meta    string
	Title   string
	URL     string
	Warning string
}

// authenticate checks for a valid session cookie.
func authenticate(r *http.Request) bool {
	auth := false

	if envPassword == "" {
		auth = true
	} else {
		rc, err := r.Cookie("session")
		if err == nil {
			muCookies.RLock()
			defer muCookies.RUnlock()
			for _, c := range cookies {
				if c.Value == rc.Value {
					auth = true
				}
			}
		}
	}

	return auth
}

// absoluteURL makes lineURL absolute using, if necessary, the host and path of baseURL.
func absoluteURL(baseURL *url.URL, lineURL string) (*url.URL, error) {
	var err error

	u, err := url.Parse(lineURL)
	if err != nil {
		return u, err
	}

	return baseURL.ResolveReference(u), err
}

// checkCert checks the Gemini server cert against known certs (TOFU).
func checkCert(u *url.URL, conn *tls.Conn) string {
	var warning string

	pc := certificate{
		host:    u.Host,
		expires: conn.ConnectionState().PeerCertificates[0].NotAfter,
		cert:    base64.StdEncoding.EncodeToString(conn.ConnectionState().PeerCertificates[0].Raw),
	}

	muPeerCerts.Lock()
	for i, c := range peerCerts {
		if c.host == pc.host {
			if c.cert == pc.cert {
				break
			} else {
				warning = fmt.Sprintf("The TLS certificate %s sent does not match the certificate it sent last time. However, we will proceed with the request, and trust the new certificate in the future.", c.host)
				peerCerts[i].cert = pc.cert
				peerCerts[i].expires = pc.expires
				peerCertsChanged = true
			}
		} else {
			if i == len(peerCerts)-1 {
				peerCerts = append(peerCerts, pc)
				peerCertsChanged = true
			}
		}
	}
	if len(peerCerts) == 0 {
		peerCerts = append(peerCerts, pc)
		peerCertsChanged = true
	}
	muPeerCerts.Unlock()

	return warning
}

// purgeOldCookies removes cookies older than maxCookieLife from cookies.
func purgeOldCookies() {
	for {
		now := time.Now()
		stale := 0

		muCookies.Lock()
		freshCookies := make([]http.Cookie, 0, len(cookies))
		for _, c := range cookies {
			if now.Sub(c.Expires) < maxCookieLife {
				freshCookies = append(freshCookies, c)
			} else {
				stale++
			}
		}
		cookies = freshCookies
		muCookies.Unlock()

		if optVerbose {
			log.Printf("purgeOldCookies: purged %d stale cookies, kept %d cookies", stale, len(cookies))
		}

		time.Sleep(time.Hour)
	}
}

// saveTOFU saves known TLS certificates to a file.
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
	muPeerCerts.Lock()
	for scanner.Scan() {
		split := strings.Split(scanner.Text(), " ")
		exp, err := time.Parse(time.RFC3339, split[1])
		if len(split) == 3 && err == nil {
			c := certificate{
				host:    split[0],
				expires: exp,
				cert:    split[2],
			}
			peerCerts = append(peerCerts, c)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("saveTOFU: failed reading line from '%s': %v", tofuFile, err)
	}
	muPeerCerts.Unlock()
	f.Close()

	for {
		now := time.Now()
		if peerCertsChanged {
			muPeerCerts.Lock()
			certs := make([]certificate, 0, len(peerCerts))
			for _, c := range peerCerts {
				if now.After(c.expires) {
					continue
				}
				certs = append(certs, c)
			}
			peerCerts = certs
			muPeerCerts.Unlock()

			f, err := os.Create(tofuFile)
			if err != nil {
				log.Printf("saveTOFU: failed to open TOFU cache file '%s' for writing: %v", tofuFile, err)
				continue
			}
			muPeerCerts.RLock()
			for _, c := range peerCerts {
				fmt.Fprintf(f, "%s %s %s\n", c.host, c.expires.Format(time.RFC3339), c.cert)
			}
			muPeerCerts.RUnlock()
			f.Close()
		}

		time.Sleep(10 * time.Minute)
	}
}

func init() {
	envPassword, _ = os.LookupEnv("password")
	if envPassword != "" {
		cookies = make([]http.Cookie, 0, 12)
	}

	flag.StringVar(&optAddr, "addr", "127.0.0.1", "IP address on which to serve web interface")
	flag.StringVar(&optCertFile, "cert", "", "TLS certificate file for web interface")
	flag.StringVar(&optCSSFile, "css", "./web/gneto.css", "path to cascading style sheets file")
	flag.BoolVar(&optDebug, "debug", false, "print very verbose debugging output")
	flag.StringVar(&optHomeFile, "home", "", "Gemini file to show on home page")
	flag.StringVar(&optKeyFile, "key", "", "TLS key file for web interface")
	flag.IntVar(&maxRedirects, "r", 5, "maximum redirects to follow")
	flag.StringVar(&optPort, "port", "8065", "port on which to serve web interface")
	flag.StringVar(&optRobots, "robots", "./web/robots.txt", "path to robots.txt file")
	flag.BoolVar(&optTrust, "trust", false, "don't warn about TLS certificate changes for visited Gemini sites")
	flag.BoolVar(&optVerbose, "v", false, "print verbose console messages")
	flag.Parse()

	templateFiles := []string{
		"./web/home.html.tmpl",
		"./web/footer.html.tmpl",
		"./web/footer-only.html.tmpl",
		"./web/header.html.tmpl",
		"./web/header-only.html.tmpl",
		"./web/help.html.tmpl",
		"./web/input.html.tmpl",
		"./web/login.html.tmpl",
	}
	tmpls = template.Must(template.ParseFiles(templateFiles...))

	reGemBlank = regexp.MustCompile(`^\s*$`)
	reGemH1 = regexp.MustCompile(`^#[^#]\s*(.*)\s*`)
	reGemH2 = regexp.MustCompile(`^##[^#]\s*(.*)\s*`)
	reGemH3 = regexp.MustCompile(`^###[^#]\s*(.*)\s*`)
	reGemLink = regexp.MustCompile(`^=>\s*(\S*)\s*(.*)`)
	reGemList = regexp.MustCompile(`^\*\s(.*)\s*`)
	reGemPre = regexp.MustCompile("^```(.*)")
	reGemQuote = regexp.MustCompile(`^>\s(.*)\s*`)
	reStatus = regexp.MustCompile(`\d\d .*`)

	maxCookieLife = 90 * 24 * time.Hour

	if !optTrust {
		peerCertsChanged = false
		peerCerts = make([]certificate, 0, 500)
	}
}

func main() {
	if envPassword != "" {
		go purgeOldCookies()
	}

	if !optTrust {
		go saveTOFU()
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", proxy)
	mux.HandleFunc("/login", login)
	mux.HandleFunc("/logout", logout)
	mux.HandleFunc("/gneto.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, optCSSFile)
	})
	mux.HandleFunc("/help.html", func(w http.ResponseWriter, r *http.Request) {
		var td templateData
		td.Title = "Gneto Help"
		err := tmpls.ExecuteTemplate(w, "help.html.tmpl", td)
		if err != nil {
			log.Println("main:", err.Error())
			http.Error(w, "Internal Server Error", 500)
		}
	})
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, optRobots)
	})

	if optCertFile != "" && optKeyFile != "" {
		if optVerbose {
			log.Printf("main: starting HTTPS server on %s", optAddr+":"+optPort)
		}
		err := http.ListenAndServeTLS(optAddr+":"+optPort, optCertFile, optKeyFile, mux)
		if err != nil {
			log.Fatalf("main: could not start HTTPS server: %v", err)
		}
	}

	if optVerbose {
		log.Printf("main: serving insecure HTTP server on %s", optAddr+":"+optPort)
	}
	err := http.ListenAndServe(optAddr+":"+optPort, mux)
	if err != nil {
		log.Fatalf("main: could not start HTTPS server: %v", err)
	}

}
