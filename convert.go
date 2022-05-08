package hapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/lovego/struct_tag"
)

type todoReqFields struct {
	Param  bool
	Query  bool
	Header bool
	Body   bool
	Ctx    bool
}

func convertHandler(h interface{}, path string) HandlerFunc {
	if handler, ok := h.(func(*Context)); ok {
		return handler
	}

	val := reflect.ValueOf(h)

	typ := val.Type()
	if typ.Kind() != reflect.Func {
		panic("handler must be a func.")
	}
	if typ.NumIn() != 2 {
		panic("handler func must have exactly two parameters.")
	}
	if typ.NumOut() != 0 {
		panic("handler func must have no return values.")
	}

	reqConvertFunc, hasCtx := newReqConvertFunc(typ.In(0), path)
	respTyp, respWriteFunc := newRespWriteFunc(typ.In(1), hasCtx)

	return func(ctx *Context) {
		req, err := reqConvertFunc(ctx)
		if err != nil {
			// ctx.Data(nil, errs.New("args-err", err.Error()))
			// TODO return args err
			panic("args-err")
			return
		}
		resp := reflect.New(respTyp)
		val.Call([]reflect.Value{req, resp})
		if respWriteFunc != nil {
			respWriteFunc(ctx, resp.Elem())
		}
	}
}

func newReqConvertFunc(typ reflect.Type, path string) (
	func(*Context) (reflect.Value, error), bool,
) {
	isPtr := false
	if typ.Kind() == reflect.Ptr {
		isPtr = true
		typ = typ.Elem()
	}
	todo := validateReqFields(typ, path)

	return func(ctx *Context) (reflect.Value, error) {
		ptr := reflect.New(typ)
		req := ptr.Elem()

		var err error
		Traverse(req, func(value reflect.Value, f reflect.StructField) bool {
			switch f.Name {
			case "Query":
				if todo.Query {
					convertNilPtr(value)
					err = Query(value, ctx.Request.URL.Query())
				}
			case "Header":
				if todo.Header {
					convertNilPtr(value)
					err = Header(value, ctx.Request.Header)
				}
			case "Body":
				if todo.Body {
					err = convertReqBody(value, ctx)
				}
			case "Ctx":
				if todo.Ctx {
					value.Set(reflect.ValueOf(ctx))
				}
			}
			return err == nil
		})
		if err != nil {
			return reflect.Value{}, err
		}

		if isPtr {
			return ptr, nil
		} else {
			return req, nil
		}
	}, todo.Ctx
}

var typeContextPtr = reflect.TypeOf((*Context)(nil))

func validateReqFields(typ reflect.Type, path string) (todo todoReqFields) {
	if typ.Kind() != reflect.Struct {
		panic("req parameter of handler func must be a struct or struct pointer.")
	}

	TraverseType(typ, func(f reflect.StructField) {
		switch f.Name {
		case "Query":
			if !isEmptyStruct(f.Type) {
				ValidateQuery(f.Type)
				todo.Query = true
			}
		case "Header":
			if !isEmptyStruct(f.Type) {
				ValidateHeader(f.Type)
				todo.Header = true
			}
		case "Ctx":
			if f.Type != typeContextPtr {
				panic("Ctx field of req parameter must be of type '*goa.Context'.")
			}
			todo.Ctx = true
		case "Body":
			if !isEmptyStruct(f.Type) {
				todo.Body = true
			}
		default:
			panic("Unknown field: req." + f.Name)
		}
	})
	return
}

func convertReqBody(value reflect.Value, ctx *Context) error {
	body, err := ctx.RequestBody()
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, value.Addr().Interface()); err != nil {
		return fmt.Errorf("req.Body: %s", err.Error())
	}
	return nil
}

func isEmptyStruct(typ reflect.Type) bool {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.Kind() == reflect.Struct && typ.NumField() == 0
}

func ValidateHeader(typ reflect.Type) {
	if !isStructOrStructPtr(typ) {
		panic("req.Header must be struct or pointer to struct.")
	}
}

func ValidateQuery(typ reflect.Type) {
	if !isStructOrStructPtr(typ) {
		panic("req.Query must be struct or pointer to struct.")
	}
}

func isStructOrStructPtr(typ reflect.Type) bool {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.Kind() == reflect.Struct
}
func convertNilPtr(v reflect.Value) {
	if v.Kind() == reflect.Ptr && v.IsNil() && v.CanSet() {
		v.Set(reflect.New(v.Type().Elem()))
	}
}

