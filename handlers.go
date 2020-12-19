// Copyright 2020 Paul Gorman.

// Gneto makes Gemini pages available over HTTP.

package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
)

// proxy handles requests not covered by another handler.
func proxy(w http.ResponseWriter, r *http.Request) {
	var err error
	var targetURL string
	var u *url.URL

	if r.Method == http.MethodPost && r.FormValue("url") != "" {
		targetURL = r.FormValue("url")

		if r.FormValue("input") != "" {
			targetURL = targetURL + "?" + url.QueryEscape(r.FormValue("input"))
			if optVerbose || optDebug {
				log.Println("proxy: submitting Gemini input:", targetURL)
			}
		}

		// TODO: Test everything to not show secrent in web interface or logs.
		if r.FormValue("secret") != "" {
			targetURL = targetURL + "?" + url.QueryEscape(r.FormValue("secret"))
			if optVerbose || optDebug {
				log.Printf("proxy: submitting Gemini sensitive input: %s?REDACTED_SECRET", r.FormValue("url"))
			}
		}

		http.Redirect(w, r, "/?url="+url.QueryEscape(targetURL), http.StatusFound)
	}

	if r.URL.Query().Get("url") == "" {
		var td templateData
		td.Title = "Gneto"
		err = tmpls.ExecuteTemplate(w, "home.html.tmpl", td)
		if err != nil {
			log.Println("proxy", err)
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}

	u, err = url.Parse(r.URL.Query().Get("url"))
	if err != nil {
		err = fmt.Errorf("proxy: failed to parse URL: %v", err)
		log.Println(err)
		http.Error(w, err.Error(), 500)
	}

	if u.Scheme == "gemini" {
		for i := 0; i <= maxRedirects; i++ {
			u, err = proxyGemini(w, r, u)
			if u != nil && u.Scheme != "gemini" {
				http.Redirect(w, r, u.String(), http.StatusFound)
			}
			if err != nil && errors.Is(err, errRedirect) {
				if i < maxRedirects-1 {
					log.Printf("proxy: redirecting to %s\n", err)
					continue
				} else {
					err = fmt.Errorf("too many redirects, ending at %s", u.String())
					i = maxRedirects + 1
					break
				}
			}
			if err != nil {
				log.Println(err)
			}
			break
		}
	} else {
		err = fmt.Errorf("proxy: proxying of %s not supported (%s)", u.Scheme, u.String())
	}

	if err != nil {
		if optDebug {
			log.Println(err)
		}
		var td templateData
		td.Error = err.Error()
		td.URL = u.String()
		td.Title = "Gneto " + td.URL
		err = tmpls.ExecuteTemplate(w, "home.html.tmpl", td)
		if err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", 500)
		}
	}
}
