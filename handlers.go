// Copyright 2020 Paul Gorman.

// Gneto makes Gemini pages available over HTTP.

package main

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
)

type tableData struct {
	Gemini template.HTML
	Error  string
	Title  string
	URL    string
}

// home handles home/start page requests.
func home(w http.ResponseWriter, r *http.Request) {
	var err error
	var gemini []string
	var u string

	if r.Method == http.MethodPost {
		u = r.FormValue("url")
		http.Redirect(w, r, "/?url="+url.QueryEscape(u), http.StatusFound)
	}

	if u == "" {
		u = r.URL.Query().Get("url")
	}

	if u != "" {
		r := 0
		for r <= maxRedirects {
			gemini, err = getGemini(u)
			if err != nil && errors.Is(err, errRedirect) {
				if r < maxRedirects-1 {
					log.Printf("redirecting to %s\n", err)
					u = fmt.Sprintf("%s", err)
					r++
					continue
				} else {
					err = fmt.Errorf("too many redirects, ending at %s\n", err)
					r = maxRedirects + 1
					break
				}
			}
			if err != nil {
				log.Println(err)
			}
			break
		}
	}

	var td tableData
	if err != nil {
		td.Error = err.Error()
	}
	td.URL = u
	td.Title = "Gneto " + td.URL
	td.Gemini = template.HTML(geminiToHTML(u, gemini))

	err = tmpls.ExecuteTemplate(w, "home.html.tmpl", td)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, "Internal Server Error", 500)
	}
}
