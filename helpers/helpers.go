/*
Copyright 2017 by Milo Christiansen

This software is provided 'as-is', without any express or implied warranty. In
no event will the authors be held liable for any damages arising from the use of
this software.

Permission is granted to anyone to use this software for any purpose, including
commercial applications, and to alter it and redistribute it freely, subject to
the following restrictions:

1. The origin of this software must not be misrepresented; you must not claim
that you wrote the original software. If you use this software in a product, an
acknowledgment in the product documentation would be appreciated but is not
required.

2. Altered source versions must be plainly marked as such, and must not be
misrepresented as being the original software.

3. This notice may not be removed or altered from any source distribution.
*/

// Helpers used to build and query the RAMS HTTP API.
package helpers

import "encoding/json"
import "io/ioutil"
import "net/http"
import "reflect"
import "strconv"
import "strings"
import "errors"
import "bytes"
import "fmt"

// MakeJSONHandler creates a HTTP handler designed to read JSON, pass it to a provided processing function, and send the data from the function
// back to the requester as JSON.
//
// Users need to provide two functions: The first (newdat) creates the object that the request JSON should be unmarshaled into, the second (core)
// takes the object from newdat after it has been populated with the request data and returns an error or an object to send to the requester.
func MakeJSONHandler(newdat func() interface{}, core func(dat interface{}) (interface{}, error)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")

		freq := jsonBody(w, r, newdat)
		if freq == nil {
			return
		}
		//fmt.Printf(" %#v\n", freq)

		coreHandler(core, freq, w)
	}
}

