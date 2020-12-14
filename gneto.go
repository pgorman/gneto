// Copyright 2020 Paul Gorman.

// Gneto makes Gemini pages available over HTTP.

package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var errRedirect error
var maxRedirects int
var optAddr string
var optCertFile string
var optCSSFile string
var optKeyFile string
var optPort string
var optVerbose bool
var reStatus *regexp.Regexp
var status bool
var tmpls *template.Template

// getGemini fetches a Gemini file from URL g.
// getGemini expects a URL like gemini://tilde.team/.
func getGemini(g string) ([]string, error) {
	var gemini = make([]string, 0, 500)

	u, err := url.Parse(g)
	if err != nil {
		return gemini, fmt.Errorf("get: can't parse URL %s: %v", g, err)
	}

	var port string
	if u.Port() != "" {
		port = u.Port()
	} else {
		port = "1965"
	}

	conn, err := tls.Dial("tcp", u.Hostname()+":"+port, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		return gemini, fmt.Errorf("get: tls.Dial error to %s: %v", g, err)
	}
	defer conn.Close()

	fmt.Fprintf(conn, u.String()+"\r\n")

	scanner := bufio.NewScanner(conn)
	l := 0
	for scanner.Scan() {
		s := scanner.Text()
		if l == 0 {
			if !reStatus.MatchString(s) {
				return gemini, fmt.Errorf("get: invalid status line: %s", s)
			}
			l++
			if status {
				fmt.Println(s)
			}
			switch s[0] {
			case "2"[0]:
				continue
			case "3"[0]:
				ru, err := url.Parse(strings.SplitAfterN(s, " ", 2)[1])
				if err != nil {
					return gemini, fmt.Errorf("get: can't parse redirect URL %s: %v", strings.SplitAfterN(s, " ", 2)[1], err)
				}
				if ru.Host == "" {
					ru.Host = u.Host
				}
				if ru.Scheme == "" {
					ru.Scheme = u.Scheme
				}
				errRedirect = errors.New(ru.String())
				return gemini, errRedirect
			default:
				return gemini, fmt.Errorf("get: status response: %s", s)
			}
		}
		gemini = append(gemini, s)
		l++
	}
	if err := scanner.Err(); err != nil {
		return gemini, fmt.Errorf("get: scanning server response for %s: %v", g, err)
	}

	return gemini, nil
}

func init() {
	flag.StringVar(&optAddr, "addr", "127.0.0.1", "IP address on which to serve web interface")
	flag.StringVar(&optCertFile, "cert", "", "TLS certificate file")
	flag.StringVar(&optCSSFile, "css", "./web/gneto.css", "path to cascading sytle sheets file")
	flag.StringVar(&optKeyFile, "key", "", "TLS key file")
	flag.IntVar(&maxRedirects, "r", 5, "maximum redirects to follow")
	flag.StringVar(&optPort, "port", "8065", "port on which to serve web interface")
	flag.BoolVar(&optVerbose, "v", false, "print verbose console messages")
	flag.Parse()

	templateFiles := []string{
		"./web/home.html.tmpl",
		"./web/footer.html.tmpl",
		"./web/header.html.tmpl",
		"./web/help.html.tmpl",
	}
	tmpls = template.Must(template.ParseFiles(templateFiles...))

	reStatus = regexp.MustCompile(`\d\d .*`)
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", home)
	mux.HandleFunc("/gneto.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, optCSSFile)
	})
	mux.HandleFunc("/help.html", func(w http.ResponseWriter, r *http.Request) {
		err := tmpls.ExecuteTemplate(w, "help.html.tmpl", nil)
		if err != nil {
			log.Println(err.Error())
			http.Error(w, "Internal Server Error", 500)
		}
	})

	if optCertFile != "" && optKeyFile != "" {
		if optVerbose {
			log.Printf("starting HTTPS server on %s", optAddr+":"+optPort)
		}
		err := http.ListenAndServeTLS(optAddr+":"+optPort, optCertFile, optKeyFile, mux)
		if err != nil {
			log.Fatalf("main: could not start HTTPS server: %v", err)
		}
	}

	if optVerbose {
		log.Printf("serving insecure HTTP server on %s", optAddr+":"+optPort)
	}
	err := http.ListenAndServe(optAddr+":"+optPort, mux)
	if err != nil {
		log.Fatalf("main: could not start HTTPS server: %v", err)
	}

}
