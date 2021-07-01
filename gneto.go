// Copyright 2020 Paul Gorman. Licensed under the GPL.

// Gneto makes Gemini pages available over HTTP.
//
// See the Project Gemini documentation and spec at:
// https://gemini.circumlunar.space/docs/
// gemini://gemini.circumlunar.space/docs/

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"html/template"
	"io/ioutil"
	"log"
	mathrand "math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var muClientCerts sync.RWMutex
var clientCerts []clientCertificate
var clientCertsChanged bool
var muCookies sync.RWMutex
var cookies []http.Cookie
var errRedirect error
var envPassword string
var maxRedirects int
var maxCookieLife time.Duration
var optAddr string
var optCertFile string
var optClientCertsFile string
var optCSSFile string
var optHomeFile string
var optHours int
var optKeyFile string
var optLang string
var optLogLevel int
var optPort string
var optRobots string
var optTextOnly bool
var optTrust bool
var muServerCerts sync.RWMutex
var serverCerts []serverCertificate
var serverCertsChanged bool
var reCharset *regexp.Regexp
var reGemBlank *regexp.Regexp
var reGemH1 *regexp.Regexp
var reGemH2 *regexp.Regexp
var reGemH3 *regexp.Regexp
var reGemLink *regexp.Regexp
var reGemList *regexp.Regexp
var reGemPre *regexp.Regexp
var reGemResponseHeader *regexp.Regexp
var reGemQuote *regexp.Regexp
var reLang *regexp.Regexp
var reStatus *regexp.Regexp
var tmpls *template.Template

type templateData struct {
	Certs       []clientCertificate
	Charset     string
	Count       int
	Error       string
	HTML        template.HTML
	Lang        string
	Logout      bool
	ManageCerts bool
	Meta        string
	Title       string
	URL         string
	Warning     string
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

		if optLogLevel > 1 {
			log.Printf("purgeOldCookies: purged %d stale cookies, kept %d cookies", stale, len(cookies))
		}

		time.Sleep(time.Hour)
	}
}

