package hapi

import (
	"testing"
)

// Used as a workaround since we can't compare functions or their addresses
var fakeHandlerValue string

type testRequests []struct {
	path       string
	nilHandler bool
	route      string
}

func fakeHandler(val string) HandlersChain {
	return HandlersChain{func(c *Context) {
		fakeHandlerValue = val
	}}
}

func checkRequests(t *testing.T, tree *node, requests testRequests) {

	for _, request := range requests {
		handlers := tree.getValue(request.path)

		if handlers == nil {
			if !request.nilHandler {
				t.Errorf("handle mismatch for route '%s': Expected non-nil handle", request.path)
			}
		} else if request.nilHandler {
			t.Errorf("handle mismatch for route '%s': Expected nil handle", request.path)
		} else {
			handlers[0](nil)
			if fakeHandlerValue != request.route {
				t.Errorf("handle mismatch for route '%s': Wrong handle (%s != %s)", request.path, fakeHandlerValue, request.route)
			}
		}
	}
}

func Test_node_addRoute(t *testing.T) {
	tree := &node{}

	routes := [...]string{
		"/hi",
		"/contact",
		"/co",
		"/c",
		"/a",
		"/ab",
		"/doc/",
		"/doc/go_faq.html",
		"/doc/go1.html",
		"/α",
		"/β",
	}
	for _, route := range routes {
		tree.addRoute(route, fakeHandler(route))
	}

	checkRequests(t, tree, testRequests{
		{"/a", false, "/a"},
		{"/", true, ""},
		{"/hi", false, "/hi"},
		{"/contact", false, "/contact"},
		{"/co", false, "/co"},
		{"/con", true, ""},  // key mismatch
		{"/cona", true, ""}, // key mismatch
		{"/no", true, ""},   // no matching child
		{"/ab", false, "/ab"},
		{"/α", false, "/α"},
		{"/β", false, "/β"},
	})
}
