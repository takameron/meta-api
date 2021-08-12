package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/mattn/go-encoding"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

type Response struct {
	Host       string            `json:"host"`
	Metas      map[string]string `json:"metas"`
	Proto      string            `json:"protocol"`
	StatusCode int               `json:"status_code"`
	StatusText string            `json:"status_text"`
	Success    bool              `json:"success"`
	Title      string            `json:"title"`
	URL        string            `json:"url"`
}

type ResponseError struct {
	Msg     string `json:"msg"`
	Success bool   `json:"success"`
}

func convertUTF8(reader io.Reader, contentType string) io.Reader {
	br := bufio.NewReader(reader)
	var r io.Reader = br
	if data, err := br.Peek(4096); err == nil {
		enc, name, _ := charset.DetermineEncoding(data, contentType)
		if enc != nil {
			r = enc.NewDecoder().Reader(br)
		} else if name != "" {
			if enc := encoding.GetEncoding(name); enc != nil {
				r = enc.NewDecoder().Reader(br)
			}
		}
	}
	return r
}

func getTags(reader io.Reader) (string, map[string]string, error) {
	title := ""
	metas := map[string]string{}
	tokenizer := html.NewTokenizer(reader)

	for {
		tokenType := tokenizer.Next()

		if tokenType == html.ErrorToken {
			err := tokenizer.Err()
			if err == io.EOF {
				break
			}

			log.Fatalf("error tokenizing HTML: %v", tokenizer.Err())
			return title, nil, tokenizer.Err()
		}

		t := tokenizer.Token()
		name := t.Data
		attrs := t.Attr

		// </head>
		if tokenType == html.EndTagToken && t.DataAtom.String() == "head" {
			break
		}

		// <title></title>
		if tokenType == html.StartTagToken && t.DataAtom.String() == "title" {
			tokenType = tokenizer.Next()
			t = tokenizer.Token()
			title = t.Data
		}

		if name == "meta" {
			key := ""
			val := ""
			for _, v := range attrs {
				if v.Key == "property" || v.Key == "name" || v.Key == "itemprop" {
					// key is unified to lowercase and replace ':' to '_'
					key = strings.Replace(v.Val, ":", "_", -1)
				}
				if v.Key == "content" {
					val = v.Val
				}
				if v.Key == "charset" {
					key = v.Key
					val = v.Val
				}
			}
			metas[key] = val
		}
	}
	return title, metas, nil
}

func Handler(w http.ResponseWriter, r *http.Request) {
	queries := r.URL.Query()

	// no query
	if queries == nil {
		w.WriteHeader(400)
		w.Write([]byte("400 Bad Request\n"))
		data := ResponseError{Success: false, Msg: "no query"}
		msg, _ := json.Marshal(data)
		fmt.Fprintf(w, string(msg))
		return
	}

	// no query of url
	if _, ok := queries["url"]; !ok {
		w.WriteHeader(400)
		w.Write([]byte("400 Bad Request\n"))
		data := ResponseError{Success: false, Msg: "need url query"}
		msg, _ := json.Marshal(data)
		fmt.Fprintf(w, string(msg))
		return
	}

	// request
	url := queries.Get("url")
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatalf("error request HTML: %v", err)
		w.WriteHeader(500)
		w.Write([]byte("500 Internal Server Error\n"))
		data := ResponseError{Success: false, Msg: err.Error()}
		msg, _ := json.Marshal(data)
		fmt.Fprintf(w, string(msg))
		return
	}

	// get
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("error get HTML: %v", err)
		w.WriteHeader(500)
		w.Write([]byte("500 Internal Server Error\n"))
		data := ResponseError{Success: false, Msg: err.Error()}
		msg, _ := json.Marshal(data)
		fmt.Fprintf(w, string(msg))
		return
	}
	defer res.Body.Close()

	data := Response{Success: false}
	title, metas, err := getTags(convertUTF8(res.Body, res.Header.Get("Content-Type")))
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("500 Internal Server Error\n"))
		data := ResponseError{Success: false, Msg: err.Error()}
		msg, _ := json.Marshal(data)
		fmt.Fprintf(w, string(msg))
		return
	}

	data.Host = req.Host
	data.Metas = metas
	data.Proto = res.Proto
	data.StatusCode = res.StatusCode
	data.StatusText = res.Status
	data.Success = true
	data.Title = title
	data.URL = req.URL.String()

	msg, _ := json.Marshal(data)
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Add("Access-Control-Allow-Methods", "GET,OPTIONS")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Max-Age", "86400")
	w.Header().Add("Content-Type", "application/json;charset=UTF-8")
	fmt.Fprintf(w, string(msg))
}