func init() {
	mathrand.Seed(time.Now().Unix())

	envPassword, _ = os.LookupEnv("password")
	if envPassword != "" {
		cookies = make([]http.Cookie, 0, 12)
	}

	flag.StringVar(&optAddr, "addr", "127.0.0.1", "IP address on which to serve web interface")
	flag.StringVar(&optCertFile, "cert", "", "TLS certificate file for web interface")
	flag.StringVar(&optClientCertsFile, "clientcerts", "", "path to JSON file listing peristent TLS client certificates")
	flag.StringVar(&optCSSFile, "css", "./web/gneto.css", "path to cascading style sheets file")
	flag.IntVar(&optLogLevel, "loglevel", 0, "print debugging output; 0=errors only, 1=verbose, 2=very verbose, 3=very very verbose")
	flag.StringVar(&optHomeFile, "home", "", "Gemini file to show on home page")
	flag.IntVar(&optHours, "hours", 72, "hours until transient client TLS certificates expire (zero disables client certs)")
	flag.StringVar(&optKeyFile, "key", "", "TLS key file for web interface")
	flag.StringVar(&optLang, "lang", "en-US", "RFC4646 language for pages that do not supply one")
	flag.IntVar(&maxRedirects, "r", 5, "maximum redirects to follow")
	flag.StringVar(&optPort, "port", "8065", "port on which to serve web interface")
	flag.StringVar(&optRobots, "robots", "./web/robots.txt", "path to robots.txt file")
	flag.BoolVar(&optTextOnly, "textonly", false, "refuse to proxy non-text file types")
	flag.BoolVar(&optTrust, "trust", false, "don't warn about TLS certificate changes for visited Gemini sites")
	flag.Parse()

	if optHomeFile != "" {
		p, err := filepath.Abs(optHomeFile)
		if err == nil {
			optHomeFile = p
		}
	}

	if optAddr != "127.0.0.1" && (optHours != 0 || envPassword == "") {
		log.Println("warning: review the Security Considerations in README.m, and consider settign the 'password' environment variable")
	}

	templateFiles := []string{
		"./web/home.html.tmpl",
		"./web/footer.html.tmpl",
		"./web/footer-only.html.tmpl",
		"./web/header.html.tmpl",
		"./web/header-only.html.tmpl",
		"./web/help.html.tmpl",
		"./web/input.html.tmpl",
		"./web/login.html.tmpl",
		"./web/certificate.html.tmpl",
		"./web/certificates.html.tmpl",
	}
	tmpls = template.Must(template.ParseFiles(templateFiles...))

	reCharset = regexp.MustCompile(`\bcharset=([\w-]+)`)
	reGemBlank = regexp.MustCompile(`^\s*$`)
	reGemH1 = regexp.MustCompile(`^#\s*([^#].*)\s*`)
	reGemH2 = regexp.MustCompile(`^##\s*([^#].*)\s*`)
	reGemH3 = regexp.MustCompile(`^###\s*([^#].*)\s*`)
	reGemLink = regexp.MustCompile(`^=>\s*(\S*)\s*(.*)`)
	reGemList = regexp.MustCompile(`^\*\s(.*)\s*`)
	reGemPre = regexp.MustCompile("^```(.*)")
	reGemResponseHeader = regexp.MustCompile(`^\d{2} (.*)\r\n`)
	reGemQuote = regexp.MustCompile(`^>\s(.*)\s*`)
	reLang = regexp.MustCompile(`\blang=([\w-]+)`)
	reStatus = regexp.MustCompile(`\d\d .*`)

	maxCookieLife = 90 * 24 * time.Hour

	if !optTrust {
		serverCertsChanged = false
		serverCerts = make([]serverCertificate, 0, 500)
	}

	clientCerts = make([]clientCertificate, 0, 500)

	if optClientCertsFile != "" {
		var pCerts []persistentCert

		jc, err := ioutil.ReadFile(optClientCertsFile)
		if err != nil {
			log.Printf("init: failed to read persistent TLS client certificates from JSON file '%s': %v", optClientCertsFile, err)
		} else {
			err := json.Unmarshal(jc, &pCerts)
			if err != nil {
				log.Printf("init: failed to unmarshal JSON client certificates from '%s': %v", optClientCertsFile, err)
			}
		}

		for _, pc := range pCerts {
			var c clientCertificate
			u, err := url.Parse(pc.URL)
			if err != nil {
				log.Printf("init: failed to parse URL %s in client certificate file '%s': %v", c.URL, optClientCertsFile, err)
				continue
			}
			c.URL = pc.URL
			c.Cert, err = tls.X509KeyPair([]byte(pc.CertPEM), []byte(pc.KeyPEM))
			if err != nil {
				log.Printf("init: failed to parse client certificate PEM data for %s: %v", c.URL, err)
				continue
			}
			c.Leaf, err = x509.ParseCertificate(c.Cert.Certificate[0])
			if err != nil {
				log.Printf("init: failed to parse certificate leaf for %s: %v", pc.URL, err)
				continue
			}
			c.Expires = c.Leaf.NotAfter.String()
			c.CertName = c.Leaf.Subject.CommonName
			c.Host = u.Host
			c.Path = strings.Split(u.Path, "/")

			clientCerts = append(clientCerts, c)
		}
	}
}

func main() {
	if envPassword != "" {
		go purgeOldCookies()
	}

	if !optTrust {
		go saveTOFU()
	}

	if optHours > 0 {
		go purgeOldClientCertificates()
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", proxy)
	mux.HandleFunc("/certificate", clientCertificateRequired)
	mux.HandleFunc("/settings/certificates", manageClientCertificates)
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
		if optLogLevel > 0 {
			log.Printf("main: starting HTTPS server on %s", optAddr+":"+optPort)
		}
		err := http.ListenAndServeTLS(optAddr+":"+optPort, optCertFile, optKeyFile, mux)
		if err != nil {
			log.Fatalf("main: could not start HTTPS server: %v", err)
		}
	}

	if optLogLevel > 0 {
		log.Printf("main: serving insecure HTTP server on %s", optAddr+":"+optPort)
	}
	err := http.ListenAndServe(optAddr+":"+optPort, mux)
	if err != nil {
		log.Fatalf("main: could not start HTTPS server: %v", err)
	}

}
