package hapi

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"math"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

// abortIndex represents a typical value used in abort functions.
const abortIndex int8 = math.MaxInt8 >> 1

// ContextKey is the key that a Context returns itself for.
const ContextKey = "_hapi/contextkey"
const ReqBodyKey = "_hapi/requestBody"

type Context struct {
	writermem responseWriter
	engine    *Engine
	handlers  HandlersChain
	index     int8
	// This mutex protects Keys map.
	mu sync.RWMutex
	// Keys is a key/value pair exclusively for the context of each request.
	Keys map[string]any

	Writer   ResponseWriter
	Request  *http.Request
	fullPath string
	data     map[string]interface{}
	err      error
}

/************************************/
/********** CONTEXT CREATION ********/
/************************************/

func (c *Context) reset() {
	c.Writer = &c.writermem
	c.handlers = nil
	c.fullPath = ""
	c.index = -1
	c.data = nil
}

// Copy returns a copy of the current context that can be safely used outside the request's scope.
// This has to be used when the context has to be passed to a goroutine.
func (c *Context) Copy() *Context {
	cp := Context{
		writermem: c.writermem,
		Request:   c.Request,
		engine:    c.engine,
	}
	cp.writermem.ResponseWriter = nil
	cp.Writer = &cp.writermem
	cp.index = abortIndex
	cp.handlers = nil
	cp.Keys = map[string]any{}
	cp.fullPath = c.fullPath
	cp.data = nil
	for k, v := range c.Keys {
		cp.Keys[k] = v
	}
	return &cp
}

/************************************/
/*********** FLOW CONTROL ***********/
/************************************/

// Next should be used only inside middleware.
// It executes the pending handlers in the chain inside the calling handler.
// See example in GitHub.
func (c *Context) Next() {
	c.index++
	for c.index < int8(len(c.handlers)) {
		c.handlers[c.index](c)
		c.index++
	}
}

// IsAborted returns true if the current context was aborted.
func (c *Context) IsAborted() bool {
	return c.index >= abortIndex
}

// Abort prevents pending handlers from being called. Note that this will not stop the current handler.
// Let's say you have an authorization middleware that validates that the current request is authorized.
// If the authorization fails (ex: the password does not match), call Abort to ensure the remaining handlers
// for this request are not called.
func (c *Context) Abort() {
	c.index = abortIndex
}

func (c *Context) RequestBody() ([]byte, error) {
	if c.Request.Body == nil {
		return nil, nil
	}
	if data, ok := c.data[ReqBodyKey]; ok {
		if body, ok := data.([]byte); ok {
			return body, nil
		}
		return nil, nil
	}
	body, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		// c.SetError(err)
		return nil, err
	}
	if c.data == nil {
		c.data = make(map[string]interface{})
	}
	c.data[ReqBodyKey] = body
	c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(body))

	return body, nil
}

func (c *Context) ResponseBodySize() int64 {
	s := reflect.ValueOf(c.Writer).Elem().FieldByName(`written`)
	if s.IsValid() {
		return s.Int()
	} else {
		return 0
	}
}

func (c *Context) Data(data interface{}, err error) {
	statusCode := http.StatusOK
	body := struct {
		Code    uint        `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data,omitempty"`
	}{}
	if err == nil {
		body.Code = 0
		body.Message = `success`
	} else {
		if err2, ok := err.(interface {
			Code() uint
			Message() string
		}); ok && err2.Code() != 0 {
			body.Code, body.Message = err2.Code(), err2.Message()

			// if err3, ok := err.(interface {
			// 	GetError() error
			// }); ok && err3.GetError() != nil {
			// 	c.SetError(err3.GetError())
			// }
		} else {
			statusCode = http.StatusInternalServerError
			body.Code, body.Message = ServerErr, `Server Error.`
			// c.SetError(err)
		}
	}
	body.Data = getData(data, err, statusCode)

	c.StatusJson(statusCode, body)
}

