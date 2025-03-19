package test_utils

import (
	"fmt"
	mock "gopkg.in/h2non/gentleman-mock.v2"
	"gopkg.in/h2non/gentleman.v2"
	"gopkg.in/h2non/gock.v1"
	"net/http"
)

func MockGentlemenGetResponse(domain string, path string, setup func(*gock.Request)) *gentleman.Response {
	return MockGentlemenResponse(http.MethodGet, domain, path, setup)
}

func MockGentlemenPostResponse(domain string, path string, setup func(*gock.Request)) *gentleman.Response {
	return MockGentlemenResponse(http.MethodPost, domain, path, setup)
}

func MockGentlemenResponse(method string, domain string, path string, setup func(*gock.Request)) *gentleman.Response {
	defer gock.Off()

	mockReq := mock.New(domain).
		Get(path)

	switch method {
	case http.MethodGet:
		mockReq.Get(path)
		break
	case http.MethodPost:
		mockReq.Post(path)
	case http.MethodPut:
		mockReq.Put(path)
	case http.MethodDelete:
		mockReq.Delete(path)
	case http.MethodPatch:
		mockReq.Patch(path)
	case http.MethodHead:
		mockReq.Head(path)
	default:
		panic(fmt.Sprintf("Unsupported HTTP method: %s", method))
	}

	setup(mockReq)

	client := gentleman.New()
	client.Use(mock.Plugin)

	req := client.Request()
	req.URL(fmt.Sprintf("%s/%s", domain, path)).Method(method)
	resp, _ := req.Send()
	return resp
}
