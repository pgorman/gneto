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
	var u *url.URL

	if r.Method == http.MethodPost && r.FormValue("url") != "" {
		http.Redirect(w, r, "/?url="+url.QueryEscape(r.FormValue("url")), http.StatusFound)
	}

	if r.URL.Query().Get("url") == "" {
		var td templateData
		if err != nil {
			td.Error = err.Error()
		}
		td.URL = ""
		td.Title = "Gneto " + td.URL
		err = tmpls.ExecuteTemplate(w, "home.html.tmpl", td)
		if err != nil {
			log.Println(err)
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
			log.Println("possible redirects, iteration", i)
			u, err = proxyGemini(w, u)
			if u.Scheme != "gemini" {
				// Redirect to home?
				http.Redirect(w, r, u.String(), http.StatusFound)
			}
			if err != nil && errors.Is(err, errRedirect) {
				if i < maxRedirects-1 {
					log.Printf("redirecting to %s\n", err)
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
	}

	if err != nil {
		var td templateData
		if err != nil {
			td.Error = err.Error()
		}
		td.URL = u.String()
		td.Title = "Gneto " + td.URL
		err = tmpls.ExecuteTemplate(w, "home.html.tmpl", td)
		if err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", 500)
		}
	}
}
