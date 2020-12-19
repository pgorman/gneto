// Copyright 2020 Paul Gorman.

// Gneto makes Gemini pages available over HTTP.
//
// See the Project Gemini documentation and spec at:
// https://gemini.circumlunar.space/docs/specification.html
// gemini://gemini.circumlunar.space/docs/

package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// geminiToHTML reads Gemini text from rd, and writes its HTML equivalent to w.
// The source URL is stored in u.
func geminiToHTML(w http.ResponseWriter, u *url.URL, rd *bufio.Reader) error {
	var err error
	list := false
	pre := false

	var td templateData
	td.URL = u.String()
	td.Title = "Gneto " + td.URL
	err = tmpls.ExecuteTemplate(w, "header-only.html.tmpl", td)
	if err != nil {
		log.Println("geminiToHTML:", err)
		http.Error(w, "Internal Server Error", 500)
	}

	var eof error
	var line string
	for eof == nil {
		line, eof = rd.ReadString("\n"[0])
		if optDebug {
			fmt.Println(line)
		}

		if reGemPre.MatchString(line) {
			if list {
				list = false
				io.WriteString(w, "</ul>\n")
			}
			if pre == true {
				pre = false
				io.WriteString(w, "</pre>\n")
				continue
			} else {
				pre = true
				// How can we provide alt text from reGemPre.FindStringSubmatch(line)[1]?
				io.WriteString(w, "<pre>\n")
				continue
			}
		} else {
			if pre == true {
				io.WriteString(w, strings.ReplaceAll(line, "<", "&lt;"))
				continue
			}
		}

		if reGemBlank.MatchString(line) {
			io.WriteString(w, "<br>\n")
		} else if reGemH1.MatchString(line) {
			if list == true {
				list = false
				io.WriteString(w, "</ul>\n")
			}
			io.WriteString(w, "<h1>"+reGemH1.FindStringSubmatch(line)[1]+"</h1>\n")
		} else if reGemH2.MatchString(line) {
			if list == true {
				list = false
				io.WriteString(w, "</ul>\n")
			}
			io.WriteString(w, "<h2>"+reGemH2.FindStringSubmatch(line)[1]+"</h2>\n")
		} else if reGemH3.MatchString(line) {
			if list == true {
				list = false
				io.WriteString(w, "</ul>\n")
			}
			io.WriteString(w, "<h3>"+reGemH3.FindStringSubmatch(line)[1]+"</h3>\n")
		} else if reGemLink.MatchString(line) {
			if list == true {
				list = false
				io.WriteString(w, "</ul>\n")
			}

			link := reGemLink.FindStringSubmatch(line)
			lineURL, err := absoluteURL(u, link[1])
			if err != nil {
				io.WriteString(w, "<p>"+line+"</p>\n")
			}
			link[1] = lineURL.String()

			if lineURL.Scheme == "gemini" {
				if link[2] != "" {
					io.WriteString(w, `<p><a href="/?url=`+url.QueryEscape(link[1])+`">`+link[2]+
						`</a> <span class="scheme"><a href="`+link[1]+`">[`+lineURL.Scheme+`]</a></span></p>`+"\n")
				} else {
					io.WriteString(w, `<p><a href="/?url=`+url.QueryEscape(link[1])+`">`+link[1]+
						`</a> <span class="scheme"><a href="`+link[1]+`">[`+lineURL.Scheme+`]</a></span></p>`+"\n")
				}
			} else {
				if link[2] != "" {
					io.WriteString(w, `<p><a href="`+link[1]+`">`+link[2]+
						`</a> <span class="scheme"><a href="`+link[1]+`">[`+lineURL.Scheme+`]</a></span></p>`+"\n")
				} else {
					io.WriteString(w, `<p><a href="`+link[1]+`">`+link[1]+
						`</a> <span class="scheme"><a href="`+link[1]+`">[`+lineURL.Scheme+`]</a></span></p>`+"\n")
				}
			}
		} else if reGemList.MatchString(line) {
			if list == false {
				list = true
				io.WriteString(w, "<ul>")
			}
			io.WriteString(w, "<li>"+reGemList.FindStringSubmatch(line)[1]+"</li>\n")
		} else if reGemQuote.MatchString(line) {
			if list == true {
				list = false
				io.WriteString(w, "</ul>")
			}
			io.WriteString(w, "<blockquote>"+reGemQuote.FindStringSubmatch(line)[1]+"</blockquote>\n")
		} else {
			if list {
				list = false
				io.WriteString(w, "</ul>\n")
			}
			io.WriteString(w, line+"<br>\n")
		}
	}

	err = tmpls.ExecuteTemplate(w, "footer-only.html.tmpl", td)
	if err != nil {
		log.Println("geminiToHTML:", err)
		http.Error(w, "Internal Server Error", 500)
	}

	return err
}

