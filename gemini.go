// Copyright 2020 Paul Gorman.

// Gneto makes Gemini pages available over HTTP.

package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// geminiToHTML parses a slice of Gemini lines, and returns HTML as one big string.
func geminiToHTML(baseURL string, gemini []string) string {
	html := make([]string, 0, 500)
	list := false
	pre := false

	for _, line := range gemini {
		if reGemBlank.MatchString(line) {
			if pre == true {
				html = append(html, line)
			} else {
				html = append(html, "<br>")
			}
			continue
		}

		if reGemPre.MatchString(line) {
			if list == true {
				list = false
				html = append(html, "</ul>")
			}
			if pre == true {
				pre = false
				html = append(html, "</pre>")
				continue
			}
			if pre == false {
				pre = true
				html = append(html, "<pre>")
				// TO DO: How do we provide alt text from reGemPre.FindStringSubmatch(line)[1]?
				continue
			}
		}

		if reGemH1.MatchString(line) {
			if list == true {
				list = false
				html = append(html, "</ul>")
			}
			if pre == true {
				html = append(html, line)
				continue
			}
			html = append(html, "<h1>"+reGemH1.FindStringSubmatch(line)[1]+"</h1>")
			continue
		}

		if reGemH2.MatchString(line) {
			if list == true {
				list = false
				html = append(html, "</ul>")
			}
			if pre == true {
				html = append(html, line)
				continue
			}
			html = append(html, "<h2>"+reGemH2.FindStringSubmatch(line)[1]+"</h2>")
			continue
		}

		if reGemH3.MatchString(line) {
			if list == true {
				list = false
				html = append(html, "</ul>")
			}
			if pre == true {
				html = append(html, line)
				continue
			}
			html = append(html, "<h3>"+reGemH3.FindStringSubmatch(line)[1]+"</h3>")
			continue
		}

		if reGemLink.MatchString(line) {
			var err error

			if list == true {
				list = false
				html = append(html, "</ul>")
			}
			if pre == true {
				html = append(html, line)
			}

			link := reGemLink.FindStringSubmatch(line)
			u, err := absoluteURL(baseURL, link[1])
			if err != nil {
				html = append(html, "<p>"+line+"</p>")
				continue
			}
			link[1] = u.String()

			if u.Scheme == "gemini" || u.Scheme == "gopher" {
				if link[2] != "" {
					html = append(html, `<p><a href="/?url=`+url.QueryEscape(link[1])+`">`+link[2]+
						`</a> <span class="scheme"><a href="`+link[1]+`">[`+u.Scheme+`]</a></span></p>`)
				} else {
					html = append(html, `<p><a href="/?url=`+url.QueryEscape(link[1])+`">`+link[1]+
						`</a> <span class="scheme"><a href="`+link[1]+`">[`+u.Scheme+`]</a></span></p>`)
				}
			} else {
				if link[2] != "" {
					html = append(html, `<p><a href="`+link[1]+`">`+link[2]+
						`</a> <span class="scheme"><a href="`+link[1]+`">[`+u.Scheme+`]</a></span></p>`)
				} else {
					html = append(html, `<p><a href="`+link[1]+`">`+link[1]+
						`</a> <span class="scheme"><a href="`+link[1]+`">[`+u.Scheme+`]</a></span></p>`)
				}
			}

			continue
		}

		if reGemList.MatchString(line) {
			if list == false {
				list = true
				html = append(html, "<ul>")
			}
			if pre == true {
				html = append(html, line)
			}
			html = append(html, "<li>"+reGemList.FindStringSubmatch(line)[1]+"</li>")
			continue
		}

		if reGemQuote.MatchString(line) {
			if list == true {
				list = false
				html = append(html, "</ul>")
			}
			if pre == true {
				html = append(html, line)
			}
			html = append(html, "<blockquote>"+reGemQuote.FindStringSubmatch(line)[1]+"</blockquote>")
			continue
		}

		if pre == true {
			html = append(html, line)
			continue
		}

		html = append(html, line+"<br>")
	}

	return strings.Join(html, "\n")
}

// getGemini fetches a Gemini file from URL g.
// getGemini expects a URL like gemini://tilde.team/.
func getGemini(g string) ([]string, error) {

	// TO DO: What if the target is an image or PDF instead of Gemini text?
	//gemini://idiomdrottning.org/fate-outcomes/

	gemini := make([]string, 0, 500)

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
		return gemini, fmt.Errorf("getGemini: tls.Dial error to %s: %v", g, err)
	}
	defer conn.Close()

	fmt.Fprintf(conn, u.String()+"\r\n")

	scanner := bufio.NewScanner(conn)
	l := 0
	for scanner.Scan() {
		s := scanner.Text()
		if optDebug {
			fmt.Println(s)
		}
		if l == 0 {
			if !reStatus.MatchString(s) {
				return gemini, fmt.Errorf("getGemini: invalid status line: %s", s)
			}
			l++
			if status {
				fmt.Println(s)
			}
			switch s[0] {
			case "2"[0]:
				// TO DO: Do something else if MIME type isn't "text/gemini".
				if strings.Contains(s, "text/gemini") {
					continue
				}
			case "3"[0]:
				ru, err := url.Parse(strings.SplitAfterN(s, " ", 2)[1])
				if err != nil {
					return gemini, fmt.Errorf("getGemini: can't parse redirect URL %s: %v", strings.SplitAfterN(s, " ", 2)[1], err)
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
				return gemini, fmt.Errorf("getGemini: status response: %s", s)
			}
		}
		gemini = append(gemini, s)
		l++
	}
	if err := scanner.Err(); err != nil {
		return gemini, fmt.Errorf("getGemini: scanning server response for %s: %v", g, err)
	}

	return gemini, nil
}
