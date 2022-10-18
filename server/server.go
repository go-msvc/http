package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-msvc/errors"
	"github.com/go-msvc/utils/ms"
	"github.com/stewelarend/logger"
)

var log = logger.New().WithLevel(logger.LevelDebug)

//implements github.com/go-msvc/utils/ms.Server using an HTTP REST interface

type Config struct {
	Addr string
	Port int
}

func (c Config) Validate() error {
	if c.Addr == "" {
		return errors.Errorf("missing addr")
	}
	if c.Port == 0 {
		return errors.Errorf("missing port")
	}
	if c.Port < 0 {
		return errors.Errorf("negative port:%d", c.Port)
	}
	return nil
}

func (c Config) Create(ms ms.MicroService) (ms.Server, error) {
	return server{
		ms:   ms,
		addr: fmt.Sprintf("%s:%d", c.Addr, c.Port),
	}, nil
}

type server struct {
	ms   ms.MicroService
	addr string
}

func (s server) Serve() error {
	log.Infof("HTTP REST server listen on %s", s.addr)
	return http.ListenAndServe(s.addr, s)
}

func (s server) ServeHTTP(httpRes http.ResponseWriter, httpReq *http.Request) {
	log.Infof("HTTP %s %s", httpReq.Method, httpReq.URL.Path)

	var err error
	defer func() {
		if err != nil {
			errCode := http.StatusInternalServerError
			if e, ok := err.(errors.IError); ok {
				if http.StatusText(e.Code()) != "" {
					errCode = e.Code()
				}
				log.Infof("code:%v->%v from err:%+v", errCode, http.StatusText(errCode), err)
			}
			if errCode >= 500 {
				log.Errorf("HTTP %s %s -> %d %s: %+v", httpReq.Method, httpReq.URL.Path, errCode, http.StatusText(errCode), err)
			}
			http.Error(httpRes, err.Error(), errCode)
			return
		}
		//success
	}()

	//get operation name from first part of URL path e.g. GET "/<oper>""
	var operName string
	{
		names := strings.SplitN(httpReq.URL.Path, "/", 2)
		if len(names) < 2 || len(names[0]) != 0 || len(names[1]) == 0 {
			err = errors.Errorc(http.StatusBadRequest, "URL does not start with /<operName>")
			return
		}
		operName = names[1]
	}
	oper, ok := s.ms.Oper(operName)
	if !ok {
		err = errors.Errorc(http.StatusNotFound, fmt.Sprintf("unknown operation %s != %s", operName, strings.Join(s.ms.OperNames(), "|")))
		return
	}

	var req interface{}
	if oper.ReqType() != nil {
		reqPtrValue := reflect.New(oper.ReqType())
		if err = json.NewDecoder(httpReq.Body).Decode(reqPtrValue.Interface()); err != nil && err != io.EOF {
			err = errors.Errorc(http.StatusBadRequest, fmt.Sprintf("failed to decode body into %v: %+v", oper.ReqType(), err))
			return
		}
		if validator, ok := reqPtrValue.Interface().(ms.Validator); ok {
			if err = validator.Validate(); err != nil {
				err = errors.Errorc(http.StatusBadRequest, fmt.Sprintf("invalid request: %+v", err))
				return
			}
		}
		req = reqPtrValue.Elem().Interface()
	}

	ctx := s.ms.NewContext()
	var res interface{}
	res, err = oper.Handle(ctx, req)
	if err != nil {
		err = errors.Wrapf(err, "%s handler failed", operName)
		return
	}

	if res != nil {
		var jsonRes []byte
		jsonRes, err = json.Marshal(res)
		httpRes.Header().Set("Content-Type", "application/json")
		httpRes.Write(jsonRes)
	}
	//http.Error(httpRes, "NYI", http.StatusNotFound)
}

func init() {
	ms.RegisteredServerImplementation("rest", Config{})
}