// proxyGemini finds the Gemini content at u.
func proxyGemini(w http.ResponseWriter, r *http.Request, u *url.URL) (*url.URL, error) {
	var err error
	var rd *bufio.Reader

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
		return u, fmt.Errorf("proxyGemini: tls.Dial error to %s: %v", u.String(), err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, u.String()+"\r\n")

	rd = bufio.NewReader(conn)

	status, err := rd.ReadString("\n"[0])
	status = strings.Trim(status, "\r\n")
	if err != nil {
		return u, fmt.Errorf("proxyGemini: failed to read status line from buffer: %v", err)
	}
	if optVerbose || optDebug {
		log.Printf("proxyGemini: %s status: %s", u.String(), status)
	}
	if !reStatus.MatchString(status) {
		return u, fmt.Errorf("proxyGemini: invalid status line: %s", status)
	}

	switch status[0] {
	case "1"[0]: // Status: input
		var td templateData
		td.URL = u.String()
		td.Title = "Gneto " + td.URL
		td.Meta = status[3:]
		switch status[1] {
		case "1"[0]: // 11 == sensitive input/password
			err = tmpls.ExecuteTemplate(w, "password.html.tmpl", td)
			if err != nil {
				err = fmt.Errorf("proxyGemini: failed to execute password template: %v", err)
				break
			}
		default:
			err = tmpls.ExecuteTemplate(w, "input.html.tmpl", td)
			if err != nil {
				err = fmt.Errorf("proxyGemini: failed to execute input template: %v", err)
				break
			}
		}
	case "2"[0]: // Status: success
		if strings.Contains(status, " text/gemini") {
			if r.URL.Query().Get("source") != "" {
				err = textToHTML(w, u, rd)
			}
			err = geminiToHTML(w, u, rd)
			if err != nil {
				break
			}
		} else if strings.Contains(status, " text") {
			err = textToHTML(w, u, rd)
			if err != nil {
				break
			}
		} else {
			err = serveFile(w, r, u, rd)
			if err != nil {
				break
			}
		}
	case "3"[0]: // Status: redirect
		var ru *url.URL
		ru, err = url.Parse(strings.TrimSpace(strings.SplitAfterN(status, " ", 2)[1]))
		if err != nil {
			err = fmt.Errorf("proxyGemini: can't parse redirect URL %s: %v", strings.SplitAfterN(status, " ", 2)[1], err)
			break
		}
		if ru.Host == "" {
			ru.Host = u.Host
		}
		if ru.Scheme == "" {
			ru.Scheme = u.Scheme
		}
		u = ru
		errRedirect = errors.New(u.String())
		err = errRedirect
	default: // Statuses 40+ indicate various failures.
		err = fmt.Errorf("proxyGemini: status: %s", status)
	}

	return u, err
}

// serveFile saves a temporary file with the contents of rd, then serves it to w.
func serveFile(w http.ResponseWriter, r *http.Request, u *url.URL, rd *bufio.Reader) error {
	var err error
	fileName := u.String()[strings.LastIndex(u.String(), "/")+1:]

	f, err := ioutil.TempFile("", "gneto*-"+fileName)
	if err != nil {
		err = fmt.Errorf("serveFile: failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name()) // clean up

	if _, err := f.ReadFrom(rd); err != nil {
		err = fmt.Errorf("serveFile: failed to write to temp file: %v", err)
	}

	// Note: If we ever want to serve images inline, we'll have to revisit
	// this content disposition header value.
	w.Header().Set("Content-Disposition", "attachment; filename="+fileName)
	http.ServeContent(w, r, fileName, time.Time{}, f)

	if err := f.Close(); err != nil {
		err = fmt.Errorf("serveFile: failed to close temp file: %v", err)
	}

	return err
}

// textToHTML reads non-Gemini text from rd, and writes its HTML equivalent to w.
// The source URL is stored in u.
func textToHTML(w http.ResponseWriter, u *url.URL, rd *bufio.Reader) error {
	var err error

	var td templateData
	td.URL = u.String()
	td.Title = "Gneto " + td.URL
	err = tmpls.ExecuteTemplate(w, "header-only.html.tmpl", td)
	if err != nil {
		log.Println("textToHTML:", err)
		http.Error(w, "Internal Server Error", 500)
	}

	io.WriteString(w, `<pre id="non-gemini-text">`+"\n")
	var eof error
	var line string
	for eof == nil {
		line, eof = rd.ReadString("\n"[0])
		if optDebug {
			fmt.Println(line)
		}
		io.WriteString(w, line+"\n")
	}
	io.WriteString(w, "</pre>\n")

	err = tmpls.ExecuteTemplate(w, "footer-only.html.tmpl", td)
	if err != nil {
		log.Println("textToHTML:", err)
		http.Error(w, "Internal Server Error", 500)
	}

	return err
}