func (c *Context) Ok(message string) {
	body := struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{
		Code:    "ok",
		Message: message,
	}
	c.StatusJson(http.StatusOK, body)
}

func (c *Context) Json(data interface{}) {
	c.StatusJson(http.StatusOK, data)
}
func (c *Context) StatusJson(status int, data interface{}) {
	// header should be set before WriteHeader or Write
	c.Writer.Header().Set(`Content-Type`, `application/json; charset=utf-8`)
	if v := reflect.ValueOf(c.Writer).Elem().FieldByName(`wroteHeader`); !v.IsValid() || !v.Bool() {
		c.Writer.WriteHeader(status)
	}

	encoder := json.NewEncoder(c.Writer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		// c.SetError(err)
		c.Writer.Write([]byte(`{"code":"json-marshal-error","message":"json marshal error"}`))
	}
}

func getData(data interface{}, err error, code int) interface{} {
	if err != nil && code != http.StatusInternalServerError {
		if err2, ok := err.(interface {
			Data() interface{}
		}); ok && err2.Data() != nil {
			return err2.Data()
		}
	}
	if data != nil && !isNilValue(data) { // 避免返回"data": null
		return data
	}
	return nil
}
func isNilValue(itfc interface{}) bool {
	v := reflect.ValueOf(itfc)
	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface:
		return v.IsNil()
	}
	return false
}

// ClientIP implements one best effort algorithm to return the real client IP.
// It called c.RemoteIP() under the hood, to check if the remote IP is a trusted proxy or not.
// If it is it will then try to parse the headers defined in Engine.RemoteIPHeaders (defaulting to [X-Forwarded-For, X-Real-Ip]).
// If the headers are not syntactically valid OR the remote IP does not correspond to a trusted proxy,
// the remote IP (coming from Request.RemoteAddr) is returned.
func (c *Context) ClientIP() string {
	// It also checks if the remoteIP is a trusted proxy or not.
	// In order to perform this validation, it will see if the IP is contained within at least one of the CIDR blocks
	// defined by Engine.SetTrustedProxies()
	remoteIP := net.ParseIP(c.RemoteIP())
	if remoteIP == nil {
		return ""
	}
	return remoteIP.String()
}

// RemoteIP parses the IP from Request.RemoteAddr, normalizes and returns the IP (without the port).
func (c *Context) RemoteIP() string {
	ip, _, err := net.SplitHostPort(strings.TrimSpace(c.Request.RemoteAddr))
	if err != nil {
		return ""
	}
	return ip
}

/************************************/
/***** GOLANG.ORG/X/NET/CONTEXT *****/
/************************************/

// Deadline returns that there is no deadline (ok==false) when c.Request has no Context.
func (c *Context) Deadline() (deadline time.Time, ok bool) {
	if c.Request == nil || c.Request.Context() == nil {
		return
	}
	return c.Request.Context().Deadline()
}

// Done returns nil (chan which will wait forever) when c.Request has no Context.
func (c *Context) Done() <-chan struct{} {
	if c.Request == nil || c.Request.Context() == nil {
		return nil
	}
	return c.Request.Context().Done()
}

// Err returns nil when c.Request has no Context.
func (c *Context) Err() error {
	if c.Request == nil || c.Request.Context() == nil {
		return nil
	}
	return c.Request.Context().Err()
}

// Value returns the value associated with this context for key, or nil
// if no value is associated with key. Successive calls to Value with
// the same key returns the same result.
func (c *Context) Value(key any) any {
	if key == 0 {
		return c.Request
	}
	if key == ContextKey {
		return c
	}
	if keyAsString, ok := key.(string); ok {
		if val, exists := c.Get(keyAsString); exists {
			return val
		}
	}
	if c.Request == nil || c.Request.Context() == nil {
		return nil
	}
	return c.Request.Context().Value(key)
}

/************************************/
/******** METADATA MANAGEMENT********/
/************************************/