// Unused
func MakeJSONRequestHandler(newdat func() interface{}, core func(dat interface{}, w http.ResponseWriter) error) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")

		freq := jsonBody(w, r, newdat)
		if freq == nil {
			return
		}
		//fmt.Printf(" %#v\n", freq)

		// Call the body of the handler.
		err := core(freq, w)
		if err != nil {
			fmt.Println("Error:", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func MakeStructHandler(newdat func() interface{}, core func(dat interface{}) (interface{}, error)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")

		err := r.ParseForm()
		if err != nil {
			fmt.Println("Error:", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var freq interface{}
		if r.Method == "POST" {
			freq = jsonBody(w, r, newdat)
			if freq == nil {
				return
			}
		} else {
			// Parse the query into a struct.
			freq = newdat()
			rval := reflect.ValueOf(freq)
			if rval.Kind() != reflect.Ptr {
				fmt.Println("Error: Incorrect type from call to newdat.")
				http.Error(w, "Internal Error.", http.StatusInternalServerError)
				return
			}
			rval = rval.Elem()
			if rval.Kind() != reflect.Struct {
				fmt.Println("Error: Incorrect type from call to newdat.")
				http.Error(w, "Internal Error.", http.StatusInternalServerError)
				return
			}

			rtyp := rval.Type()

			// For each struct field find a matching query parameter.
			fl := rtyp.NumField()
			for i := 0; i < fl; i++ {
				f := rtyp.Field(i)

				name := f.Tag.Get("url")
				if name == "" {
					name = f.Name
				}
				if name == "-" {
					continue
				}

				values, ok := r.Form[name]
				if !ok {
					// field not supplied, leave at zero value
					continue
				}

				err := from(f.Type.Kind())(values, rval.Field(i))
				if err != nil {
					fmt.Println("Error:", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}

		coreHandler(core, freq, w)
	}
}

func MakeIntHandler(prefix string, core func(dat int) (interface{}, error)) func(http.ResponseWriter, *http.Request) {
	return MakeValueHandler(prefix, func() interface{} { return new(int) }, func(dat interface{}) (interface{}, error) {
		return core(*dat.(*int))
	})
}

func MakeStringHandler(prefix string, core func(dat string) (interface{}, error)) func(http.ResponseWriter, *http.Request) {
	return MakeValueHandler(prefix, func() interface{} { return new(string) }, func(dat interface{}) (interface{}, error) {
		return core(*dat.(*string))
	})
}

func MakeValueHandler(prefix string, newdat func() interface{}, core func(dat interface{}) (interface{}, error)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")

		var freq interface{}
		if r.Method == "POST" {
			freq = jsonBody(w, r, newdat)
			if freq == nil {
				return
			}
		} else {
			freq = newdat()
			rval := reflect.ValueOf(freq)

			err := from(rval.Type().Kind())(strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/"), rval)
			if err != nil {
				fmt.Println("Error:", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		coreHandler(core, freq, w)
	}
}

func MakeValueRequestHandler(prefix string, newdat func() interface{}, core func(dat interface{}, w http.ResponseWriter) error) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")

		var freq interface{}
		if r.Method == "POST" {
			freq = jsonBody(w, r, newdat)
			if freq == nil {
				return
			}
		} else {
			freq = newdat()
			rval := reflect.ValueOf(freq)

			err := from(rval.Type().Kind())(strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/"), rval)
			if err != nil {
				fmt.Println("Error:", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		err := core(freq, w)
		if err != nil {
			fmt.Println("Error:", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func JSONQuery(addr string, query interface{}, newdat func() interface{}) (interface{}, error) {
	buff := new(bytes.Buffer)
	enc := json.NewEncoder(buff)
	err := enc.Encode(query)
	if err != nil {
		return nil, err
	}

	r, err := http.Post(addr, "application/json", buff)
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	dec := json.NewDecoder(r.Body)
	rtn := newdat()
	err = dec.Decode(rtn)
	return rtn, err
}

// Read the body as JSON
func jsonBody(w http.ResponseWriter, r *http.Request, newdat func() interface{}) interface{} {
	// Read the request
	rreq, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Println("Error:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	// Create the unmarshal target, and populate it.
	freq := newdat()
	err = json.Unmarshal(rreq, freq)
	if err != nil {
		fmt.Println("Error:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}
	return freq
}

// Call the core function and marshal the response as JSON.
func coreHandler(core func(dat interface{}) (interface{}, error), dat interface{}, w http.ResponseWriter) {
	ret, err := core(dat)
	if err != nil {
		fmt.Println("Error:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Marshal the response object.
	retbytes, err := json.Marshal(ret)
	if err != nil {
		fmt.Println("Error:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send the marshaled response object.
	i, err := w.Write(retbytes)
	if i != len(retbytes) || err != nil {
		fmt.Println("Error:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// []string -> reflect.Value

var ErrCantSet = errors.New("Cannot set given value.")
var ErrCantConv = errors.New("Conversion to given type not implemented.")
var ErrBadConv = errors.New("Conversion to required type not possible for this value.")

func from(k reflect.Kind) func([]string, reflect.Value) error {
	switch k {
	case reflect.String:
		return fromString
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return fromUInt
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fromInt
	case reflect.Float32, reflect.Float64:
		return fromFloat
	case reflect.Bool:
		return fromBool
	case reflect.Array, reflect.Slice:
		return fromSlice
	case reflect.Ptr, reflect.Interface:
		return fromPtr
	}
	return fromErr
}

func fromString(v []string, r reflect.Value) error {
	if !r.CanSet() {
		return ErrCantSet
	}

	r.SetString(v[0])
	return nil
}

func fromUInt(v []string, r reflect.Value) error {
	if !r.CanSet() {
		return ErrCantSet
	}

	u, err := strconv.ParseUint(v[0], 0, 64)
	if err != nil {
		return ErrBadConv
	}

	r.SetUint(u)
	return nil
}

func fromInt(v []string, r reflect.Value) error {
	if !r.CanSet() {
		return ErrCantSet
	}

	i, err := strconv.ParseInt(v[0], 0, 64)
	if err != nil {
		return ErrBadConv
	}

	r.SetInt(i)
	return nil
}

func fromFloat(v []string, r reflect.Value) error {
	if !r.CanSet() {
		return ErrCantSet
	}

	f, err := strconv.ParseFloat(v[0], 64)
	if err != nil {
		return ErrBadConv
	}

	r.SetFloat(f)
	return nil
}

func fromBool(v []string, r reflect.Value) error {
	if !r.CanSet() {
		return ErrCantSet
	}

	b, err := strconv.ParseBool(v[0])
	if err != nil {
		return ErrBadConv
	}

	r.SetBool(b)
	return nil
}

func fromSlice(v []string, r reflect.Value) error {
	if !r.CanSet() {
		return ErrCantSet
	}

	l := len(v)

	// Based on code from "encoding/json"
	if r.Kind() == reflect.Slice {
		// Grow slice if necessary
		if l > r.Cap() {
			newcap := r.Cap() + r.Cap()/2
			if newcap < 4 {
				newcap = 4
			}
			if newcap < l {
				newcap = l
			}
			newv := reflect.MakeSlice(r.Type(), r.Len(), newcap)
			reflect.Copy(newv, r)
			r.Set(newv)
		}
		if l > r.Len() {
			r.SetLen(l)
		}
	}

	// Adjust l
	if l > r.Len() {
		l = r.Len()
	}

	for i := 0; i < l; i++ {
		d := r.Index(i)

		err := from(d.Kind())([]string{v[i]}, d)
		if err != nil {
			return err
		}
	}
	return nil
}

func fromPtr(v []string, r reflect.Value) error {
	isnil := r.IsNil()
	r = r.Elem()

	// If nil and a concrete type make new item.
	// This *should* be all that is needed for auto-vivification.
	if isnil {
		switch r.Kind() {
		case reflect.String,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Float32, reflect.Float64,
			reflect.Bool, reflect.Ptr, reflect.Array:
			r.Set(reflect.New(r.Type()).Elem())
		case reflect.Slice:
			r.Set(reflect.MakeSlice(r.Type(), 0, 0))
		default:
			return ErrCantConv
		}
	}

	return from(r.Kind())(v, r)
}

func fromErr(v []string, r reflect.Value) error {
	return ErrCantConv
}