func Query(value reflect.Value, map2strs map[string][]string) (err error) {
	if len(map2strs) == 0 {
		return nil
	}
	Traverse(value, func(v reflect.Value, f reflect.StructField) bool {
		paramName, arrayParamName := queryParamName(f)
		if paramName == "" {
			return true
		}
		// value is always empty, so Set only when len(values) > 0
		if values := queryParamValues(map2strs, paramName, arrayParamName); len(values) > 0 {
			if err = SetArray(v, values); err != nil {
				err = fmt.Errorf("req.Query.%s: %s", f.Name, err.Error())
			}
			return err == nil // if err == nil, go on Traverse
		}
		return true // go on Traverse
	})
	return
}

func queryParamName(field reflect.StructField) (string, string) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", ""
	}
	name := field.Name
	if tag != "" {
		if idx := strings.Index(tag, ","); idx > 0 {
			name = tag[:idx]
		} else if idx < 0 {
			name = tag
		}
	}
	if kind := field.Type.Kind(); kind == reflect.Slice || kind == reflect.Array {
		return name, name + "[]"
	}
	return name, ""
}

func queryParamValues(map2strs map[string][]string, paramName, arrayParamName string) []string {
	if values, ok := map2strs[paramName]; ok {
		return values
	}
	if arrayParamName != "" {
		if values, ok := map2strs[arrayParamName]; ok {
			return values
		}
	}

	paramName, arrayParamName = strings.ToLower(paramName), strings.ToLower(arrayParamName)
	for key, values := range map2strs {
		key = strings.ToLower(key)
		if key == paramName || arrayParamName != "" && key == arrayParamName {
			return values
		}
	}

	return nil
}

func Header(value reflect.Value, map2strs map[string][]string) (err error) {
	Traverse(value, func(v reflect.Value, f reflect.StructField) bool {
		key, _ := struct_tag.Lookup(string(f.Tag), "header")
		if key == "" {
			key = f.Name
		}
		values := map2strs[key]
		if len(values) > 0 && values[0] != "" {
			if err = Set(v, values[0]); err != nil {
				err = fmt.Errorf("req.Header.%s: %s", f.Name, err.Error())
			}
		}
		return err == nil // if err == nil, go on Traverse
	})
	return
}

func newRespWriteFunc(typ reflect.Type, hasCtx bool) (reflect.Type, func(*Context, reflect.Value)) {
	if typ.Kind() != reflect.Ptr {
		panic("resp parameter of handler func must be a struct pointer.")
	}
	typ = typ.Elem()
	if typ.Kind() != reflect.Struct {
		panic("resp parameter of handler func must be a struct pointer.")
	}
	if validateRespFields(typ) {
		return typ, nil
	}
	return typ, func(ctx *Context, resp reflect.Value) {
		if hasCtx && ctx.ResponseBodySize() > 0 {
			return
		}

		var data interface{}
		var err error

		Traverse(resp, func(v reflect.Value, f reflect.StructField) bool {
			switch f.Name {
			case "Error":
				if e := v.Interface(); e != nil {
					err = e.(error)
				}
			case "Data":
				data = v.Interface()
			case "Header":
				WriteRespHeader(v, ctx.Writer.Header())
			}
			return true
		})
		ctx.Data(data, err)
	}
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

func validateRespFields(typ reflect.Type) bool {
	empty := true
	TraverseType(typ, func(f reflect.StructField) {
		switch f.Name {
		case "Data":
			// data can be of any type
		case "Error":
			if !f.Type.Implements(errorType) {
				panic(`resp.Error must be of "error" type.`)
			}
		case "Header":
			ValidateRespHeader(f.Type)
		default:
			panic("Unknown field: resp." + f.Name)
		}
		empty = false
	})
	return empty
}

func ValidateRespHeader(typ reflect.Type) {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		panic("resp.Header must be struct or pointer to struct.")
	}
	Traverse(reflect.New(typ).Elem(), func(_ reflect.Value, f reflect.StructField) bool {
		if f.Type.Kind() != reflect.String {
			panic("resp.Header." + f.Name + ": type must be string.")
		}
		return true
	})
	return
}
func WriteRespHeader(value reflect.Value, header http.Header) {
	Traverse(value, func(v reflect.Value, f reflect.StructField) bool {
		if value := v.String(); value != "" {
			key, _ := struct_tag.Lookup(string(f.Tag), "header")
			if key == "" {
				key = f.Name
			}
			header.Set(key, value)
		}
		return false
	})
	return
}
