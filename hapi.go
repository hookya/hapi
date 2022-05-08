package hapi

import (
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// HandlerFunc defines the handler used by gin middleware as return value.
type HandlerFunc func(*Context)

// HandlersChain defines a HandlerFunc slice.
type HandlersChain []HandlerFunc

// Last returns the last handler in the chain. i.e. the last handler is the main one.
func (c HandlersChain) Last() HandlerFunc {
	if length := len(c); length > 0 {
		return c[length-1]
	}
	return nil
}

type Engine struct {
	RouterGroup
	pool   sync.Pool
	trees  methodTrees
	UseH2C bool
}

var _ Group = &Engine{}

var (
	default404Body = []byte("404 page not found")
	default405Body = []byte("405 method not allowed")
	default500Body = []byte("500 server err")
)

func New() *Engine {
	engine := &Engine{
		RouterGroup: RouterGroup{
			Handlers: nil,
			basePath: "/",
			root:     false,
		},
		trees:  make(methodTrees, 0, 7),
		UseH2C: true,
	}
	engine.RouterGroup.engine = engine
	engine.pool.New = func() any {
		return engine.allocateContext()
	}
	return engine
}

func Default() *Engine {
	engine := New()
	// engine.Use(Logger(), Recovery())
	return engine
}

func (engine *Engine) allocateContext() *Context {
	return &Context{engine: engine}
}

// Use attaches a global middleware to the router. i.e. the middleware attached through Use() will be
// included in the handlers chain for every single request. Even 404, 405, static files...
// For example, this is the right place for a logger or error management middleware.
func (engine *Engine) Use(middleware ...HandlerFunc) Group {
	engine.RouterGroup.Use(middleware...)
	// engine.rebuild404Handlers()
	// engine.rebuild405Handlers()
	return engine
}

func (engine *Engine) addRoute(method, path string, handlers HandlersChain) {
	assert1(path[0] == '/', "path must begin with '/'")
	assert1(method != "", "HTTP method can not be empty")
	assert1(len(handlers) > 0, "there must be at least one handler")

	debugPrintRoute(method, path, handlers)

	root := engine.trees.get(method)
	if root == nil {
		root = new(node)
		root.fullPath = "/"
		engine.trees = append(engine.trees, methodTree{method: method, root: root})
	}
	root.addRoute(path, handlers)
}

// Run attaches the router to a http.Server and starts listening and serving HTTP requests.
// It is a shortcut for http.ListenAndServe(addr, router)
// Note: this method will block the calling goroutine indefinitely unless an error happens.
func (engine *Engine) Run(addr string) (err error) {

	address := resolveAddress(addr)
	debugPrint("Listening and serving HTTP on %s\n", address)
	err = http.ListenAndServe(address, engine.Handler())
	return
}

// ServeHTTP conforms to the http.Handler interface.
func (engine *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	c := engine.pool.Get().(*Context)
	c.writermem.reset(w)
	c.Request = req
	c.reset()
	defer func() {
		err := recover()
		if err != nil {
			fmt.Println(err)
		}
		serveError(c, http.StatusInternalServerError, default500Body)
	}()
	engine.handleHTTPRequest(c)
	engine.pool.Put(c)
}

func (engine *Engine) Handler() http.Handler {
	if !engine.UseH2C {
		return engine
	}

	h2s := &http2.Server{}
	return h2c.NewHandler(engine, h2s)
}

func (engine *Engine) handleHTTPRequest(c *Context) {
	httpMethod := c.Request.Method
	rPath := c.Request.URL.Path

	// Find root of the tree for the given HTTP method
	t := engine.trees
	for i, tl := 0, len(t); i < tl; i++ {
		if t[i].method != httpMethod {
			continue
		}
		root := t[i].root
		// Find route in tree
		handlers := root.getValue(rPath)
		if handlers != nil {
			c.handlers = handlers
			// c.fullPath = fullPath
			c.Next()
			c.writermem.WriteHeaderNow()
			return
		}
		break
	}

	serveError(c, http.StatusNotFound, default404Body)
}

func serveError(c *Context, code int, defaultMessage []byte) {
	c.writermem.status = code
	// c.Next()
	if c.writermem.Written() {
		return
	}
	if c.writermem.Status() == code {
		c.writermem.Header()["Content-Type"] = []string{"application/json"}
		_, err := c.Writer.Write(defaultMessage)
		if err != nil {
			debugPrint("cannot write message to writer during serve error: %v", err)
		}
		return
	}
	c.writermem.WriteHeaderNow()
}
