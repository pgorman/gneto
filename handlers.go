// Copyright 2020 Paul Gorman.

// Gneto makes Gemini pages available over HTTP.

package main

import (
	cryptorand "crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"
	"time"
)

// login displays the page requesting a password.
func login(w http.ResponseWriter, r *http.Request) {
	var err error

	if r.Method == http.MethodPost && envPassword != "" {
		if envPassword == r.FormValue("password") {
			b := make([]byte, 32)
			_, err = cryptorand.Read(b)
			if err != nil {
				log.Println(err)
			}
			c := http.Cookie{
				Name:     "session",
				Value:    base64.StdEncoding.EncodeToString(b),
				Expires:  time.Now().Add(90 * 24 * time.Hour),
				HttpOnly: true,
			}
			muCookies.Lock()
			cookies = append(cookies, c)
			muCookies.Unlock()
			http.SetCookie(w, &c)
			if optDebug {
				log.Println("login: new login from", r.RemoteAddr)
			}
			http.Redirect(w, r, "/", http.StatusFound)
		} else {
			log.Println("login: failed login from", r.RemoteAddr)
		}
	}

	var td templateData
	td.Title = "Gneto Login"
	err = tmpls.ExecuteTemplate(w, "login.html.tmpl", td)
	if err != nil {
		log.Println("login:", err)
		http.Error(w, "Internal Server Error", 500)
	}
}

// logout deletes a session cookie.
func logout(w http.ResponseWriter, r *http.Request) {
	if envPassword == "" {
		return
	}

	rc, err := r.Cookie("session")
	if err == nil {
		muCookies.Lock()
		defer muCookies.Unlock()
		tc := make([]http.Cookie, len(cookies), len(cookies))
		for _, c := range cookies {
			if c.Value == rc.Value {
				if optDebug {
					log.Println("logout: removing cookie:", c.Value)
				}
				continue
			}
			tc = append(tc, c)
		}
		cookies = tc
	}

	http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
}

// proxy handles requests not covered by another handler.
func proxy(w http.ResponseWriter, r *http.Request) {
	if !authenticate(r) {
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
	}

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

		// TODO: Test everything to not show secret in web interface or logs.
		if r.FormValue("secret") != "" {
			targetURL = targetURL + "?" + url.QueryEscape(r.FormValue("secret"))
			if optVerbose || optDebug {
				log.Printf("proxy: submitting Gemini sensitive input: %s?REDACTED_SECRET", r.FormValue("url"))
			}
		}

		http.Redirect(w, r, "/?url="+url.QueryEscape(targetURL), http.StatusFound)
	}

	if r.URL.Query().Get("url") == "" {
		if optHomeFile != "" {
			// TODO: Custom home page.
			u, err := url.Parse(path.Join("file://", optHomeFile))
			if err != nil {
				log.Println("proxy: failed to parse home file path to URL:", err)
			}
			proxyGemini(w, r, u)
		} else {
			var td templateData
			td.Title = "Gneto"
			if envPassword != "" {
				td.Logout = true
			}
			err = tmpls.ExecuteTemplate(w, "home.html.tmpl", td)
			if err != nil {
				log.Println("proxy:", err)
				http.Error(w, "Internal Server Error", 500)
			}
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
		if envPassword != "" {
			td.Logout = true
		}
		err = tmpls.ExecuteTemplate(w, "home.html.tmpl", td)
		if err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", 500)
		}
	}
}
