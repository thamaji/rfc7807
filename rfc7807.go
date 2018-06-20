package rfc7807

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"path"

	"github.com/pressly/chi"
	"github.com/russross/blackfriday"
)

func New(url string) *RFC7807 {
	return &RFC7807{
		URL:             url,
		mux:             chi.NewMux(),
		problemHandlers: map[string]problemHandlerFunc{},
	}
}

type RFC7807 struct {
	URL             string
	mux             *chi.Mux
	problemHandlers map[string]problemHandlerFunc
}

type problemHandlerFunc func(http.ResponseWriter, int, string, ...*Extension)

type Extension struct {
	Key   string
	Value interface{}
}

func Ext(key string, value interface{}) *Extension {
	return &Extension{Key: key, Value: value}
}

var DefaultTemplate = `<html>
  <head>
    <meta charset="utf-8">
    <title>Error {{.Title}}</title>
  </head>
  <body>
    <h1>{{.Title}}</h1>
    <pre>{{.Description}}</pre>
  </body>
</html>`

func (rfc7807 *RFC7807) Doc(title, description string) (problemHandlerFunc, error) {
	return rfc7807.TemplateDoc(title, description, DefaultTemplate)
}

func (rfc7807 *RFC7807) TemplateDoc(title string, description string, templateStr string) (problemHandlerFunc, error) {
	template, tError := template.New("default.tpl").Parse(templateStr)
	if tError != nil {
		return nil, tError
	}

	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	if err := template.Execute(buf, map[string]string{"Title": title, "Description": description}); err != nil {
		return nil, err
	}

	return rfc7807.HtmlDoc(title, buf.Bytes()), nil
}

func (rfc7807 *RFC7807) MarkdownDoc(title string, markdown []byte) problemHandlerFunc {
	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	buf.WriteString(`<html>\n<head>\n  <meta charset="utf-8">\n  <title>Error `)
	buf.WriteString(title)
	buf.WriteString(`</title>\n</head>\n<body>`)
	buf.Write(blackfriday.MarkdownCommon([]byte(markdown)))
	buf.WriteString(`</body>\n</html>`)

	return rfc7807.HtmlDoc(title, buf.Bytes())
}

func (rfc7807 *RFC7807) HtmlDoc(title string, html []byte) problemHandlerFunc {
	if rfc7807.mux == nil {
		rfc7807.mux = chi.NewMux()
	}

	docURL := ""
	if html != nil && len(html) > 0 {
		p := fmt.Sprintf("/%s.html", url.PathEscape(title))

		rfc7807.mux.Get(p, func(aWriter http.ResponseWriter, aRequest *http.Request) {
			aWriter.WriteHeader(http.StatusOK)
			aWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
			aWriter.Write(html)
		})

		url, _ := url.Parse(rfc7807.URL)
		url.Path = path.Join(url.Path, p)
		docURL = url.String()
	}

	if rfc7807.problemHandlers == nil {
		rfc7807.problemHandlers = map[string]problemHandlerFunc{}
	}

	rfc7807.problemHandlers[title] = func(w http.ResponseWriter, status int, detail string, extensions ...*Extension) {
		problem := map[string]interface{}{}

		for _, extension := range extensions {
			problem[extension.Key] = extension.Value
		}

		problem["type"] = docURL
		problem["title"] = title
		problem["status"] = status
		problem["detail"] = detail

		w.WriteHeader(status)
		w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		encoder.Encode(problem)
	}

	return rfc7807.problemHandlers[title]
}

func (rfc7807 *RFC7807) Error(w http.ResponseWriter, title string, status int, detail string, extensions ...*Extension) {
	if handler := rfc7807.problemHandlers[title]; handler != nil {
		handler(w, status, detail, extensions...)
		return
	}

	problem := map[string]interface{}{}

	for _, extension := range extensions {
		problem[extension.Key] = extension.Value
	}

	if title == "" {
		title = http.StatusText(status)
	}

	problem["title"] = title
	problem["status"] = status
	problem["detail"] = detail

	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.Encode(problem)
}

func (rfc7807 *RFC7807) ServeHTTP(aWriter http.ResponseWriter, aRequest *http.Request) {
	rfc7807.mux.ServeHTTP(aWriter, aRequest)
}
