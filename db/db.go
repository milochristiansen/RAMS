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

// The RAMS database access layer.
package db

import _ "github.com/mattn/go-sqlite3"
import "database/sql"

import "strings"

var DB *sql.DB

const (
	SortName   sortOrder = iota
	SortTracks           // Not used by tracks
	SortAlbums           // Not used by tracks
	SortArtist
	SortAlbum
	SortDisk
	SortTrack
	SortLen
	SortYear
	SortPath

	SortID
)

type queryHolder struct {
	Code string
	Asc  *sql.Stmt
	Dsc  *sql.Stmt
}

var InitCode = `
create table if not exists Tracks (
	ID integer primary key,
	Name text collate nocase,

	Artist text,
	AlbumArtist text,
	Album text,

	Composer text,
	Publisher text,

	Comments text,

	Disk integer,
	Track integer,

	Length integer,

	Genre text,
	Year integer,

	Path text,

	Art blob,
	ArtMIME text
);
`

var queryPrefix = `select ID from Tracks where (?1 = "" or Tracks.Artist = ?1) and (?2 = "" or Tracks.Album = ?2) `

var queries = map[sortOrder]*queryHolder{
	SortName:   &queryHolder{queryPrefix + `order by Name asc, Artist, Album, Disk, Track;`, nil, nil},
	SortArtist: &queryHolder{queryPrefix + `order by Name asc, Album, Disk, Track, Name;`, nil, nil},
	SortAlbum:  &queryHolder{queryPrefix + `order by Album asc, Disk, Track, Name;`, nil, nil},
	SortDisk:   &queryHolder{queryPrefix + `order by Disk asc, Track, Album, Name;`, nil, nil},
	SortTrack:  &queryHolder{queryPrefix + `order by Track asc, Disk, Album, Name;`, nil, nil},
	SortLen:    &queryHolder{queryPrefix + `order by Length asc, Track, Disk, Album, Name;`, nil, nil},
	SortYear:   &queryHolder{queryPrefix + `order by Year asc, Album, Disk, Track, Name;`, nil, nil},
	SortPath:   &queryHolder{queryPrefix + `order by Path asc, Album, Disk, Track, Name;`, nil, nil},
	SortID:     &queryHolder{queryPrefix + `order by ID asc;`, nil, nil},
}

var listCacheQuery *sql.Stmt
var trackQuery *sql.Stmt
var trackArtQuery *sql.Stmt
var trackInsert *sql.Stmt
var trackBulkLoad *sql.Stmt
var trackUpdate *sql.Stmt
var trackArtUpdate *sql.Stmt

func init() {
	var err error
	DB, err = sql.Open("sqlite3", "file:metadata.db")
	if err != nil {
		panic(err)
	}

	_, err = DB.Exec(InitCode)
	if err != nil {
		panic(err)
	}

	listCacheQuery, err = DB.Prepare(`select Artist, Album, Year from Tracks;`)
	if err != nil {
		panic(err)
	}

	trackQuery, err = DB.Prepare(`
		select
		--  1     2      3         4         5        6         7         8       9      0       1      2     3      4
			ID, Name, Artist, AlbumArtist, Album, Composer, Publisher, Comments, Disk, Track, Length, Genre, Year, Path
		from Tracks where ID = ?;
	`)
	if err != nil {
		panic(err)
	}

	trackArtQuery, err = DB.Prepare(`
		select
			Art, ArtMIME
		from Tracks where ID = ?;
	`)
	if err != nil {
		panic(err)
	}

	trackInsert, err = DB.Prepare(`
		insert into Tracks
			-- 2      3         4         5       6          7         8       9      0      1       2     3     4
			(Name, Artist, AlbumArtist, Album, Composer, Publisher, Comments, Disk, Track, Length, Genre, Year, Path)
			--      2  3  4  5  6  7  8  9  0  1  2  3  4
			values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`)
	if err != nil {
		panic(err)
	}

	// Used to load database backups.
	trackBulkLoad, err = DB.Prepare(`
		insert or replace into Tracks
			--1   2      3         4         5       6          7         8       9      0      1       2     3      4
			(ID, Name, Artist, AlbumArtist, Album, Composer, Publisher, Comments, Disk, Track, Length, Genre, Year, Path)
			--      1  2  3  4  5  6  7  8  9  0  1  2  3  4
			values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`)
	if err != nil {
		panic(err)
	}

	trackUpdate, err = DB.Prepare(`
		update Tracks
			ID = ?1,
			Name = ?2,
			Artist = ?3,
			AlbumArtist = ?4,
			Album = ?5,
			Composer = ?6,
			Publisher = ?7,
			Comments = ?8,
			Disk = ?9,
			Track = ?10,
			Length = ?11,
			Genre = ?12,
			Year = ?13,
			Path = ?14
	`)
	if err != nil {
		panic(err)
	}

	trackArtUpdate, err = DB.Prepare(`
		update Tracks
			Art = ?2,
			ArtMIME = ?3
		where
			ID = ?1
		limit 1;
	`)
	if err != nil {
		panic(err)
	}

	for _, v := range queries {
		v.Asc, err = DB.Prepare(v.Code)
		if err != nil {
			panic(err)
		}
		v.Dsc, err = DB.Prepare(strings.Replace(v.Code, " asc", " desc", -1))
		if err != nil {
			panic(err)
		}
	}
}
