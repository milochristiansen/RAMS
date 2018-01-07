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

// RAMS Player Client.
//
// Needs libvlc to be installed and the cgo flags set.
package main

import "net/http"
import "flag"
import "strings"
import "strconv"
import "io/ioutil"

import "rams/helpers"

var MediaRoot = "\\\\FREENAS\\Global Documents\\Music\\"
var DBServer = "http://127.0.0.1:2330"
var Addr = ":2331"

func main() {
	// Load options from the config file if possible.
	file, err := ioutil.ReadFile("./rams.ini")
	if err == nil {
		options := map[string]*string{
			"media": &MediaRoot,
			"db":    &DBServer,
			"addr":  &Addr,
		}
		ParseINI(string(file), "\n", func(key, value string) {
			opt, ok := options[key]
			if ok {
				*opt = value
			}
		})
	}

	flag.StringVar(&MediaRoot, "media", MediaRoot, "Path to the directory containing the media library.")
	flag.StringVar(&DBServer, "db", DBServer, "Address of the RAMS server.")
	flag.StringVar(&Addr, "addr", Addr, "Address to listen for requests on.")

	flag.Parse()

	// Start key handler goroutine
	keys := KeyServer()

	http.HandleFunc("/enqueue/", helpers.MakeValueHandler("/enqueue/", func() interface{} {
		return new([]int)
	}, func(dat interface{}) (interface{}, error) {
		ids := *dat.(*[]int)
		GlobalPlayer.Enqueue(true, ids...)
		return GlobalPlayer.State(), nil
	}))

	http.HandleFunc("/dequeue/", helpers.MakeIntHandler("/dequeue/", func(index int) (interface{}, error) {
		GlobalPlayer.Dequeue(index)
		return GlobalPlayer.State(), nil
	}))

	// Call with a player keycode to control playback or call with a negative value to query current status.
	http.HandleFunc("/state/", helpers.MakeIntHandler("/state/", func(state int) (interface{}, error) {
		GlobalPlayer.FakeInput(state)
		// May not reflect the new state yet!
		return GlobalPlayer.State(), nil
	}))

	http.HandleFunc("/playlist/save/", helpers.MakeStringHandler("/playlist/save/", func(playlist string) (interface{}, error) {
		err := GlobalPlayer.SavePlaylist(playlist)
		if err != nil {
			return nil, err
		}
		return true, nil
	}))

	http.HandleFunc("/playlist/load/", helpers.MakeStringHandler("/playlist/load/", func(playlist string) (interface{}, error) {
		err := GlobalPlayer.LoadPlaylist(playlist, false)
		if err != nil {
			return nil, err
		}
		return true, nil
	}))

	http.HandleFunc("/socket", GlobalSockets.Upgrade)

	InitPlayer(keys, MediaRoot, DBServer)

	err = http.ListenAndServe(Addr, nil)
	if err != nil {
		panic(err)
	}
}

// ParseINI is an extremely lazy INI parser.
// Malformed lines are silently skipped.
func ParseINI(input string, linedelim string, handler func(key, value string)) {
	lines := strings.Split(input, linedelim)
	for i := range lines {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		parts[0] = strings.TrimSpace(parts[0])
		parts[1] = strings.TrimSpace(parts[1])
		if un, err := strconv.Unquote(parts[1]); err == nil {
			parts[1] = un
		}
		handler(parts[0], parts[1])
	}
}
