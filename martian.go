package martian

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"

	"github.com/luraproject/lura/v2/config"
	"github.com/luraproject/lura/v2/logging"
	"github.com/luraproject/lura/v2/proxy"
	"github.com/luraproject/lura/v2/transport/http/client"

	// import the required martian packages so they can be used
	"github.com/google/martian"
	_ "github.com/google/martian/body"
	_ "github.com/google/martian/cookie"
	_ "github.com/google/martian/fifo"
	_ "github.com/google/martian/header"
	_ "github.com/google/martian/martianurl"
	"github.com/google/martian/parse"
	_ "github.com/google/martian/port"
	_ "github.com/google/martian/priority"
	_ "github.com/google/martian/stash"
	_ "github.com/google/martian/status"
)

var globalvariable string

// NewBackendFactory creates a proxy.BackendFactory with the martian request executor wrapping the injected one.
// If there is any problem parsing the extra config data, it just uses the injected request executor.
func NewBackendFactory(logger logging.Logger, re client.HTTPRequestExecutor) proxy.BackendFactory {
	return NewConfiguredBackendFactory(logger, func(_ *config.Backend) client.HTTPRequestExecutor { return re })
}

// NewConfiguredBackendFactory creates a proxy.BackendFactory with the martian request executor wrapping the injected one.
// If there is any problem parsing the extra config data, it just uses the injected request executor.
func NewConfiguredBackendFactory(logger logging.Logger, ref func(*config.Backend) client.HTTPRequestExecutor) proxy.BackendFactory {
	parse.Register("static.Modifier", staticModifierFromJSON)

	return func(remote *config.Backend) proxy.Proxy {
		re := ref(remote)
		result, ok := ConfigGetter(remote.ExtraConfig).(Result)
		if !ok {
			return proxy.NewHTTPProxyWithHTTPExecutor(remote, re, remote.Decoder)
		}
		switch result.Err {
		case nil:
			return proxy.NewHTTPProxyWithHTTPExecutor(remote, HTTPRequestExecutor(result.Result, re), remote.Decoder)
		case ErrEmptyValue:
			return proxy.NewHTTPProxyWithHTTPExecutor(remote, re, remote.Decoder)
		default:
			logger.Error(result, remote.ExtraConfig)
			return proxy.NewHTTPProxyWithHTTPExecutor(remote, re, remote.Decoder)
		}
	}
}

// HTTPRequestExecutor creates a wrapper over the received request executor, so the martian modifiers can be
// executed before and after the execution of the request
func HTTPRequestExecutor(result *parse.Result, re client.HTTPRequestExecutor) client.HTTPRequestExecutor {
	return func(ctx context.Context, req *http.Request) (resp *http.Response, err error) {
		fmt.Println("\n ----> INTO HTTPRequestExecutor: ", req, resp)
		// fmt.Println("\t\tglobalvariable:", globalvariable)

		if err = modifyRequest(result.RequestModifier(), req, "REQ"); err != nil {
			return
		}

		mctx, ok := req.Context().(*Context)
		if !ok || !mctx.SkippingRoundTrip() {
			resp, err = re(ctx, req)
			if err != nil {
				return
			}
			if resp == nil {
				err = ErrEmptyResponse
				return
			}
		} else if resp == nil {
			resp = &http.Response{
				Request:    req,
				Header:     http.Header{},
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(bytes.NewBufferString("")),
			}
		}

		// fmt.Println("\n ----> RESPONSE_)_): ", resp.Body)
		// err = modifyRequest(result.RequestModifier(), req, "RES")
		err = modifyResponse(result.ResponseModifier(), resp)
		return
	}
}

// func fetchRequest(ctx context.Context, req *http.Request) {
// 	mctx, ok := req.Context().(*Context)
// 	if !ok || !mctx.SkippingRoundTrip() {
// 		resp, err = re(ctx, req)
// 		if err != nil {
// 			return
// 		}
// 		if resp == nil {
// 			err = ErrEmptyResponse
// 			return
// 		}
// 	} else if resp == nil {
// 		resp = &http.Response{
// 			Request:    req,
// 			Header:     http.Header{},
// 			StatusCode: http.StatusOK,
// 			Body:       ioutil.NopCloser(bytes.NewBufferString("")),
// 		}
// 	}
// }

