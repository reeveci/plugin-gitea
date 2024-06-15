package main

import (
	"bytes"
	"io"
	"text/template"
)

func ParseTemplate(name string, templ string, data any) (io.Reader, error) {
	t, err := template.New(name).Parse(templ)
	if err != nil {
		return nil, err
	}

	res := new(bytes.Buffer)
	err = t.Execute(res, data)
	if err != nil {
		return nil, err
	}

	return res, nil
}
