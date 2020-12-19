// Copyright 2020 Paul Gorman.

// Gneto makes Gemini pages available over HTTP.

package main

import (
	"flag"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"regexp"
)

var errRedirect error
var maxRedirects int
var optAddr string
var optCertFile string
var optCSSFile string
var optDebug bool
var optKeyFile string
var optPort string
var optRobots string
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

type templateData struct {
	HTML  template.HTML
	Error string
	Title string
	URL   string
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

func init() {
	flag.StringVar(&optAddr, "addr", "127.0.0.1", "IP address on which to serve web interface")
	flag.StringVar(&optCertFile, "cert", "", "TLS certificate file")
	flag.StringVar(&optCSSFile, "css", "./web/gneto.css", "path to cascading style sheets file")
	flag.BoolVar(&optDebug, "debug", false, "print very verbose debugging output")
	flag.StringVar(&optKeyFile, "key", "", "TLS key file")
	flag.IntVar(&maxRedirects, "r", 5, "maximum redirects to follow")
	flag.StringVar(&optPort, "port", "8065", "port on which to serve web interface")
	flag.StringVar(&optRobots, "robots", "./web/robots.txt", "path to robots.txt file")
	flag.BoolVar(&optVerbose, "v", false, "print verbose console messages")
	flag.Parse()

	templateFiles := []string{
		"./web/home.html.tmpl",
		"./web/footer.html.tmpl",
		"./web/footer-only.html.tmpl",
		"./web/header.html.tmpl",
		"./web/header-only.html.tmpl",
		"./web/help.html.tmpl",
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
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", proxy)
	mux.HandleFunc("/gneto.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, optCSSFile)
	})
	mux.HandleFunc("/help.html", func(w http.ResponseWriter, r *http.Request) {
		err := tmpls.ExecuteTemplate(w, "help.html.tmpl", nil)
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
