package main

import (
	"errors"

	"github.com/hooky/hapi"
)

type IndexReq struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

type IndexResp struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

func main() {
	serv := hapi.Default()
	serv.GET("/", func(req *struct {
		Query IndexReq
	}, resp *struct {
		Data  *IndexResp
		Error error
	}) {
		resp.Data = (*IndexResp)(&req.Query)
	})
	serv.GET("/err", func(req *struct {
		Query IndexReq
	}, resp *struct {
		Data  *IndexResp
		Error error
	}) {
		resp.Error = errors.New("dasda")
	})
	serv.GET("/panic", func(req *struct {
		Query IndexReq
	}, resp *struct {
		Data  *IndexResp
		Error error
	}) {
		panic("panic")
	})
	if err := serv.Run(""); err != nil {
		panic(err)
	}
}
