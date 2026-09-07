package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
)

type Output[T any] interface {
	Emit(T)
	EmitAll([]T)
	ErrorResponse(*resty.Response) error
	ShouldStop() bool
	Done()
}

type output[T any] struct {
	isSingle    bool
	hasPrevious bool
	writer      io.Writer
}

func (o *output[T]) EmitAll(vs []T) {
	for _, v := range vs {
		o.Emit(v)
	}
}

func (o *output[T]) Emit(v T) {
	indent := ""
	if !o.isSingle {
		indent = "  "
		if !o.hasPrevious {
			fmt.Fprint(o.writer, "[\n")
		} else {
			fmt.Fprint(o.writer, ",\n")
		}
	}

	formatted, _ := json.MarshalIndent(v, indent, "  ")
	fmt.Fprint(o.writer, indent+string(formatted))
	o.hasPrevious = true
}

func (o *output[T]) ErrorResponse(resp *resty.Response) error {
	restErr := resp.Error()
	if restErr == nil {
		restErr = resp.Body()
	}

	errorJson := struct {
		Status   int         `json:"status"`
		Response interface{} `json:"response"`
	}{
		Status:   resp.StatusCode(),
		Response: restErr,
	}

	prettyJSON, _ := json.MarshalIndent(errorJson, "", "  ")
	fmt.Fprintln(o.writer, string(prettyJSON))

	return errors.New("error from API")
}

func (o *output[T]) ShouldStop() bool {
	return false
}

func (o *output[T]) Done() {
	if !o.isSingle {
		if o.hasPrevious {
			fmt.Fprint(o.writer, "\n]")
		} else {
			fmt.Fprintln(o.writer, "[]")
		}
	}
}

func OutputSingle[T any](cmd *cobra.Command) Output[T] {
	return &output[T]{
		isSingle: true,
		writer:   cmd.OutOrStdout(),
	}
}

func OutputMultiple[T any](cmd *cobra.Command) Output[T] {
	return &output[T]{
		isSingle: false,
		writer:   cmd.OutOrStdout(),
	}
}
