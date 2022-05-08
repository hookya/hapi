package hapi

import (
	"net/http"
)

type Group interface {
	Group(string, ...HandlerFunc) *RouterGroup

	GET(string, interface{}) Group
	POST(string, interface{}) Group
	DELETE(string, interface{}) Group
	PATCH(string, interface{}) Group
	PUT(string, interface{}) Group
	OPTIONS(string, interface{}) Group
	HEAD(string, interface{}) Group
}

// RouterGroup is used internally to configure router, a RouterGroup is associated with
// a prefix and an array of handlers (middleware).
type RouterGroup struct {
	Handlers []HandlerFunc
	basePath string
	engine   *Engine
	root     bool
}

// var _ Group = &RouterGroup{}

// For example, if v := router.Group("/rest/n/v1/api"), v.BasePath() is "/rest/n/v1/api".
func (group *RouterGroup) BasePath() string {
	return group.basePath
}

func (group *RouterGroup) handle(httpMethod, relativePath string, handler interface{}) Group {
	absolutePath := group.calculateAbsolutePath(relativePath)
	handlers := group.combineHandlers(convertHandler(handler, relativePath))
	group.engine.addRoute(httpMethod, absolutePath, handlers)
	return group.returnObj()
}

// func (group *RouterGroup) makeHandlers(
// 	method, fullPath string, args []interface{}, routeHandler ...interface{}) []func(*Context) {
// 	handlers := make([]func(*Context), 0, len(group.Handlers)+1)
// 	for _, v := range group.Handlers {
// 		switch h := v.(type) {
// 		case func(*Context):
// 			handlers = append(handlers, h)
// 		// case func(method, fullPath string, args []interface{}) func(*Context):
// 		// 	if handler := h(method, fullPath, args); handler != nil {
// 		// 		handlers = append(handlers, handler)
// 		// 	}
// 		default:
// 			log.Panicf("Unknown handler: %v\n", h)
// 		}
// 	}
// 	return append(handlers, convertHandler(routeHandler, fullPath))
// }

// Use adds middleware to the group, see example code in GitHub.
func (group *RouterGroup) Use(middleware ...HandlerFunc) Group {
	for _, handler := range middleware {
		group.Handlers = append(group.Handlers, handler)
	}
	return group.returnObj()
}

// POST is a shortcut for router.Handle("POST", path, handle).
func (group *RouterGroup) POST(relativePath string, handler interface{}) Group {
	return group.handle(http.MethodPost, relativePath, handler)
}

// GET is a shortcut for router.Handle("GET", path, handle).
func (group *RouterGroup) GET(relativePath string, handler interface{}) Group {
	return group.handle(http.MethodGet, relativePath, handler)
}

// DELETE is a shortcut for router.Handle("DELETE", path, handle).
func (group *RouterGroup) DELETE(relativePath string, handler interface{}) Group {
	return group.handle(http.MethodDelete, relativePath, handler)
}

// PATCH is a shortcut for router.Handle("PATCH", path, handle).
func (group *RouterGroup) PATCH(relativePath string, handler interface{}) Group {
	return group.handle(http.MethodPatch, relativePath, handler)
}

// PUT is a shortcut for router.Handle("PUT", path, handle).
func (group *RouterGroup) PUT(relativePath string, handler interface{}) Group {
	return group.handle(http.MethodPut, relativePath, handler)
}

// OPTIONS is a shortcut for router.Handle("OPTIONS", path, handle).
func (group *RouterGroup) OPTIONS(relativePath string, handler interface{}) Group {
	return group.handle(http.MethodOptions, relativePath, handler)
}

// HEAD is a shortcut for router.Handle("HEAD", path, handle).
func (group *RouterGroup) HEAD(relativePath string, handler interface{}) Group {
	return group.handle(http.MethodHead, relativePath, handler)
}

// Group creates a new router group. You should add all the routes that have common middlewares or the same path prefix.
// For example, all the routes that use a common middleware for authorization could be grouped.
func (group *RouterGroup) Group(relativePath string, handlers ...HandlerFunc) *RouterGroup {
	return &RouterGroup{
		Handlers: group.combineHandlers(handlers...),
		basePath: group.calculateAbsolutePath(relativePath),
		engine:   group.engine,
	}
}

func (group *RouterGroup) combineHandlers(handlers ...HandlerFunc) HandlersChain {
	finalSize := len(group.Handlers) + len(handlers)
	mergedHandlers := make(HandlersChain, finalSize)
	copy(mergedHandlers, group.Handlers)
	copy(mergedHandlers[len(group.Handlers):], handlers)
	return mergedHandlers
}

func (group *RouterGroup) calculateAbsolutePath(relativePath string) string {
	return joinPaths(group.basePath, relativePath)
}

func (group *RouterGroup) returnObj() Group {
	if group.root {
		return group.engine
	}
	return group
}