// Set is used to store a new key/value pair exclusively for this context.
// It also lazy initializes  c.Keys if it was not used previously.
func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	if c.Keys == nil {
		c.Keys = make(map[string]any)
	}

	c.Keys[key] = value
	c.mu.Unlock()
}

// Get returns the value for the given key, ie: (value, true).
// If the value does not exist it returns (nil, false)
func (c *Context) Get(key string) (value any, exists bool) {
	c.mu.RLock()
	value, exists = c.Keys[key]
	c.mu.RUnlock()
	return
}

// MustGet returns the value for the given key if it exists, otherwise it panics.
func (c *Context) MustGet(key string) any {
	if value, exists := c.Get(key); exists {
		return value
	}
	panic("Key \"" + key + "\" does not exist")
}

// GetString returns the value associated with the key as a string.
func (c *Context) GetString(key string) (s string) {
	if val, ok := c.Get(key); ok && val != nil {
		s, _ = val.(string)
	}
	return
}

// GetBool returns the value associated with the key as a boolean.
func (c *Context) GetBool(key string) (b bool) {
	if val, ok := c.Get(key); ok && val != nil {
		b, _ = val.(bool)
	}
	return
}

// GetInt returns the value associated with the key as an integer.
func (c *Context) GetInt(key string) (i int) {
	if val, ok := c.Get(key); ok && val != nil {
		i, _ = val.(int)
	}
	return
}

// GetInt64 returns the value associated with the key as an integer.
func (c *Context) GetInt64(key string) (i64 int64) {
	if val, ok := c.Get(key); ok && val != nil {
		i64, _ = val.(int64)
	}
	return
}

// GetUint returns the value associated with the key as an unsigned integer.
func (c *Context) GetUint(key string) (ui uint) {
	if val, ok := c.Get(key); ok && val != nil {
		ui, _ = val.(uint)
	}
	return
}

// GetUint64 returns the value associated with the key as an unsigned integer.
func (c *Context) GetUint64(key string) (ui64 uint64) {
	if val, ok := c.Get(key); ok && val != nil {
		ui64, _ = val.(uint64)
	}
	return
}

// GetFloat64 returns the value associated with the key as a float64.
func (c *Context) GetFloat64(key string) (f64 float64) {
	if val, ok := c.Get(key); ok && val != nil {
		f64, _ = val.(float64)
	}
	return
}

// GetTime returns the value associated with the key as time.
func (c *Context) GetTime(key string) (t time.Time) {
	if val, ok := c.Get(key); ok && val != nil {
		t, _ = val.(time.Time)
	}
	return
}

// GetDuration returns the value associated with the key as a duration.
func (c *Context) GetDuration(key string) (d time.Duration) {
	if val, ok := c.Get(key); ok && val != nil {
		d, _ = val.(time.Duration)
	}
	return
}

// GetStringSlice returns the value associated with the key as a slice of strings.
func (c *Context) GetStringSlice(key string) (ss []string) {
	if val, ok := c.Get(key); ok && val != nil {
		ss, _ = val.([]string)
	}
	return
}

// GetStringMap returns the value associated with the key as a map of interfaces.
func (c *Context) GetStringMap(key string) (sm map[string]any) {
	if val, ok := c.Get(key); ok && val != nil {
		sm, _ = val.(map[string]any)
	}
	return
}

// GetStringMapString returns the value associated with the key as a map of strings.
func (c *Context) GetStringMapString(key string) (sms map[string]string) {
	if val, ok := c.Get(key); ok && val != nil {
		sms, _ = val.(map[string]string)
	}
	return
}

// GetStringMapStringSlice returns the value associated with the key as a map to a slice of strings.
func (c *Context) GetStringMapStringSlice(key string) (smss map[string][]string) {
	if val, ok := c.Get(key); ok && val != nil {
		smss, _ = val.(map[string][]string)
	}
	return
}
