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

// RAMS: Remote Audio Metadata Server.
package main

import "fmt"
import "net/http"
import "io/ioutil"

import "flag"

import "rams/db"
import "rams/helpers"

var Addr = ""

func main() {
	defer db.DB.Close()

	flag.StringVar(&Addr, "addr", ":2330", "Address to listen for requests on.")

	flag.Parse()

	err := db.RefreshLists()
	if err != nil {
		panic(err)
	}

	http.HandleFunc("/list", helpers.MakeStructHandler(func() interface{} {
		return new(dbListRequest)
	}, func(dat interface{}) (interface{}, error) {
		dbreq := dat.(*dbListRequest)

		switch dbreq.Typ {
		case "artist":
			sort := db.SortName
			switch dbreq.Sort {
			case "name":
				sort = db.SortName
			case "tracks":
				sort = db.SortTracks
			case "albums":
				sort = db.SortAlbums
			}

			rows := db.ListArtists(dbreq.Album, sort, dbreq.Asc)

			//fmt.Printf("%v Records returned.\n", len(rows))
			return rows, nil
		case "album":
			sort := db.SortYear
			switch dbreq.Sort {
			case "name":
				sort = db.SortName
			case "year":
				sort = db.SortYear
			case "tracks":
				sort = db.SortTracks
			}

			rows := db.ListAlbums(dbreq.Artist, sort, dbreq.Asc)

			//fmt.Printf("%v Records returned.\n", len(rows))
			return rows, nil
		case "track":
			sort := db.SortAlbum
			switch dbreq.Sort {
			case "name":
				sort = db.SortName
			case "artist":
				sort = db.SortArtist
			case "album":
				sort = db.SortAlbum
			case "disk":
				sort = db.SortDisk
			case "track":
				sort = db.SortTrack
			case "length":
				sort = db.SortLen
			case "year":
				sort = db.SortYear
			case "path":
				sort = db.SortPath
			}

			rows, err := db.QueryTracks(dbreq.Artist, dbreq.Album, sort, dbreq.Asc)
			if err != nil {
				return nil, err
			}

			//fmt.Printf("%v Records returned.\n", len(rows))
			return rows, nil
		}
		return nil, fmt.Errorf("Invalid request type: \"%v\"", dbreq.Typ)
	}))

	http.HandleFunc("/data/", helpers.MakeValueHandler("/data/", func() interface{} {
		return new([]int)
	}, func(dat interface{}) (interface{}, error) {
		dbreq := *dat.(*[]int)

		rows := make([]*db.Track, len(dbreq))
		for i, id := range dbreq {
			row, err := db.QueryTrack(id)
			if err != nil {
				return nil, err
			}
			rows[i] = row
		}

		//fmt.Printf("%v Records returned.\n", len(rows))
		return rows, nil
	}))

	http.HandleFunc("/art/", helpers.MakeValueRequestHandler("/art/", func() interface{} {
		return new(int)
	}, func(dat interface{}, w http.ResponseWriter) error {
		idx := *dat.(*int)

		row, err := db.QueryTrackArt(idx)
		if err != nil {
			goto defimg
		}

		if row.ArtMIME == "" {
			goto defimg
		}
		w.Header().Set("Content-Type", row.ArtMIME)
		fmt.Fprintf(w, "%s", row.Art)
		return nil

	defimg:
		content, err := ioutil.ReadFile("./default.png")
		if err != nil {
			return err
		}

		w.Header().Set("Content-Type", "image/png")
		fmt.Fprintf(w, "%s", content)
		return nil
	}))

	http.HandleFunc("/update", helpers.MakeStructHandler(func() interface{} { return new(db.Track) }, func(dat interface{}) (interface{}, error) {
		dbupd := dat.(*db.Track)

		err := db.UpdateTrack(dbupd)
		if err != nil {
			return nil, err
		}

		err = db.RefreshLists()
		if err != nil {
			return nil, err
		}
		return true, nil
	}))

	http.HandleFunc("/refresh", func(w http.ResponseWriter, r *http.Request) {
		err := db.RefreshLists()
		if err != nil {
			fmt.Println("Error:", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	err = http.ListenAndServe(Addr, nil)
	if err != nil {
		panic(err)
	}
}

type dbListRequest struct {
	Typ    string `json:"typ" url:"typ"`
	Artist string `json:"artist" url:"artist"`
	Album  string `json:"album" url:"album"`
	Sort   string `json:"sort" url:"sort"`
	Asc    bool   `json:"asc" url:"asc"`
}