func toString(body io.Reader) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(body)
	return buf.String()
}

func modifyRequest(mod martian.RequestModifier, req *http.Request, reqType string) error {
	// b, err := httputil.DumpRequest(req, true)
	c, err := httputil.DumpRequest(req, true)
	if err != nil {
		fmt.Println("ERROR: ", err)
	}
	// fmt.Println("\n ----> REQUEST_0: ", reqType, string(b))
	fmt.Println("\n ----> REQUEST_0: ", reqType, string(c))

	fmt.Println("\n ----> REQUEST_globalvariable: ", globalvariable)
	// fmt.Println("\n ----> REQUEST_globalvariable: ", toString(req.Body))
	if globalvariable != "" {
		req.Body = ioutil.NopCloser(bytes.NewBufferString(globalvariable))
	}
	// fmt.Println(string(b))

	// if req.Body == nil {
	// 	req.Body = ioutil.NopCloser(bytes.NewBufferString(""))
	// }
	if req.Header == nil {
		req.Header = http.Header{}
	}

	// fmt.Println("91.1: REQUEST : BODY: ", toString(req.Body))

	if mod == nil {
		return nil
	}
	return mod.ModifyRequest(req)
}

func modifyResponse(mod martian.ResponseModifier, resp *http.Response) error {
	// b, err := httputil.DumpResponse(resp, true)
	c, err := httputil.DumpResponse(resp, true)
	if err != nil {
		fmt.Println("ERROR: ", err)
	}
	fmt.Println("\n ----> RESPONSE_globalvariable: ", globalvariable)
	// fmt.Println("\n ----> RESPONSE_0: ", string(b))
	fmt.Println("\n ----> RESPONSE_0: ", string(c))

	globalvariable = toString(resp.Body)
	// body, err := ioutil.ReadAll(resp.Body)
	// fmt.Println("\n ----> RESPONSE_0: ", string(body))

	resp.Body = ioutil.NopCloser(bytes.NewBufferString(globalvariable))
	// if resp.Body == nil {
	// }
	if resp.Header == nil {
		resp.Header = http.Header{}
	}
	if resp.StatusCode == 0 {
		resp.StatusCode = http.StatusOK
	}

	if mod == nil {
		return nil
	}
	return mod.ModifyResponse(resp)
}

// Namespace is the key to look for extra configuration details
const Namespace = "github.com/devopsfaith/krakend-martian"

// Result is a simple wrapper over the parse.FromJSON response tuple
type Result struct {
	Result *parse.Result
	Err    error
}

// ConfigGetter implements the config.ConfigGetter interface. It parses the extra config for the
// martian adapter and returns a Result wrapping the results.
func ConfigGetter(e config.ExtraConfig) interface{} {
	cfg, ok := e[Namespace]
	if !ok {
		return Result{nil, ErrEmptyValue}
	}

	data, ok := cfg.(map[string]interface{})
	if !ok {
		return Result{nil, ErrBadValue}
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return Result{nil, ErrMarshallingValue}
	}

	r, err := parse.FromJSON(raw)

	return Result{r, err}
}

var (
	// ErrEmptyValue is the error returned when there is no config under the namespace
	ErrEmptyValue = errors.New("getting the extra config for the martian module")
	// ErrBadValue is the error returned when the config is not a map
	ErrBadValue = errors.New("casting the extra config for the martian module")
	// ErrMarshallingValue is the error returned when the config map can not be marshalled again
	ErrMarshallingValue = errors.New("marshalling the extra config for the martian module")
	// ErrEmptyResponse is the error returned when the modifier receives a nil response
	ErrEmptyResponse = errors.New("getting the http response from the request executor")
)
